CREATE TABLE IF NOT EXISTS reports.user_prosthesis_telemetry_v2 (
    username String,
    first_name String,
    last_name String,
    prosthesis_id String,
    prosthesis_type String,
    market String,
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

CREATE MATERIALIZED VIEW IF NOT EXISTS reports.mv_user_prosthesis_telemetry_v2
REFRESH EVERY 1 MINUTE
TO reports.user_prosthesis_telemetry_v2
AS
SELECT
    telemetry.username AS username,
    crm.first_name AS first_name,
    crm.last_name AS last_name,
    telemetry.prosthesis_id AS prosthesis_id,
    crm.prosthesis_type AS prosthesis_type,
    crm.market AS market,
    telemetry.telemetry_records_count AS telemetry_records_count,
    telemetry.first_event_at AS first_event_at,
    telemetry.last_event_at AS last_event_at,
    telemetry.avg_signal_rms AS avg_signal_rms,
    telemetry.avg_signal_noise AS avg_signal_noise,
    telemetry.avg_response_time_ms AS avg_response_time_ms,
    telemetry.avg_battery_level AS avg_battery_level,
    telemetry.last_battery_level AS last_battery_level,
    now64(3, 'UTC') AS updated_at
FROM (
    SELECT
        username,
        prosthesis_id,
        telemetry_records_count,
        first_event_at,
        last_event_at,
        avg_signal_rms,
        avg_signal_noise,
        avg_response_time_ms,
        avg_battery_level,
        last_battery_level
    FROM reports.user_prosthesis_telemetry FINAL
) AS telemetry
INNER JOIN (
    SELECT
        username,
        first_name,
        last_name,
        prosthesis_id,
        prosthesis_type,
        market
    FROM reports.crm_users_cdc_current FINAL
    WHERE is_deleted = 0
) AS crm
    ON telemetry.username = crm.username
   AND telemetry.prosthesis_id = crm.prosthesis_id;
