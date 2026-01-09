package main

import (
	"context"
	"log"
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

	appCfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := appCfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := storage.NewPool(ctx, storage.Config{DatabaseURL: appCfg.DatabaseURL})
	if err != nil {
		log.Fatalf("db pool: %v", err)
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
		if err := enqueueIncremental(ctx, pool); err != nil {
			log.Printf("enqueue incremental: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("cron schedule: %v", err)
	}
	c.Start()
	defer c.Stop()

	// Also attempt once at startup (helpful in interviews).
	if err := enqueueIncremental(ctx, pool); err != nil {
		log.Printf("enqueue incremental (startup): %v", err)
	}

	<-ctx.Done()
}

func enqueueIncremental(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := db.New(tx)

	if _, err := q.GetLatestRunningSyncRun(ctx); err == nil {
		// A run is already active; don't enqueue another.
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

