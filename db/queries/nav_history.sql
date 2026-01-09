-- name: UpsertNavHistory :exec
INSERT INTO nav_history (scheme_code, nav_date, nav_value, created_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (scheme_code, nav_date) DO UPDATE SET
  nav_value = EXCLUDED.nav_value;

-- name: GetLatestNav :one
SELECT scheme_code, nav_date, nav_value, created_at
FROM nav_history
WHERE scheme_code = $1
ORDER BY nav_date DESC
LIMIT 1;

-- name: ListNavHistoryForScheme :many
SELECT scheme_code, nav_date, nav_value, created_at
FROM nav_history
WHERE scheme_code = $1
ORDER BY nav_date ASC;

-- name: ListNavHistoryBetween :many
SELECT scheme_code, nav_date, nav_value, created_at
FROM nav_history
WHERE scheme_code = $1
  AND nav_date >= $2
  AND nav_date <= $3
ORDER BY nav_date ASC;

