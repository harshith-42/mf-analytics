package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"mf-analytics-service/internal/api"
	"mf-analytics-service/internal/config"
	"mf-analytics-service/internal/storage"
)

func main() {
	ctx := context.Background()

	appCfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := appCfg.Validate(); err != nil {
		log.Fatalf("config: %v", err)
	}

	cfg := storage.Config{DatabaseURL: appCfg.DatabaseURL}

	pool, err := storage.NewPool(ctx, cfg)
	if err != nil {
		log.Fatalf("db pool: %v", err)
	}
	defer pool.Close()

	addr := appCfg.HTTPAddr
	srv := api.NewServer(pool)

	go func() {
		log.Printf("api listening on %s", addr)
		if err := srv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
