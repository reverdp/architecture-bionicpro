package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	ListenAddr         string
	FrontendURL        string
	ReportDBDSN        string
	AuthInternalURL    string
	AuthSessionCookie  string
	AuthInternalSecret string
	HTTPTimeout        time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:         getEnv("API_LISTEN_ADDR", ":8000"),
		FrontendURL:        getEnv("API_FRONTEND_URL", "http://localhost:3000"),
		ReportDBDSN:        getEnv("REPORTS_DB_DSN", ""),
		AuthInternalURL:    getEnv("AUTH_INTERNAL_URL", "http://bionicpro-auth:8001"),
		AuthSessionCookie:  getEnv("AUTH_SESSION_COOKIE_NAME", "bionicpro_auth_session"),
		AuthInternalSecret: getEnv("AUTH_INTERNAL_SECRET", ""),
		HTTPTimeout:        10 * time.Second,
	}

	if cfg.ReportDBDSN == "" {
		return Config{}, fmt.Errorf("REPORTS_DB_DSN is required")
	}
	if cfg.AuthInternalSecret == "" {
		return Config{}, fmt.Errorf("AUTH_INTERNAL_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, dft string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return dft
}
