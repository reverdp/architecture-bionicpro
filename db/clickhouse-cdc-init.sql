CREATE TABLE IF NOT EXISTS reports.crm_users_cdc_queue (
    raw_message String
)
ENGINE = Kafka
SETTINGS
    kafka_broker_list = 'kafka:9092',
    kafka_topic_list = 'crm-cdc.public.crm_users',
    kafka_group_name = 'clickhouse-crm-users-cdc',
    kafka_format = 'JSONAsString',
    kafka_num_consumers = 1,
    kafka_handle_error_mode = 'stream';

CREATE TABLE IF NOT EXISTS reports.crm_users_cdc_log (
    username String,
    email String,
    first_name String,
    last_name String,
    prosthesis_id String,
    prosthesis_type String,
    market String,
    op LowCardinality(String),
    source_table LowCardinality(String),
    is_deleted UInt8,
    source_ts DateTime64(3, 'UTC'),
    ingested_at DateTime64(3, 'UTC')
)
ENGINE = MergeTree
ORDER BY (username, source_ts);

CREATE TABLE IF NOT EXISTS reports.crm_users_cdc_current (
    username String,
    email String,
    first_name String,
    last_name String,
    prosthesis_id String,
    prosthesis_type String,
    market String,
    is_deleted UInt8,
    source_ts DateTime64(3, 'UTC')
)
ENGINE = ReplacingMergeTree(source_ts)
ORDER BY username;

CREATE MATERIALIZED VIEW IF NOT EXISTS reports.mv_crm_users_cdc_log
TO reports.crm_users_cdc_log
AS
SELECT
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'username') AS username,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'email') AS email,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'first_name') AS first_name,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'last_name') AS last_name,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'prosthesis_id') AS prosthesis_id,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'prosthesis_type') AS prosthesis_type,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'market') AS market,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), '__op') AS op,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), '__table') AS source_table,
    if(JSONExtractString(JSONExtractRaw(raw_message, 'payload'), '__deleted') = 'true', 1, 0) AS is_deleted,
    if(
        isNull(JSONExtract(JSONExtractRaw(raw_message, 'payload'), '__source_ts_ms', 'Nullable(Int64)')),
        now64(3, 'UTC'),
        fromUnixTimestamp64Milli(JSONExtract(JSONExtractRaw(raw_message, 'payload'), '__source_ts_ms', 'Nullable(Int64)'), 'UTC')
    ) AS source_ts,
    now64(3, 'UTC') AS ingested_at
FROM reports.crm_users_cdc_queue
WHERE JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'username') != '';

CREATE MATERIALIZED VIEW IF NOT EXISTS reports.mv_crm_users_cdc_current
TO reports.crm_users_cdc_current
AS
SELECT
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'username') AS username,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'email') AS email,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'first_name') AS first_name,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'last_name') AS last_name,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'prosthesis_id') AS prosthesis_id,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'prosthesis_type') AS prosthesis_type,
    JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'market') AS market,
    if(JSONExtractString(JSONExtractRaw(raw_message, 'payload'), '__deleted') = 'true', 1, 0) AS is_deleted,
    if(
        isNull(JSONExtract(JSONExtractRaw(raw_message, 'payload'), '__source_ts_ms', 'Nullable(Int64)')),
        now64(3, 'UTC'),
        fromUnixTimestamp64Milli(JSONExtract(JSONExtractRaw(raw_message, 'payload'), '__source_ts_ms', 'Nullable(Int64)'), 'UTC')
    ) AS source_ts
FROM reports.crm_users_cdc_queue
WHERE JSONExtractString(JSONExtractRaw(raw_message, 'payload'), 'username') != '';
