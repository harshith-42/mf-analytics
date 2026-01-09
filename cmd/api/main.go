package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mf-analytics-service/internal/api"
	"mf-analytics-service/internal/config"
	"mf-analytics-service/internal/logging"
	"mf-analytics-service/internal/storage"
)

func main() {
	ctx := context.Background()

	logger := logging.New(logging.Options{Service: "api"})

	appCfg, err := config.Load()
	if err != nil {
		logger.Error("config load", "error", err)
		os.Exit(1)
	}
	if err := appCfg.Validate(); err != nil {
		logger.Error("config validate", "error", err)
		os.Exit(1)
	}

	cfg := storage.Config{DatabaseURL: appCfg.DatabaseURL}

	pool, err := storage.NewPool(ctx, cfg)
	if err != nil {
		logger.Error("db pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	addr := appCfg.HTTPAddr
	srv := api.NewServer(pool, logger)

	go func() {
		logger.Info("api listening", "addr", addr)
		if err := srv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
			logger.Error("listen", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Warn("shutdown", "error", err)
	}
}
