package ratelimiter

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLimiter_ConcurrencyRespectsLimits(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	if err := resetSchema(ctx, pool); err != nil {
		t.Fatalf("resetSchema: %v", err)
	}

	l, err := New(pool, Config{
		Now: time.Now,
		Windows: []WindowConfig{
			{Type: WindowSecond, Duration: 100 * time.Millisecond, Limit: 2},
			{Type: WindowMinute, Duration: 500 * time.Millisecond, Limit: 10},
			{Type: WindowHour, Duration: 2 * time.Second, Limit: 50},
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	const n = 10
	var (
		wg    sync.WaitGroup
		mu    sync.Mutex
		times = make([]time.Time, 0, n)
	)
	wg.Add(n)
	start := time.Now()
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if err := l.Acquire(ctx); err != nil {
				t.Errorf("Acquire: %v", err)
				return
			}
			mu.Lock()
			times = append(times, time.Now())
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(times) != n {
		t.Fatalf("expected %d acquires, got %d", n, len(times))
	}

	// No more than 2 acquires in any 100ms fixed window.
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	dur := 100 * time.Millisecond
	for i := 0; i < len(times); i++ {
		windowEnd := times[i].Add(dur)
		count := 0
		for j := i; j < len(times) && times[j].Before(windowEnd); j++ {
			count++
		}
		if count > 2 {
			t.Fatalf("rate exceeded: %d acquires within %s starting at %s", count, dur, times[i].Format(time.RFC3339Nano))
		}
	}

	elapsed := time.Since(start)
	// 10 requests at 2/100ms requires at least 5 windows => ~400ms minimum (allow slack).
	if elapsed < 300*time.Millisecond {
		t.Fatalf("unexpectedly fast; limiter may not be enforcing: elapsed=%s", elapsed)
	}
}

func TestLimiter_PersistsAcrossInstances(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	defer pool.Close()

	if err := resetSchema(ctx, pool); err != nil {
		t.Fatalf("resetSchema: %v", err)
	}

	cfg := Config{
		Now: time.Now,
		Windows: []WindowConfig{
			{Type: WindowSecond, Duration: 500 * time.Millisecond, Limit: 2},
		},
	}
	l1, err := New(pool, cfg)
	if err != nil {
		t.Fatalf("New l1: %v", err)
	}

	// Consume the whole window quota.
	if err := l1.Acquire(ctx); err != nil {
		t.Fatalf("Acquire1: %v", err)
	}
	if err := l1.Acquire(ctx); err != nil {
		t.Fatalf("Acquire2: %v", err)
	}

	// New instance should observe persisted state and deny immediately.
	l2, err := New(pool, cfg)
	if err != nil {
		t.Fatalf("New l2: %v", err)
	}
	wait, ok, err := l2.TryAcquire(ctx)
	if err != nil {
		t.Fatalf("TryAcquire: %v", err)
	}
	if ok {
		t.Fatalf("expected deny due to persisted count; got ok=true")
	}
	if wait <= 0 {
		t.Fatalf("expected positive wait; got %s", wait)
	}
}

func resetSchema(ctx context.Context, pool *pgxpool.Pool) error {
	// Best-effort cleanup; ignore errors for non-existent tables.
	drop := []string{
		"DROP TABLE IF EXISTS sync_runs",
		"DROP TABLE IF EXISTS rate_limiter_state",
		"DROP TABLE IF EXISTS sync_state",
		"DROP TABLE IF EXISTS fund_analytics",
		"DROP TABLE IF EXISTS nav_history",
		"DROP TABLE IF EXISTS funds",
	}
	for _, s := range drop {
		_, _ = pool.Exec(ctx, s)
	}

	ddlBytes, err := os.ReadFile("migrations/000001_init_schema.up.sql")
	if err != nil {
		return err
	}

	ddl := strings.TrimSpace(string(ddlBytes))
	if ddl == "" {
		return nil
	}
	_, err = pool.Exec(ctx, ddl)
	return err
}

