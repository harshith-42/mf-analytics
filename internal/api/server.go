package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool *pgxpool.Pool
	r    *chi.Mux
	srv  *http.Server
	log  *slog.Logger
}

func NewServer(pool *pgxpool.Pool, logger *slog.Logger) *Server {
	s := &Server{
		pool: pool,
		r:    chi.NewRouter(),
		log:  logger,
	}
	s.routes()

	// Standard middleware set for observability and safety.
	s.r.Use(middleware.RequestID)
	s.r.Use(middleware.Recoverer)
	s.r.Use(s.requestLogger())

	s.srv = &http.Server{
		Handler:      s.r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return s
}

func (s *Server) ListenAndServe(addr string) error {
	s.srv.Addr = addr
	return s.srv.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}
