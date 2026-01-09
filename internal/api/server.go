package api

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Server struct {
	pool *pgxpool.Pool
	r    *chi.Mux
	srv  *http.Server
}

func NewServer(pool *pgxpool.Pool) *Server {
	s := &Server{
		pool: pool,
		r:    chi.NewRouter(),
	}
	s.routes()
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
