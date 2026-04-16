from collections import defaultdict
from datetime import datetime, timezone
import base64
import json
import os
from urllib import error, parse, request

from airflow import DAG
from airflow.operators.python import PythonOperator
from airflow.providers.postgres.hooks.postgres import PostgresHook


default_args = {
    "owner": "airflow",
    "start_date": datetime(2026, 4, 11),
}

CRM_SELECT_SQL = """
SELECT
    username,
    email,
    first_name,
    last_name,
    prosthesis_id,
    prosthesis_type,
    market
FROM crm_users;
"""

TELEMETRY_SELECT_SQL = """
SELECT
    event_time,
    prosthesis_id,
    username,
    signal_rms,
    signal_noise,
    gesture_predicted,
    response_time_ms,
    battery_level
FROM telemetry_events;
"""

CLICKHOUSE_URL = os.getenv("CLICKHOUSE_URL", "http://clickhouse:8123").rstrip("/")
CLICKHOUSE_DATABASE = os.getenv("CLICKHOUSE_DATABASE", "reports")
CLICKHOUSE_USER = os.getenv("CLICKHOUSE_USER", "")
CLICKHOUSE_PASSWORD = os.getenv("CLICKHOUSE_PASSWORD", "")

CREATE_DATABASE_SQL = f"CREATE DATABASE IF NOT EXISTS {CLICKHOUSE_DATABASE};"

CREATE_TABLE_SQL = f"""
CREATE TABLE IF NOT EXISTS {CLICKHOUSE_DATABASE}.user_prosthesis_telemetry (
    username String,
    first_name String,
    last_name String,
    prosthesis_id String,
    prosthesis_type String,
    market String,
    telemetry_records_count UInt32,
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
"""

INSERT_SQL = f"""
INSERT INTO {CLICKHOUSE_DATABASE}.user_prosthesis_telemetry (
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
) FORMAT JSONEachRow
"""


def _load_crm_rows():
    crm_rows = {}
    hook = PostgresHook(postgres_conn_id="crm_db")
    for row in hook.get_records(CRM_SELECT_SQL):
        crm_rows[(row[0], row[4])] = {
            "username": row[0],
            "email": row[1],
            "first_name": row[2],
            "last_name": row[3],
            "prosthesis_id": row[4],
            "prosthesis_type": row[5],
            "market": row[6],
        }
    return crm_rows


def _build_report_rows():
    crm_rows = _load_crm_rows()
    grouped = defaultdict(
        lambda: {
            "telemetry_records_count": 0,
            "first_event_at": None,
            "last_event_at": None,
            "signal_rms_total": 0.0,
            "signal_noise_total": 0.0,
            "response_time_total": 0.0,
            "battery_level_total": 0.0,
            "last_battery_level": None,
            "last_battery_event_at": None,
        }
    )

    hook = PostgresHook(postgres_conn_id="telemetry_db")
    for row in hook.get_records(TELEMETRY_SELECT_SQL):
        event_time = row[0]
        key = (row[2], row[1])
        crm_row = crm_rows.get(key)
        if crm_row is None:
            raise ValueError(
                f"CRM record not found for username={row[2]} prosthesis_id={row[1]}"
            )

        stats = grouped[key]
        stats["telemetry_records_count"] += 1
        stats["signal_rms_total"] += float(row[3])
        stats["signal_noise_total"] += float(row[4])
        stats["response_time_total"] += float(row[6])
        stats["battery_level_total"] += float(row[7])

        if stats["first_event_at"] is None or event_time < stats["first_event_at"]:
            stats["first_event_at"] = event_time

        if stats["last_event_at"] is None or event_time > stats["last_event_at"]:
            stats["last_event_at"] = event_time

        if stats["last_battery_event_at"] is None or event_time >= stats["last_battery_event_at"]:
            stats["last_battery_event_at"] = event_time
            stats["last_battery_level"] = int(row[7])

    report_rows = []
    for key, stats in grouped.items():
        username, prosthesis_id = key
        crm_row = crm_rows[key]
        count = stats["telemetry_records_count"]
        report_rows.append(
            (
                username,
                crm_row["first_name"],
                crm_row["last_name"],
                prosthesis_id,
                crm_row["prosthesis_type"],
                crm_row["market"],
                count,
                stats["first_event_at"],
                stats["last_event_at"],
                round(stats["signal_rms_total"] / count, 4),
                round(stats["signal_noise_total"] / count, 4),
                round(stats["response_time_total"] / count, 2),
                round(stats["battery_level_total"] / count, 2),
                stats["last_battery_level"],
            )
        )

    return report_rows


def _format_datetime(value):
    return value.astimezone(timezone.utc).strftime("%Y-%m-%d %H:%M:%S.%f")[:-3]


def _clickhouse_request(query, payload=""):
    req = request.Request(
        url=f"{CLICKHOUSE_URL}/?query={parse.quote(query)}",
        data=payload.encode("utf-8"),
        method="POST",
    )
    req.add_header("Content-Type", "application/json; charset=utf-8")
    if CLICKHOUSE_USER:
        credentials = f"{CLICKHOUSE_USER}:{CLICKHOUSE_PASSWORD}".encode("utf-8")
        req.add_header(
            "Authorization",
            f"Basic {base64.b64encode(credentials).decode('ascii')}",
        )

    try:
        with request.urlopen(req, timeout=30) as response:
            response.read()
    except error.HTTPError as exc:
        details = exc.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"ClickHouse query failed: {details}") from exc


def load_user_prosthesis_telemetry():
    report_rows = _build_report_rows()
    if not report_rows:
        raise ValueError("No telemetry rows found for loading")
    updated_at = _format_datetime(datetime.now(timezone.utc))
    payload = "\n".join(
        json.dumps(
            {
                "username": row[0],
                "first_name": row[1],
                "last_name": row[2],
                "prosthesis_id": row[3],
                "prosthesis_type": row[4],
                "market": row[5],
                "telemetry_records_count": row[6],
                "first_event_at": _format_datetime(row[7]),
                "last_event_at": _format_datetime(row[8]),
                "avg_signal_rms": row[9],
                "avg_signal_noise": row[10],
                "avg_response_time_ms": row[11],
                "avg_battery_level": row[12],
                "last_battery_level": row[13],
                "updated_at": updated_at,
            },
            ensure_ascii=True,
        )
        for row in report_rows
    )

    _clickhouse_request(CREATE_DATABASE_SQL)
    _clickhouse_request(CREATE_TABLE_SQL)
    _clickhouse_request(INSERT_SQL, payload)


with DAG(
    dag_id="user_prosthesis_telemetry_dag",
    default_args=default_args,
    schedule_interval="*/10 * * * *",
    catchup=False,
    max_active_runs=1,
    tags=["rag", "telemetry", "reports"],
) as dag:
    load_telemetry_report = PythonOperator(
        task_id="load_user_prosthesis_telemetry",
        python_callable=load_user_prosthesis_telemetry,
    )
