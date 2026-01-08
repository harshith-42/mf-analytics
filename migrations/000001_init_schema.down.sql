DROP INDEX IF EXISTS idx_sync_state_status;
DROP INDEX IF EXISTS idx_fund_analytics_window_drawdown;
DROP INDEX IF EXISTS idx_fund_analytics_window_median;
DROP INDEX IF EXISTS idx_nav_history_scheme_date;

DROP TABLE IF EXISTS sync_runs;
DROP TABLE IF EXISTS rate_limiter_state;
DROP TABLE IF EXISTS sync_state;
DROP TABLE IF EXISTS fund_analytics;
DROP TABLE IF EXISTS nav_history;
DROP TABLE IF EXISTS funds;

