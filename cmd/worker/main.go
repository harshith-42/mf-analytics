package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mf-analytics-service/internal/config"
	"mf-analytics-service/internal/mfapi"
	"mf-analytics-service/internal/pipeline"
	"mf-analytics-service/internal/ratelimiter"
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

	rlCfg, err := appCfg.RateLimiterConfig()
	if err != nil {
		log.Fatalf("rate limiter config: %v", err)
	}
	rl, err := ratelimiter.New(pool, rlCfg)
	if err != nil {
		log.Fatalf("rate limiter: %v", err)
	}

	mf := mfapi.New("https://api.mfapi.in", mfapi.WithRateLimiter(rl))

	staleAfter := 15 * time.Minute
	if v := os.Getenv("SYNC_STALE_AFTER"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			staleAfter = d
		}
	}

	runner := pipeline.NewBackfillRunner(pool, mf, staleAfter)

	pollEvery := 2 * time.Second
	for {
		processed, err := runner.RunLatest(ctx)
		if err != nil {
			log.Fatalf("worker run: %v", err)
		}
		if processed {
			log.Printf("worker: finished a run; waiting for next")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollEvery):
		}
	}
}
