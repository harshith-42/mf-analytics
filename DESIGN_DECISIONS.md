# DESIGN_DECISIONS.md

## Overview
This service ingests mutual fund NAV time-series from `https://api.mfapi.in`, computes rolling performance analytics (1Y/3Y/5Y/10Y), and serves fast query endpoints (rankings and per-fund analytics). The system is designed to be correct under strict API quotas, resumable after crashes, and scalable in a production setting.

Key principles:
- **Correctness over cleverness** for rate limiting (provable, persistent).
- **Idempotent ingestion** (safe retries, no duplicates).
- **Precomputed analytics** for predictable <200ms API responses.
- **Stateless API** and **scalable workers** (horizontal scaling with DB-coordinated claiming).

---

## Rate limiting strategy (persistent fixed windows)
### Constraints
mfapi.in enforces:
- 2 requests/sec
- 50 requests/min
- 300 requests/hour

All three apply simultaneously; violations can lead to 429s, blocks, or exhausted quota.

### Design choice
Use a **DB-persisted fixed-window counter** per limit in `rate_limiter_state`:
- `window_type`: `second | minute | hour`
- `window_start`: start of the active window
- `request_count`: number of requests within that window

### How all three limits are coordinated
Every outbound mfapi request must first call `Limiter.Acquire(ctx)` (see `internal/ratelimiter/limiter.go`). `Acquire` uses a single Postgres transaction and row locks:
- Ensure the 3 rows exist (upsert with count=0 if missing)
- `SELECT ... FOR UPDATE` each row
- If any window is exhausted, **do not increment anything** and return the required wait time
- Otherwise increment all three counters and commit

This yields a simple correctness proof:
- **Safety**: row-level locks + transactional increments guarantee counters never exceed limits, even with concurrent workers.
- **Persistence**: counters survive restarts because state is stored in Postgres.
- **Coordination**: a request is permitted iff **all** windows allow it; increments are applied together atomically.

### Trade-offs / scalability path
This approach adds DB contention, but the external cap is 2 req/sec, so contention is bounded. If future requirements add higher outbound RPS to other providers, the limiter can be swapped to Redis/Lua or a dedicated limiter service without changing ingestion semantics.

---

## Backfill orchestration under quota constraints
### Backfill behavior
Backfill fetches full NAV history per scheme and upserts it into `nav_history`.

Implementation highlights:
- **Idempotency**: `(scheme_code, nav_date)` primary key + `ON CONFLICT DO UPDATE` makes retries safe.
- **Resumability**: per-scheme progress stored in `sync_state` with statuses `PENDING|IN_PROGRESS|COMPLETED|FAILED`.
- **Operational visibility**: each run recorded in `sync_runs`.

### Crash recovery
If a worker crashes after claiming a scheme, that scheme may remain `IN_PROGRESS`. On startup, workers requeue stale work via `RequeueStaleInProgressSyncState` so backfill can resume.

### Worker scaling model
Workers claim work using `FOR UPDATE SKIP LOCKED`, allowing multiple worker replicas without duplicating the same scheme.

---

## Incremental sync strategy
Incremental runs are enqueued by `cmd/cron` on a configurable schedule (`INCREMENTAL_CRON`, `TZ`). The cron process only enqueues work in Postgres; workers execute it.

Per-scheme incremental uses mfapi date ranges:
- `startDate = last_synced_date + 1 day`
- `endDate = now`

This reduces ingestion cost as the system scales to more schemes.

---

## Storage schema rationale
The schema separates concerns:
- **`funds`**: small, frequently queried fund master data.
- **`nav_history`**: time-series NAV storage, keyed by `(scheme_code, nav_date)` for dedupe and range query performance.
- **`fund_analytics`**: precomputed numeric columns for fast sorting/ranking (avoid JSON).
- **`sync_state`**: resumability and idempotency in ingestion.
- **`rate_limiter_state`**: persistent quota enforcement across restarts.
- **`sync_runs`**: operational visibility for `/sync/status`.

Indexes are chosen to make rank queries and NAV lookups predictable and <200ms.

---

## Precomputation vs on-demand analytics
We precompute analytics into `fund_analytics` to keep the API predictable and fast:
- Rank endpoints sort on indexed numeric columns.
- Fund analytics endpoint is a single-row read.

Trade-off: more work during ingestion. This is acceptable because ingestion is rate-limited externally and can run asynchronously.

---

## Handling insufficient history
If a fund lacks enough data to compute a given window (e.g., 10Y), the service still upserts a row for that window with:
- availability fields populated
- metric columns left NULL

This allows the API to respond consistently and makes data gaps explicit.

