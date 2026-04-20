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

TRUNCATE TABLE telemetry_events;

INSERT INTO telemetry_events (
    event_time,
    prosthesis_id,
    username,
    signal_rms,
    signal_noise,
    gesture_predicted,
    response_time_ms,
    battery_level
)
VALUES
    (CURRENT_DATE + TIME '09:00:00', 'BP-RU-001', 'prothetic1', 0.82, 0.09, 'open_hand', 74, 96),
    (CURRENT_DATE + TIME '09:05:00', 'BP-RU-001', 'prothetic1', 0.91, 0.07, 'close_hand', 68, 95),
    (CURRENT_DATE + TIME '09:10:00', 'BP-RU-001', 'prothetic1', 0.77, 0.11, 'pinch', 83, 94),
    (CURRENT_DATE + TIME '09:15:00', 'BP-RU-001', 'prothetic1', 0.88, 0.08, 'wrist_rotate', 79, 93),
    (CURRENT_DATE + TIME '09:20:00', 'BP-RU-001', 'prothetic1', 0.80, 0.10, 'rest', 65, 92),
    (CURRENT_DATE + TIME '10:00:00', 'BP-RU-002', 'prothetic2', 0.79, 0.12, 'open_hand', 86, 89),
    (CURRENT_DATE + TIME '10:05:00', 'BP-RU-002', 'prothetic2', 0.84, 0.10, 'close_hand', 90, 88),
    (CURRENT_DATE + TIME '10:10:00', 'BP-RU-002', 'prothetic2', 0.76, 0.13, 'pinch', 94, 87),
    (CURRENT_DATE + TIME '10:15:00', 'BP-RU-002', 'prothetic2', 0.81, 0.09, 'wrist_rotate', 88, 86),
    (CURRENT_DATE + TIME '10:20:00', 'BP-RU-002', 'prothetic2', 0.74, 0.14, 'rest', 92, 85),
    (CURRENT_DATE + TIME '11:00:00', 'BP-RU-003', 'prothetic3', 0.71, 0.16, 'open_hand', 98, 78),
    (CURRENT_DATE + TIME '11:05:00', 'BP-RU-003', 'prothetic3', 0.75, 0.15, 'close_hand', 101, 77),
    (CURRENT_DATE + TIME '11:10:00', 'BP-RU-003', 'prothetic3', 0.69, 0.18, 'pinch', 105, 76),
    (CURRENT_DATE + TIME '11:15:00', 'BP-RU-003', 'prothetic3', 0.73, 0.14, 'wrist_rotate', 99, 75),
    (CURRENT_DATE + TIME '11:20:00', 'BP-RU-003', 'prothetic3', 0.67, 0.19, 'rest', 97, 74);
