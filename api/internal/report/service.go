package report

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Service struct {
	baseURL    string
	authHeader string
	client     *http.Client
}

type Query struct {
	Username string
	DateFrom time.Time
	DateTo   time.Time
}

func NewService(baseURL, user, password string, timeout time.Duration) *Service {
	credentials := base64.StdEncoding.EncodeToString([]byte(user + ":" + password))
	return &Service{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authHeader: "Basic " + credentials,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (s *Service) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/ping", nil)
	if err != nil {
		return fmt.Errorf("build clickhouse ping request: %w", err)
	}
	req.Header.Set("Authorization", s.authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("execute clickhouse ping request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("clickhouse ping failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return nil
}

func (s *Service) CSV(ctx context.Context, query Query) ([]byte, error) {
	sqlQuery := fmt.Sprintf(`
		SELECT
			username,
			first_name,
			last_name,
			prosthesis_id,
			prosthesis_type,
			market,
			telemetry_records_count,
			first_event_at,
			last_event_at,
			avg_signal_rms,
			avg_signal_noise,
			avg_response_time_ms,
			avg_battery_level,
			last_battery_level,
			updated_at
		FROM reports.user_prosthesis_telemetry
		WHERE username = '%s'
		  AND first_event_at <= toDateTime64('%s', 3, 'UTC')
		  AND last_event_at >= toDateTime64('%s', 3, 'UTC')
		ORDER BY prosthesis_id
		FORMAT CSVWithNames
		`,
		escapeString(query.Username),
		formatClickHouseDateTime(query.DateTo),
		formatClickHouseDateTime(query.DateFrom),
	)

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.baseURL+"/?query="+url.QueryEscape(sqlQuery),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("build clickhouse query request: %w", err)
	}
	req.Header.Set("Authorization", s.authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute clickhouse query request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read clickhouse response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clickhouse query failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func escapeString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func formatClickHouseDateTime(value time.Time) string {
	return value.UTC().Format("2006-01-02 15:04:05.000")
}
