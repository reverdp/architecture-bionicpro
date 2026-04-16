CREATE TABLE IF NOT EXISTS telemetry_events (
    event_time TIMESTAMPTZ NOT NULL,
    prosthesis_id TEXT NOT NULL,
    username TEXT NOT NULL,
    signal_rms NUMERIC(6,4) NOT NULL,
    signal_noise NUMERIC(6,4) NOT NULL,
    gesture_predicted TEXT NOT NULL,
    response_time_ms INTEGER NOT NULL,
    battery_level INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_telemetry_events_username_prosthesis
    ON telemetry_events (username, prosthesis_id);
