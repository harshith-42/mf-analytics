package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"mf-analytics-service/internal/config"
	"mf-analytics-service/internal/db"
	"mf-analytics-service/internal/logging"
	"mf-analytics-service/internal/storage"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-stop
		cancel()
	}()

	logger := logging.New(logging.Options{Service: "cron"})

	appCfg, err := config.Load()
	if err != nil {
		logger.Error("config load", "error", err)
		os.Exit(1)
	}
	if err := appCfg.Validate(); err != nil {
		logger.Error("config validate", "error", err)
		os.Exit(1)
	}

	pool, err := storage.NewPool(ctx, storage.Config{DatabaseURL: appCfg.DatabaseURL})
	if err != nil {
		logger.Error("db pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	sched := os.Getenv("INCREMENTAL_CRON")
	if sched == "" {
		sched = "0 2 * * *" // daily 02:00
	}

	loc := time.UTC
	if tz := os.Getenv("TZ"); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}

	c := cron.New(cron.WithLocation(loc))
	_, err = c.AddFunc(sched, func() {
		if err := enqueueIncremental(ctx, pool, logger); err != nil {
			logger.Warn("enqueue incremental", "error", err)
		}
	})
	if err != nil {
		logger.Error("cron schedule", "error", err)
		os.Exit(1)
	}
	c.Start()
	defer c.Stop()

	// Also attempt once at startup (helpful in interviews).
	if err := enqueueIncremental(ctx, pool, logger); err != nil {
		logger.Warn("enqueue incremental startup", "error", err)
	}

	<-ctx.Done()
}

func enqueueIncremental(ctx context.Context, pool *pgxpool.Pool, logger *slog.Logger) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)

	if _, err := q.GetLatestRunningSyncRun(ctx); err == nil {
		// A run is already active; don't enqueue another.
		if logger != nil {
			logger.Info("skip enqueue: run already running")
		}
		return tx.Commit(ctx)
	} else if err != nil && err != pgx.ErrNoRows {
		return err
	}

	u := uuid.New()
	pgID := pgtype.UUID{Bytes: uuidToBytes16(u), Valid: true}

	if err := q.CreateSyncRun(ctx, db.CreateSyncRunParams{
		RunID:   pgID,
		RunType: "INCREMENTAL",
	}); err != nil {
		return err
	}
	if logger != nil {
		logger.Info("enqueued incremental run", "run_id", u.String())
	}

	// Only queue schemes that have been completed before (or previously failed) for incremental.
	if err := q.ResetEligibleIncrementalSyncStateToPending(ctx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func uuidToBytes16(u uuid.UUID) [16]byte {
	var b [16]byte
	copy(b[:], u[:])
	return b
}

