-- name: GetRateLimiterStateForUpdate :one
SELECT window_type, window_start, request_count, updated_at
FROM rate_limiter_state
WHERE window_type = $1
FOR UPDATE;

-- name: UpsertRateLimiterState :exec
INSERT INTO rate_limiter_state (window_type, window_start, request_count, updated_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (window_type) DO UPDATE SET
  window_start = EXCLUDED.window_start,
  request_count = EXCLUDED.request_count,
  updated_at = NOW();

