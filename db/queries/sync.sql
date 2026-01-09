-- name: InitSyncStateIfMissing :exec
INSERT INTO sync_state (scheme_code, status, updated_at)
VALUES ($1, 'PENDING', NOW())
ON CONFLICT (scheme_code) DO NOTHING;

-- name: UpdateSyncStateAttempt :exec
UPDATE sync_state
SET
  status = $2,
  retry_count = $3,
  last_error = $4,
  last_attempt_at = NOW(),
  updated_at = NOW()
WHERE scheme_code = $1;

-- name: UpdateSyncStateSuccess :exec
UPDATE sync_state
SET
  status = 'COMPLETED',
  last_synced_date = $2,
  retry_count = 0,
  last_error = NULL,
  last_attempt_at = NOW(),
  updated_at = NOW()
WHERE scheme_code = $1;

-- name: ListSyncState :many
SELECT *
FROM sync_state
ORDER BY scheme_code ASC;

-- name: CountSyncStateByStatus :many
SELECT status, COUNT(*) AS count
FROM sync_state
GROUP BY status;

-- name: CreateSyncRun :exec
INSERT INTO sync_runs (run_id, run_type, status, started_at)
VALUES ($1, $2, 'RUNNING', NOW());

-- name: GetLatestRunningSyncRun :one
SELECT *
FROM sync_runs
WHERE status = 'RUNNING'
ORDER BY started_at DESC
LIMIT 1;

-- name: FinishSyncRunSuccess :exec
UPDATE sync_runs
SET status = 'COMPLETED',
    finished_at = NOW(),
    error_summary = NULL
WHERE run_id = $1;

-- name: FinishSyncRunFailure :exec
UPDATE sync_runs
SET status = 'FAILED',
    finished_at = NOW(),
    error_summary = $2
WHERE run_id = $1;

-- name: GetLatestSyncRun :one
SELECT *
FROM sync_runs
ORDER BY started_at DESC
LIMIT 1;

-- name: ResetAllSyncStateToPending :exec
UPDATE sync_state
SET
  status = 'PENDING',
  updated_at = NOW()
WHERE status <> 'IN_PROGRESS';

-- name: ResetEligibleIncrementalSyncStateToPending :exec
UPDATE sync_state
SET
  status = 'PENDING',
  updated_at = NOW()
WHERE status IN ('COMPLETED', 'FAILED');

-- name: ClaimNextSyncState :one
WITH candidate AS (
  SELECT scheme_code
  FROM sync_state
  WHERE status IN ('PENDING', 'FAILED')
  ORDER BY updated_at ASC
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE sync_state
SET
  status = 'IN_PROGRESS',
  last_attempt_at = NOW(),
  updated_at = NOW()
WHERE scheme_code IN (SELECT scheme_code FROM candidate)
RETURNING scheme_code, last_synced_date, status, retry_count, last_error, last_attempt_at, updated_at;

-- name: RequeueStaleInProgressSyncState :exec
UPDATE sync_state
SET
  status = 'PENDING',
  updated_at = NOW()
WHERE status = 'IN_PROGRESS'
  AND last_attempt_at IS NOT NULL
  AND last_attempt_at < $1;
