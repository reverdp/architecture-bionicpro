package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr       string
	PublicBaseURL    string
	FrontendURL      string
	InternalSecret   string
	SessionCookie    string
	SessionTTL       time.Duration
	EncryptionKey    []byte
	KeycloakPublic   string
	KeycloakInternal string
	Realm            string
	ClientID         string
	ClientSecret     string
	RedirectURL      string
	Scopes           string
	DefaultIDPAlias  string
	ProfileDBPath    string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:       getEnv("AUTH_LISTEN_ADDR", ":8001"),
		PublicBaseURL:    getEnv("AUTH_PUBLIC_BASE_URL", "http://localhost:8001"),
		FrontendURL:      getEnv("AUTH_FRONTEND_URL", "http://localhost:3000"),
		InternalSecret:   getEnv("AUTH_INTERNAL_SECRET", ""),
		SessionCookie:    getEnv("AUTH_SESSION_COOKIE_NAME", "bionicpro_auth_session"),
		SessionTTL:       getEnvDuration("AUTH_SESSION_TTL", 25*time.Minute),
		KeycloakPublic:   getEnv("KEYCLOAK_PUBLIC_URL", "http://localhost:8080"),
		KeycloakInternal: getEnv("KEYCLOAK_INTERNAL_URL", "http://keycloak:8080"),
		Realm:            getEnv("KEYCLOAK_REALM", "reports-realm"),
		ClientID:         getEnv("KEYCLOAK_CLIENT_ID", "bionicpro-auth"),
		ClientSecret:     getEnv("KEYCLOAK_CLIENT_SECRET", ""),
		Scopes:           getEnv("KEYCLOAK_SCOPES", "openid profile email"),
		DefaultIDPAlias:  getEnv("KEYCLOAK_DEFAULT_IDP_ALIAS", ""),
		ProfileDBPath:    getEnv("AUTH_PROFILE_DB_PATH", "data/bionicpro-auth.db"),
	}
	cfg.RedirectURL = getEnv("KEYCLOAK_REDIRECT_URL", fmt.Sprintf("%s/auth/callback", cfg.PublicBaseURL))

	key, generated, err := getEncryptionKey()
	if err != nil {
		return Config{}, err
	}
	if generated {
		fmt.Fprintf(os.Stderr, "AUTH_ENCRYPTION_KEY is not set; generated ephemeral in-memory key\n")
	}
	cfg.EncryptionKey = key
	if cfg.InternalSecret == "" {
		return Config{}, fmt.Errorf("AUTH_INTERNAL_SECRET is required")
	}

	return cfg, nil
}

func getEncryptionKey() ([]byte, bool, error) {
	raw := os.Getenv("AUTH_ENCRYPTION_KEY")
	if raw == "" {
		return nil, true, fmt.Errorf("empty encryption key")
	}

	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, false, fmt.Errorf("decode AUTH_ENCRYPTION_KEY: %w", err)
	}

	return key, false, nil
}

func getEnv(key, dft string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return dft
}

func getEnvBool(key string, dft bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return dft
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return dft
	}
	return parsed
}

func getEnvDuration(key string, dft time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return dft
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return dft
	}
	return parsed
}
