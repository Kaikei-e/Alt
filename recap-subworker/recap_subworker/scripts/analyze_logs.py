import argparse
import re
import sqlite3
import sys
from datetime import UTC, datetime
from pathlib import Path

import pandas as pd


def parse_logs(log_file_path: str) -> pd.DataFrame:
    """
    Parse the log file to extract errors and timestamp.
    Focus on 'duplicate key value violates unique constraint' and general errors.
    """
    error_patterns = {
        "duplicate_key": r"duplicate key value violates unique constraint",
        "validation_error": r"ValidationError",
        "connection_error": r"Connection refused",
        "timeout": r"TimeoutError",
    }

    data = []

    # Heuristic regex for timestamp, adjust based on actual log format
    # Example: 2024-12-09 01:00:00 [info] ...
    # Or JSON logs. Assuming standard text logs for now as per "failure.log" mention.
    timestamp_pattern = r"^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})"

    try:
        with Path(log_file_path).open(encoding="utf-8") as f:
            for line in f:
                # Extract timestamp
                timestamp = None
                ts_match = re.search(timestamp_pattern, line)
                if ts_match:
                    timestamp = ts_match.group(1)

                # Check for errors
                for error_type, pattern in error_patterns.items():
                    if re.search(pattern, line):
                        data.append(
                            {
                                "timestamp": timestamp
                                if timestamp
                                else datetime.now(UTC).astimezone().strftime("%Y-%m-%d %H:%M:%S"),
                                "error_type": error_type,
                                "raw_line": line.strip()[:200],  # truncate
                            }
                        )
    except FileNotFoundError:
        sys.stdout.write(f"Log file not found: {log_file_path}\n")
        return pd.DataFrame()

    return pd.DataFrame(data)


def save_to_db(df: pd.DataFrame, db_path: str = "recap_logs.db") -> None:
    if df.empty:
        return

    conn = sqlite3.connect(db_path)
    df.to_sql("log_errors", conn, if_exists="append", index=False)
    conn.close()
    sys.stdout.write(f"Saved {len(df)} error records to {db_path}\n")


def main() -> None:
    parser = argparse.ArgumentParser(description="Analyze recap logs for errors.")
    parser.add_argument("log_file", help="Path to the log file")
    parser.add_argument(
        "--db-path", default="recap_logs.db", help="Path to SQLite DB for dashboard"
    )

    args = parser.parse_args()

    df = parse_logs(args.log_file)
    if not df.empty:
        sys.stdout.write(f"Found {len(df)} errors.\n")
        sys.stdout.write(f"{df['error_type'].value_counts()}\n")
        save_to_db(df, args.db_path)
    else:
        sys.stdout.write("No errors found or empty file.\n")


if __name__ == "__main__":
    main()
