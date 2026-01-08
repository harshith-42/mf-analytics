CREATE TABLE funds (
    scheme_code        VARCHAR(20) PRIMARY KEY,
    scheme_name        TEXT NOT NULL,
    amc                TEXT NOT NULL,
    category            TEXT NOT NULL,
    inception_date     DATE,
    created_at         TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE nav_history (
    scheme_code   VARCHAR(20) NOT NULL,
    nav_date      DATE NOT NULL,
    nav_value     NUMERIC(10,4) NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT NOW(),

    PRIMARY KEY (scheme_code, nav_date),
    FOREIGN KEY (scheme_code) REFERENCES funds(scheme_code)
);

CREATE TABLE fund_analytics (
    scheme_code    VARCHAR(20) NOT NULL,
    "window"       VARCHAR(5) NOT NULL, -- 1Y | 3Y | 5Y | 10Y

    rolling_min    NUMERIC(6,2),
    rolling_max    NUMERIC(6,2),
    rolling_median NUMERIC(6,2),
    rolling_p25    NUMERIC(6,2),
    rolling_p75    NUMERIC(6,2),

    max_drawdown   NUMERIC(6,2),

    cagr_min       NUMERIC(6,2),
    cagr_max       NUMERIC(6,2),
    cagr_median    NUMERIC(6,2),

    data_start_date DATE,
    data_end_date   DATE,
    nav_points      INT,
    rolling_periods INT,

    computed_at     TIMESTAMP NOT NULL DEFAULT NOW(),

    PRIMARY KEY (scheme_code, "window"),
    FOREIGN KEY (scheme_code) REFERENCES funds(scheme_code)
);

CREATE TABLE sync_state (
    scheme_code        VARCHAR(20) PRIMARY KEY,
    last_synced_date   DATE,
    status             VARCHAR(20) NOT NULL,
    -- PENDING | IN_PROGRESS | COMPLETED | FAILED

    retry_count        INT NOT NULL DEFAULT 0,
    last_error         TEXT,
    last_attempt_at    TIMESTAMP,

    updated_at         TIMESTAMP NOT NULL DEFAULT NOW(),

    FOREIGN KEY (scheme_code) REFERENCES funds(scheme_code)
);

CREATE TABLE rate_limiter_state (
    window_type   VARCHAR(10) PRIMARY KEY,
    -- second | minute | hour

    window_start  TIMESTAMP NOT NULL,
    request_count INT NOT NULL,

    updated_at    TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE sync_runs (
    run_id        UUID PRIMARY KEY,
    run_type      VARCHAR(20) NOT NULL,
    -- BACKFILL | INCREMENTAL | MANUAL

    status        VARCHAR(20) NOT NULL,
    -- RUNNING | COMPLETED | FAILED

    started_at    TIMESTAMP NOT NULL,
    finished_at   TIMESTAMP,
    error_summary TEXT
);

CREATE INDEX idx_nav_history_scheme_date
ON nav_history (scheme_code, nav_date);

CREATE INDEX idx_fund_analytics_window_median
ON fund_analytics ("window", rolling_median DESC);

CREATE INDEX idx_fund_analytics_window_drawdown
ON fund_analytics ("window", max_drawdown ASC);

CREATE INDEX idx_sync_state_status
ON sync_state (status);

