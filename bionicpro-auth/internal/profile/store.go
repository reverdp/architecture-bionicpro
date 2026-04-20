package profile

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type YandexProfile struct {
	ID             string          `json:"id"`
	Username       string          `json:"username"`
	DisplayName    string          `json:"display_name"`
	FirstName      string          `json:"first_name"`
	LastName       string          `json:"last_name"`
	Email          string          `json:"email"`
	EmailVerified  bool            `json:"email_verified"`
	Scope          string          `json:"scope"`
	Audience       json.RawMessage `json:"audience"`
	AllowedOrigins []string        `json:"allowed_origins"`
	RawJSON        json.RawMessage `json:"-"`
}

type Record struct {
	Subject   string        `json:"subject"`
	Provider  string        `json:"provider"`
	Profile   YandexProfile `json:"profile"`
	UpdatedAt time.Time     `json:"updated_at"`
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create profile db dir: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open profile db: %w", err)
	}

	store := &Store{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SaveYandexProfile(ctx context.Context, subject, provider string, profile YandexProfile) error {
	audienceJSON := profile.Audience
	if len(audienceJSON) == 0 {
		audienceJSON = []byte("null")
	}

	raw := profile.RawJSON
	var err error
	if len(raw) == 0 {
		raw, err = json.Marshal(profile)
		if err != nil {
			return fmt.Errorf("marshal profile: %w", err)
		}
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO yandex_profiles (
			subject, provider, external_id, username, display_name, first_name, last_name,
			email, email_verified, scope, audience_json, allowed_origins_json, raw_json, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(subject, provider) DO UPDATE SET
			external_id = excluded.external_id,
			username = excluded.username,
			display_name = excluded.display_name,
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			email = excluded.email,
			email_verified = excluded.email_verified,
			scope = excluded.scope,
			audience_json = excluded.audience_json,
			allowed_origins_json = excluded.allowed_origins_json,
			raw_json = excluded.raw_json,
			updated_at = CURRENT_TIMESTAMP
	`, subject, provider, profile.ID, profile.Username, profile.DisplayName,
		profile.FirstName, profile.LastName, profile.Email, profile.EmailVerified, profile.Scope,
		string(audienceJSON), string(profile.AllowedOriginsJSON()), string(raw))
	if err != nil {
		return fmt.Errorf("save yandex profile: %w", err)
	}

	return nil
}

func (s *Store) GetBySubject(ctx context.Context, subject string) (*Record, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT subject, provider, external_id, username, display_name, first_name, last_name,
		       email, email_verified, scope, audience_json, allowed_origins_json, raw_json, updated_at
		FROM yandex_profiles
		WHERE subject = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, subject)

	var rec Record
	var audienceJSON string
	var allowedOriginsJSON string
	var rawJSON string
	if err := row.Scan(
		&rec.Subject,
		&rec.Provider,
		&rec.Profile.ID,
		&rec.Profile.Username,
		&rec.Profile.DisplayName,
		&rec.Profile.FirstName,
		&rec.Profile.LastName,
		&rec.Profile.Email,
		&rec.Profile.EmailVerified,
		&rec.Profile.Scope,
		&audienceJSON,
		&allowedOriginsJSON,
		&rawJSON,
		&rec.UpdatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get yandex profile: %w", err)
	}

	if audienceJSON != "" {
		rec.Profile.Audience = json.RawMessage(audienceJSON)
	}
	if allowedOriginsJSON != "" {
		if err := json.Unmarshal([]byte(allowedOriginsJSON), &rec.Profile.AllowedOrigins); err != nil {
			return nil, fmt.Errorf("decode allowed origins: %w", err)
		}
	}
	rec.Profile.RawJSON = json.RawMessage(rawJSON)

	return &rec, nil
}

func (s *Store) init(ctx context.Context) error {
	if err := s.ensureSchema(ctx); err != nil {
		return err
	}

	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS yandex_profiles (
			subject TEXT NOT NULL,
			provider TEXT NOT NULL,
			external_id TEXT NOT NULL,
			username TEXT,
			display_name TEXT,
			first_name TEXT,
			last_name TEXT,
			email TEXT,
			email_verified BOOLEAN NOT NULL DEFAULT 0,
			scope TEXT,
			audience_json TEXT NOT NULL,
			allowed_origins_json TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY(subject, provider)
		)
	`)
	if err != nil {
		return fmt.Errorf("migrate profile db: %w", err)
	}

	return nil
}

func (s *Store) ensureSchema(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(yandex_profiles)`)
	if err != nil {
		return fmt.Errorf("inspect profile schema: %w", err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan profile schema: %w", err)
		}
		columns[name] = true
	}

	if len(columns) == 0 {
		return nil
	}

	required := []string{
		"external_id",
		"username",
		"display_name",
		"first_name",
		"last_name",
		"email",
		"email_verified",
		"scope",
		"audience_json",
		"allowed_origins_json",
		"raw_json",
	}
	for _, column := range required {
		if !columns[column] {
			if _, err := s.db.ExecContext(ctx, `DROP TABLE IF EXISTS yandex_profiles`); err != nil {
				return fmt.Errorf("drop incompatible profile schema: %w", err)
			}
			return nil
		}
	}

	return nil
}

func (p YandexProfile) AllowedOriginsJSON() []byte {
	raw, _ := json.Marshal(p.AllowedOrigins)
	if raw == nil {
		return []byte("[]")
	}
	return raw
}
