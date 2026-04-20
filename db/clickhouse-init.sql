CREATE DATABASE IF NOT EXISTS reports;

CREATE TABLE IF NOT EXISTS reports.user_prosthesis_telemetry (
    username String,
    prosthesis_id String,
    telemetry_records_count UInt64,
    first_event_at DateTime64(3, 'UTC'),
    last_event_at DateTime64(3, 'UTC'),
    avg_signal_rms Float64,
    avg_signal_noise Float64,
    avg_response_time_ms Float64,
    avg_battery_level Float64,
    last_battery_level UInt8,
    updated_at DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(updated_at)
ORDER BY (username, prosthesis_id);
