package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HTTPAddr    string          `yaml:"http_addr"`
	DatabaseURL string          `yaml:"database_url"`
	RateLimiter RateLimiterYAML `yaml:"rate_limiter"`
}

type RateLimiterYAML struct {
	Windows []RateLimiterWindowYAML `yaml:"windows"`
}

type RateLimiterWindowYAML struct {
	Type     string `yaml:"type"`
	Duration string `yaml:"duration"`
	Limit    int32  `yaml:"limit"`
}

func Load() (Config, error) {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = "config.yml"
	}

	var cfg Config
	if b, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return Config{}, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}

	// Env overrides (preferred for containerized deployment).
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("database_url/DATABASE_URL is required")
	}
	for _, w := range c.RateLimiter.Windows {
		if w.Type == "" {
			return fmt.Errorf("rate_limiter.windows[].type is required")
		}
		if w.Limit <= 0 {
			return fmt.Errorf("rate_limiter.windows[%s].limit must be > 0", w.Type)
		}
		d, err := time.ParseDuration(w.Duration)
		if err != nil || d <= 0 {
			return fmt.Errorf(
				"rate_limiter.windows[%s].duration must be valid duration (e.g. 1s, 1m, 1h)",
				w.Type,
			)
		}
	}
	return nil
}
