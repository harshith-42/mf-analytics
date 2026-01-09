package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mf-analytics-service/internal/config"
	"mf-analytics-service/internal/logging"
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

	logger := logging.New(logging.Options{Service: "worker"})

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

	rlCfg, err := appCfg.RateLimiterConfig()
	if err != nil {
		logger.Error("rate limiter config", "error", err)
		os.Exit(1)
	}
	rlCfg.Logger = logging.PrintfAdapter{L: logger}
	rl, err := ratelimiter.New(pool, rlCfg)
	if err != nil {
		logger.Error("rate limiter", "error", err)
		os.Exit(1)
	}

	mf := mfapi.New("https://api.mfapi.in", mfapi.WithRateLimiter(rl), mfapi.WithLogger(logger))

	staleAfter := 15 * time.Minute
	if v := os.Getenv("SYNC_STALE_AFTER"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			staleAfter = d
		}
	}

	runner := pipeline.NewBackfillRunner(pool, mf, staleAfter, logger)

	pollEvery := 2 * time.Second
	for {
		processed, err := runner.RunLatest(ctx)
		if err != nil {
			logger.Error("worker run", "error", err)
			os.Exit(1)
		}
		if processed {
			logger.Info("worker finished a run; waiting for next")
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollEvery):
		}
	}
}
