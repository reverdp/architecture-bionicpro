package report

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"time"
)

type Service struct {
	db *sql.DB
}

type Query struct {
	Username string
	DateFrom time.Time
	DateTo   time.Time
}

func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

func (s *Service) CSV(ctx context.Context, query Query) ([]byte, error) {
	const sqlQuery = `
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
FROM user_prosthesis_telemetry
WHERE username = $1
  AND first_event_at <= $3
  AND last_event_at >= $2
ORDER BY prosthesis_id
`

	rows, err := s.db.QueryContext(ctx, sqlQuery, query.Username, query.DateFrom, query.DateTo)
	if err != nil {
		return nil, fmt.Errorf("query report rows: %w", err)
	}
	defer rows.Close()

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write([]string{
		"username",
		"first_name",
		"last_name",
		"prosthesis_id",
		"prosthesis_type",
		"market",
		"telemetry_records_count",
		"first_event_at",
		"last_event_at",
		"avg_signal_rms",
		"avg_signal_noise",
		"avg_response_time_ms",
		"avg_battery_level",
		"last_battery_level",
		"updated_at",
	}); err != nil {
		return nil, fmt.Errorf("write csv header: %w", err)
	}

	for rows.Next() {
		var (
			username              string
			firstName             string
			lastName              string
			prosthesisID          string
			prosthesisType        string
			market                string
			telemetryRecordsCount int
			firstEventAt          time.Time
			lastEventAt           time.Time
			avgSignalRMS          string
			avgSignalNoise        string
			avgResponseTimeMS     string
			avgBatteryLevel       string
			lastBatteryLevel      int
			updatedAt             time.Time
		)

		if err := rows.Scan(
			&username,
			&firstName,
			&lastName,
			&prosthesisID,
			&prosthesisType,
			&market,
			&telemetryRecordsCount,
			&firstEventAt,
			&lastEventAt,
			&avgSignalRMS,
			&avgSignalNoise,
			&avgResponseTimeMS,
			&avgBatteryLevel,
			&lastBatteryLevel,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan report row: %w", err)
		}

		record := []string{
			username,
			firstName,
			lastName,
			prosthesisID,
			prosthesisType,
			market,
			fmt.Sprintf("%d", telemetryRecordsCount),
			firstEventAt.Format(time.RFC3339),
			lastEventAt.Format(time.RFC3339),
			avgSignalRMS,
			avgSignalNoise,
			avgResponseTimeMS,
			avgBatteryLevel,
			fmt.Sprintf("%d", lastBatteryLevel),
			updatedAt.Format(time.RFC3339),
		}
		if err := writer.Write(record); err != nil {
			return nil, fmt.Errorf("write csv row: %w", err)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report rows: %w", err)
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("flush csv: %w", err)
	}

	return buf.Bytes(), nil
}
