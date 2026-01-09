package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Options struct {
	Service string
}

func New(opts Options) *slog.Logger {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	format := strings.ToLower(strings.TrimSpace(os.Getenv("LOG_FORMAT")))
	if format == "" {
		format = "text"
	}

	hopts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if format == "json" {
		h = slog.NewJSONHandler(os.Stdout, hopts)
	} else {
		h = slog.NewTextHandler(os.Stdout, hopts)
	}

	l := slog.New(h)
	if opts.Service != "" {
		l = l.With("service", opts.Service)
	}
	slog.SetDefault(l)
	return l
}

// PrintfAdapter adapts slog to the minimal Printf interface used by ratelimiter.
type PrintfAdapter struct {
	L *slog.Logger
}

func (p PrintfAdapter) Printf(format string, args ...any) {
	if p.L == nil {
		return
	}
	// Keep it as a single message so existing limiter logs stay readable.
	p.L.Info("ratelimiter", "msg", sprintf(format, args...))
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func sprintf(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}
