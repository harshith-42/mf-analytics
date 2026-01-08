
# Mutual Fund Analytics – Product Requirements Document (PRD)

## 1. Service Background

You are building an **analytics backend** for a mutual fund investment platform.
The platform computes **performance metrics** for investors, including:

* Rolling returns
* Drawdowns
* Volatility / CAGR distributions

The service integrates with the **live external API**:

```
https://api.mfapi.in
```

This API provides:

* Daily NAV (Net Asset Value) history per scheme
* Scheme metadata (fund house, category, scheme name)

---

## 2. API Rate Limits (Strictly Enforced)

All **three limits apply simultaneously**.

| Limit Type | Constraint        | Violation Consequence |
| ---------- | ----------------- | --------------------- |
| Per-second | 2 requests / sec  | HTTP 429              |
| Per-minute | 50 requests / min | 5-minute block        |
| Per-hour   | 300 requests / hr | Quota exhausted       |

**Important:**
Your design **must provably respect all three limits at the same time**.

---

## 3. Scope

### Asset Management Companies (AMCs) – 5

* ICICI Prudential
* HDFC
* Axis
* SBI
* Kotak Mahindra

### Categories – 2

* Equity: Mid Cap Direct Growth
* Equity: Small Cap Direct Growth

Total schemes tracked: **10**

---

## 4. Key Challenges

1. **Scheme Discovery**

   * mfapi.in works using `scheme_code`
   * You must identify which scheme codes belong to:

     * the 5 AMCs
     * the 2 required categories

2. **Rate Limit Orchestration**

   * Three concurrent limits (second, minute, hour)
   * All must be respected simultaneously

3. **Historical Backfill**

   * Each scheme needs **full NAV history (up to 10 years)**
   * With a **300/hour limit**, requests must be **spread intelligently**

4. **Analytics Computation**

   * Rolling 3Y, 5Y, and 10Y analytics across long history

5. **Data Quality**

   * NAV gaps (weekends, holidays)
   * Some schemes have limited historical data

---

## 5. Your Task

Design and implement a **Go service** that:

1. Ingests NAV data from mfapi.in **within rate limits**
2. Computes rolling performance analytics
3. Serves **fast ranking and analytics queries**

---

## 6. Required Deliverables

---

## 6.1 HTTP API

### 1. List Funds

```
GET /funds
```

Query parameters:

* `category`
* `amc`

---

### 2. Fund Details

```
GET /funds/{code}
```

Returns:

* Fund metadata
* Latest NAV

---

### 3. Fund Analytics

```
GET /funds/{code}/analytics
```

Query parameters:

* `window` (required): `1Y | 3Y | 5Y | 10Y`

Returns:

* Rolling returns
* Max drawdown
* CAGR distribution

---

### 4. Rank Funds

```
GET /funds/rank
```

Query parameters:

* `category` (required)
* `sort_by`: `median_return | max_drawdown`
* `window` (required): `1Y | 3Y | 5Y | 10Y`
* `limit` (default: 5)

#### Example Ranking Response

```json
{
  "category": "Equity: Mid Cap",
  "window": "3Y",
  "sorted_by": "median_return",
  "total_funds": 28,
  "showing": 10,
  "funds": [
    {
      "rank": 1,
      "fund_code": "119598",
      "fund_name": "Axis Midcap Fund - Direct Plan - Growth",
      "amc": "Axis Mutual Fund",
      "median_return_3y": 22.3,
      "max_drawdown_3y": -32.1,
      "current_nav": 78.45,
      "last_updated": "2026-01-06"
    }
  ]
}
```

---

### 5. Trigger Sync

```
POST /sync/trigger
```

Manually triggers data ingestion.

---

### 6. Sync Status

```
GET /sync/status
```

Returns pipeline state and progress.

---

## 6.2 Example Analytics Response

```json
{
  "fund_code": "119598",
  "fund_name": "Axis Midcap Fund - Direct Plan - Growth",
  "category": "Equity: Mid Cap",
  "amc": "Axis Mutual Fund",
  "window": "3Y",
  "data_availability": {
    "start_date": "2016-01-15",
    "end_date": "2026-01-06",
    "total_days": 3644,
    "nav_data_points": 2513
  },
  "rolling_periods_analyzed": 731,
  "rolling_returns": {
    "min": 8.2,
    "max": 48.5,
    "median": 22.3,
    "p25": 15.7,
    "p75": 28.9
  },
  "max_drawdown": -32.1,
  "cagr": {
    "min": 9.5,
    "max": 45.2,
    "median": 21.8
  },
  "computed_at": "2026-01-06T02:30:15Z"
}
```

---

## 7. Data Pipeline Requirements

### Backfill

* Fetch full NAV history for all schemes
* Respect all rate limits

### Incremental Sync

* Daily fetch for new NAV values

### State Persistence

* Rate limiter state **must survive restarts**

### Resumability

* Pipeline must resume correctly after crashes

---

## 8. Analytics Engine

For **each fund** and **each window (1Y / 3Y / 5Y / 10Y)**, pre-compute:

| Metric            | Description                  |
| ----------------- | ---------------------------- |
| Rolling returns   | min, max, median, p25, p75   |
| Max drawdown      | Worst peak-to-trough decline |
| CAGR distribution | min, max, median             |

---

## 9. DESIGN_DECISIONS.md (2–3 pages)

You must document:

* Rate limiting strategy and proof of correctness
* How all three limits are coordinated
* Backfill orchestration under quota constraints
* Storage schema for NAV time-series
* Pre-computation vs on-demand trade-offs
* Handling schemes with insufficient history

---

## 10. Tests Required

* Rate limiter correctness (all three limits + concurrency)
* Rate limiter state persistence
* Analytics correctness (manual verification)
* Pipeline resumability after crash
* API response time **< 200ms**

---

## 11. Design Questions (Explain Your Reasoning)

You should address:

### Rate Limiter

* Token bucket vs sliding window?
* How are three limits coordinated?

### Backfill Strategy

* Sequential or concurrent?
* How to maximize throughput without violations?

### Storage

* SQL vs time-series DB?
* Efficient range queries + writes?

### Caching

* What to cache?
* TTL strategy?
* How to achieve <200ms responses?

### Failure Handling

* Retry vs fail-fast?
* Backoff strategy?
* Circuit breaking?

---

## 12. Constraints

* Language: **Go 1.21+**
* API: Must use **live** `https://api.mfapi.in/mf/{scheme_code}`
* No mocking the external API
* Rate limits must be **provably respected** (logs/metrics)
* Expected implementation time: **4–5 hours**

---

## 13. Getting Started Hint

* Visit [https://www.mfapi.in/](https://www.mfapi.in/)
* Understand how scheme discovery works via metadata:

  * `fund_house`
  * `scheme_category`
