package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	ListenAddr                string
	FrontendURL               string
	ReportsClickHouseURL      string
	ReportsClickHouseUser     string
	ReportsClickHousePassword string
	AuthInternalURL           string
	AuthSessionCookie         string
	AuthInternalSecret        string
	S3Endpoint                string
	S3Region                  string
	S3Bucket                  string
	S3AccessKey               string
	S3SecretKey               string
	CDNBaseURL                string
	HTTPTimeout               time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:                getEnv("API_LISTEN_ADDR", ":8000"),
		FrontendURL:               getEnv("API_FRONTEND_URL", "http://localhost:3000"),
		ReportsClickHouseURL:      getEnv("REPORTS_CLICKHOUSE_URL", ""),
		ReportsClickHouseUser:     getEnv("REPORTS_CLICKHOUSE_USER", ""),
		ReportsClickHousePassword: getEnv("REPORTS_CLICKHOUSE_PASSWORD", ""),
		AuthInternalURL:           getEnv("AUTH_INTERNAL_URL", "http://bionicpro-auth:8001"),
		AuthSessionCookie:         getEnv("AUTH_SESSION_COOKIE_NAME", "bionicpro_auth_session"),
		AuthInternalSecret:        getEnv("AUTH_INTERNAL_SECRET", ""),
		S3Endpoint:                getEnv("S3_ENDPOINT", "http://minio:9000"),
		S3Region:                  getEnv("S3_REGION", "us-east-1"),
		S3Bucket:                  getEnv("S3_BUCKET", "reports"),
		S3AccessKey:               getEnv("S3_ACCESS_KEY", ""),
		S3SecretKey:               getEnv("S3_SECRET_KEY", ""),
		CDNBaseURL:                getEnv("CDN_BASE_URL", "http://localhost:9002"),
		HTTPTimeout:               10 * time.Second,
	}

	if cfg.ReportsClickHouseURL == "" {
		return Config{}, fmt.Errorf("REPORTS_CLICKHOUSE_URL is required")
	}
	if cfg.ReportsClickHouseUser == "" {
		return Config{}, fmt.Errorf("REPORTS_CLICKHOUSE_USER is required")
	}
	if cfg.ReportsClickHousePassword == "" {
		return Config{}, fmt.Errorf("REPORTS_CLICKHOUSE_PASSWORD is required")
	}
	if cfg.AuthInternalSecret == "" {
		return Config{}, fmt.Errorf("AUTH_INTERNAL_SECRET is required")
	}
	if cfg.S3AccessKey == "" {
		return Config{}, fmt.Errorf("S3_ACCESS_KEY is required")
	}
	if cfg.S3SecretKey == "" {
		return Config{}, fmt.Errorf("S3_SECRET_KEY is required")
	}
	if cfg.CDNBaseURL == "" {
		return Config{}, fmt.Errorf("CDN_BASE_URL is required")
	}

	return cfg, nil
}

func getEnv(key, dft string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return dft
}
