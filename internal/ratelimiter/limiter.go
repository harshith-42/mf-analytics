package ratelimiter

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"mf-analytics-service/internal/db"
)

type WindowType string

const (
	WindowSecond WindowType = "second"
	WindowMinute WindowType = "minute"
	WindowHour   WindowType = "hour"
)

type WindowConfig struct {
	Type     WindowType
	Duration time.Duration
	Limit    int32
}

type Config struct {
	Now     func() time.Time
	Windows []WindowConfig
}

func DefaultConfig() Config {
	return Config{
		Now: time.Now,
		Windows: []WindowConfig{
			{Type: WindowSecond, Duration: time.Second, Limit: 2},
			{Type: WindowMinute, Duration: time.Minute, Limit: 50},
			{Type: WindowHour, Duration: time.Hour, Limit: 300},
		},
	}
}

type Limiter struct {
	pool *pgxpool.Pool
	cfg  Config
}

func New(pool *pgxpool.Pool, cfg Config) (*Limiter, error) {
	if pool == nil {
		return nil, fmt.Errorf("pool is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if len(cfg.Windows) == 0 {
		return nil, fmt.Errorf("at least one window is required")
	}
	for _, w := range cfg.Windows {
		if w.Type == "" {
			return nil, fmt.Errorf("window type is required")
		}
		if w.Duration <= 0 {
			return nil, fmt.Errorf("window %q duration must be > 0", w.Type)
		}
		if w.Limit <= 0 {
			return nil, fmt.Errorf("window %q limit must be > 0", w.Type)
		}
	}
	return &Limiter{pool: pool, cfg: cfg}, nil
}

// Acquire blocks until a request is permitted by *all* configured windows, or ctx is cancelled.
func (l *Limiter) Acquire(ctx context.Context) error {
	for {
		wait, ok, err := l.TryAcquire(ctx)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}

		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (l *Limiter) TryAcquire(ctx context.Context) (wait time.Duration, ok bool, err error) {
	now := l.cfg.Now().UTC()

	tx, err := l.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)

	// Ensure rows exist so FOR UPDATE works reliably.
	for _, w := range l.cfg.Windows {
		ws := truncateTo(now, w.Duration)
		if err := q.UpsertRateLimiterState(ctx, db.UpsertRateLimiterStateParams{
			WindowType:   string(w.Type),
			WindowStart:  toPgTimestamp(ws),
			RequestCount: 0,
		}); err != nil {
			return 0, false, err
		}
	}

	type windowEval struct {
		cfg         WindowConfig
		windowStart time.Time
		nextCount   int32
	}

	evals := make([]windowEval, 0, len(l.cfg.Windows))
	var maxWait time.Duration

	for _, w := range l.cfg.Windows {
		st, err := q.GetRateLimiterStateForUpdate(ctx, string(w.Type))
		if err != nil {
			return 0, false, err
		}
		if !st.WindowStart.Valid {
			return 0, false, fmt.Errorf("rate limiter window_start invalid for %q", w.Type)
		}

		ws := st.WindowStart.Time.UTC()
		if now.Sub(ws) >= w.Duration {
			// Window expired: reset to current boundary.
			ws = truncateTo(now, w.Duration)
			st.RequestCount = 0
		}

		if st.RequestCount >= w.Limit {
			until := ws.Add(w.Duration)
			wWait := until.Sub(now)
			if wWait < 0 {
				wWait = 0
			}
			if wWait > maxWait {
				maxWait = wWait
			}
			evals = append(evals, windowEval{cfg: w, windowStart: ws, nextCount: st.RequestCount})
			continue
		}

		evals = append(evals, windowEval{cfg: w, windowStart: ws, nextCount: st.RequestCount + 1})
	}

	if maxWait > 0 {
		return maxWait, false, nil
	}

	for _, e := range evals {
		if err := q.UpsertRateLimiterState(ctx, db.UpsertRateLimiterStateParams{
			WindowType:   string(e.cfg.Type),
			WindowStart:  toPgTimestamp(e.windowStart),
			RequestCount: e.nextCount,
		}); err != nil {
			return 0, false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, false, err
	}
	return 0, true, nil
}

func truncateTo(t time.Time, d time.Duration) time.Time {
	return t.Truncate(d)
}

func toPgTimestamp(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t, Valid: true}
}

var ErrRateLimiterMisconfigured = errors.New("rate limiter misconfigured")
