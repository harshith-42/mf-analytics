-- name: UpsertFundAnalytics :exec
INSERT INTO fund_analytics (
  scheme_code, "window",
  rolling_min, rolling_max, rolling_median, rolling_p25, rolling_p75,
  max_drawdown,
  cagr_min, cagr_max, cagr_median,
  data_start_date, data_end_date, nav_points, rolling_periods,
  computed_at
)
VALUES (
  $1, $2,
  $3, $4, $5, $6, $7,
  $8,
  $9, $10, $11,
  $12, $13, $14, $15,
  NOW()
)
ON CONFLICT (scheme_code, "window") DO UPDATE SET
  rolling_min = EXCLUDED.rolling_min,
  rolling_max = EXCLUDED.rolling_max,
  rolling_median = EXCLUDED.rolling_median,
  rolling_p25 = EXCLUDED.rolling_p25,
  rolling_p75 = EXCLUDED.rolling_p75,
  max_drawdown = EXCLUDED.max_drawdown,
  cagr_min = EXCLUDED.cagr_min,
  cagr_max = EXCLUDED.cagr_max,
  cagr_median = EXCLUDED.cagr_median,
  data_start_date = EXCLUDED.data_start_date,
  data_end_date = EXCLUDED.data_end_date,
  nav_points = EXCLUDED.nav_points,
  rolling_periods = EXCLUDED.rolling_periods,
  computed_at = NOW();

-- name: GetFundAnalytics :one
SELECT *
FROM fund_analytics
WHERE scheme_code = $1
  AND "window" = $2;

-- name: RankFundsByMedianReturn :many
SELECT
  fa.scheme_code,
  f.scheme_name,
  f.amc,
  f.category,
  fa."window",
  fa.rolling_median,
  fa.max_drawdown,
  nav.nav_value AS current_nav,
  nav.nav_date AS last_updated
FROM fund_analytics fa
JOIN funds f ON f.scheme_code = fa.scheme_code
LEFT JOIN LATERAL (
  SELECT nh.nav_value, nh.nav_date
  FROM nav_history nh
  WHERE nh.scheme_code = fa.scheme_code
  ORDER BY nh.nav_date DESC
  LIMIT 1
) nav ON true
WHERE f.category = $1
  AND fa."window" = $2
ORDER BY fa.rolling_median DESC NULLS LAST
LIMIT $3;

-- name: RankFundsByMaxDrawdown :many
SELECT
  fa.scheme_code,
  f.scheme_name,
  f.amc,
  f.category,
  fa."window",
  fa.rolling_median,
  fa.max_drawdown,
  nav.nav_value AS current_nav,
  nav.nav_date AS last_updated
FROM fund_analytics fa
JOIN funds f ON f.scheme_code = fa.scheme_code
LEFT JOIN LATERAL (
  SELECT nh.nav_value, nh.nav_date
  FROM nav_history nh
  WHERE nh.scheme_code = fa.scheme_code
  ORDER BY nh.nav_date DESC
  LIMIT 1
) nav ON true
WHERE f.category = $1
  AND fa."window" = $2
ORDER BY fa.max_drawdown ASC NULLS LAST
LIMIT $3;

