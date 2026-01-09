-- name: ListFunds :many
SELECT scheme_code, scheme_name, amc, category, inception_date, created_at, updated_at
FROM funds
WHERE (sqlc.narg('category')::text IS NULL OR category = sqlc.narg('category')::text)
  AND (sqlc.narg('amc')::text IS NULL OR amc = sqlc.narg('amc')::text)
ORDER BY scheme_name ASC;

-- name: GetFund :one
SELECT scheme_code, scheme_name, amc, category, inception_date, created_at, updated_at
FROM funds
WHERE scheme_code = $1;

-- name: UpsertFund :exec
INSERT INTO funds (scheme_code, scheme_name, amc, category, inception_date, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
ON CONFLICT (scheme_code) DO UPDATE SET
  scheme_name = EXCLUDED.scheme_name,
  amc = EXCLUDED.amc,
  category = EXCLUDED.category,
  inception_date = EXCLUDED.inception_date,
  updated_at = NOW();

-- name: CountFundsByCategory :one
SELECT COUNT(*)::bigint
FROM funds
WHERE category = $1;
