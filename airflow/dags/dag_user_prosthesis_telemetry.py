import csv
from collections import defaultdict
from datetime import datetime

from airflow import DAG
from airflow.operators.python import PythonOperator
from airflow.providers.postgres.hooks.postgres import PostgresHook


default_args = {
    "owner": "airflow",
    "start_date": datetime(2026, 4, 11),
}

CRM_FILE_PATH = "/opt/airflow/source_data/crm.csv"
TELEMETRY_FILE_PATH = "/opt/airflow/source_data/telemetry.csv"

CREATE_TABLE_SQL = """
CREATE TABLE IF NOT EXISTS user_prosthesis_telemetry (
    username TEXT NOT NULL,
    first_name TEXT NOT NULL,
    last_name TEXT NOT NULL,
    prosthesis_id TEXT NOT NULL,
    prosthesis_type TEXT NOT NULL,
    market TEXT NOT NULL,
    telemetry_records_count INTEGER NOT NULL,
    first_event_at TIMESTAMPTZ NOT NULL,
    last_event_at TIMESTAMPTZ NOT NULL,
    avg_signal_rms NUMERIC(6,4) NOT NULL,
    avg_signal_noise NUMERIC(6,4) NOT NULL,
    avg_response_time_ms NUMERIC(8,2) NOT NULL,
    avg_battery_level NUMERIC(6,2) NOT NULL,
    last_battery_level INTEGER NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_prosthesis_telemetry_username
    ON user_prosthesis_telemetry (username);

CREATE INDEX IF NOT EXISTS idx_user_prosthesis_telemetry_prosthesis_id
    ON user_prosthesis_telemetry (prosthesis_id);
"""

INSERT_SQL = """
INSERT INTO user_prosthesis_telemetry (
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
)
VALUES (
    %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, NOW()
);
"""


def _parse_timestamp(value):
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def _load_crm_rows():
    crm_rows = {}
    with open(CRM_FILE_PATH, newline="", encoding="utf-8") as csvfile:
        reader = csv.DictReader(csvfile)
        for row in reader:
            crm_rows[(row["username"], row["prosthesis_id"])] = row
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

    with open(TELEMETRY_FILE_PATH, newline="", encoding="utf-8") as csvfile:
        reader = csv.DictReader(csvfile)
        for row in reader:
            event_time = _parse_timestamp(row["event_time"])
            key = (row["username"], row["prosthesis_id"])
            crm_row = crm_rows.get(key)
            if crm_row is None:
                raise ValueError(
                    f"CRM record not found for username={row['username']} prosthesis_id={row['prosthesis_id']}"
                )

            stats = grouped[key]
            stats["telemetry_records_count"] += 1
            stats["signal_rms_total"] += float(row["signal_rms"])
            stats["signal_noise_total"] += float(row["signal_noise"])
            stats["response_time_total"] += float(row["response_time_ms"])
            stats["battery_level_total"] += float(row["battery_level"])

            if stats["first_event_at"] is None or event_time < stats["first_event_at"]:
                stats["first_event_at"] = event_time

            if stats["last_event_at"] is None or event_time > stats["last_event_at"]:
                stats["last_event_at"] = event_time

            if stats["last_battery_event_at"] is None or event_time >= stats["last_battery_event_at"]:
                stats["last_battery_event_at"] = event_time
                stats["last_battery_level"] = int(row["battery_level"])

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


def load_user_prosthesis_telemetry():
    report_rows = _build_report_rows()
    if not report_rows:
        raise ValueError("No telemetry rows found for loading")

    hook = PostgresHook(postgres_conn_id="report_db")
    connection = hook.get_conn()

    try:
        with connection:
            with connection.cursor() as cursor:
                cursor.execute(CREATE_TABLE_SQL)
                cursor.execute(DROP_CONSTRAINT_SQL)
                cursor.executemany(INSERT_SQL, report_rows)
    finally:
        connection.close()


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
