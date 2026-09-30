"""Compare multiple Irodori-TTS benchmark result JSON files."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path


def load_result_file(path: Path) -> dict:
    """Load and validate benchmark result JSON file.

    Raises ValueError if required fields are missing.
    """
    text = path.read_text(encoding="utf-8")
    data = json.loads(text)
    if not isinstance(data, dict):
        raise TypeError(f"Expected JSON object in {path}, got {type(data).__name__}")
    if "label" not in data:
        data["label"] = path.stem
    if "rtf" not in data or not isinstance(data["rtf"], dict):
        raise ValueError(f"Missing required 'rtf' object in {path}")
    return data


def format_comparison_table(results: list[dict]) -> str:
    """Render an ASCII comparison table sorted by configuration label.

    Columns: Label, Budget Pass, Effective Peak (MiB)*, Peak Proc (MiB),
             Peak Reserved (MiB), PID Hits, RTF p50, RTF p95, Failures, Both Gates.
    """
    if not results:
        return "No results to compare."

    sorted_results = sorted(results, key=lambda r: str(r.get("label", "")))

    headers = [
        "Label",
        "Budget Pass",
        "Effective Peak (MiB)*",
        "Peak Proc (MiB)",
        "Peak Reserved (MiB)",
        "PID Hits",
        "RTF p50",
        "RTF p95",
        "Failures",
        "Both Gates",
    ]

    rows = []
    for r in sorted_results:
        label = str(r.get("label", "unknown"))
        b_pass = "PASS" if r.get("budget_pass") else "FAIL"

        eff_peak = r.get("effective_peak_mib")
        eff_str = f"{eff_peak:.1f}" if isinstance(eff_peak, (int, float)) else "N/A"

        peak_proc = r.get("peak_process_mib")
        proc_str = f"{peak_proc:.1f}" if isinstance(peak_proc, (int, float)) else "N/A"

        peak_res = r.get("peak_reserved_mib")
        res_str = f"{peak_res:.1f}" if isinstance(peak_res, (int, float)) else "N/A"

        pid_hits = r.get("pid_hits")
        pid_hits_str = str(pid_hits) if isinstance(pid_hits, int) else "N/A"

        rtf_dict = r.get("rtf", {})
        rtf_p50 = rtf_dict.get("p50")
        p50_str = f"{rtf_p50:.4f}" if isinstance(rtf_p50, (int, float)) else "N/A"

        rtf_p95 = rtf_dict.get("p95")
        p95_str = f"{rtf_p95:.4f}" if isinstance(rtf_p95, (int, float)) else "N/A"

        failures = r.get("failures", 0)
        fail_str = str(failures)

        both_pass = bool(r.get("all_passed"))
        gate_str = "PASS [OK]" if both_pass else "FAIL"

        rows.append(
            [
                label,
                b_pass,
                eff_str,
                proc_str,
                res_str,
                pid_hits_str,
                p50_str,
                p95_str,
                fail_str,
                gate_str,
            ]
        )

    # Determine column widths
    col_widths = [len(h) for h in headers]
    for row in rows:
        for idx, val in enumerate(row):
            col_widths[idx] = max(col_widths[idx], len(val))

    def make_row(cells: list[str]) -> str:
        padded = [
            cells[i].ljust(col_widths[i])
            if i in (0, 1, len(cells) - 1)
            else cells[i].rjust(col_widths[i])
            for i in range(len(cells))
        ]
        return "| " + " | ".join(padded) + " |"

    separator = "+-" + "-+-".join("-" * w for w in col_widths) + "-+"

    lines = [
        separator,
        make_row(headers),
        separator,
    ]
    for row in rows:
        lines.append(make_row(row))
    lines.append(separator)
    lines.append(
        "* Budget gate evaluates Effective Peak (MiB) <= limit"
        " (and failures == 0, pid_hits > 0)."
    )

    # Note summary of passed configs
    passed_labels = [row[0] for row in rows if row[-1] == "PASS [OK]"]
    lines.append("")
    if passed_labels:
        lines.append(f"Configurations meeting both gates: {', '.join(passed_labels)}")
    else:
        lines.append("No configurations met both gates.")

    return "\n".join(lines)


def parse_args(args=None):
    """Parse CLI arguments for compare.py."""
    parser = argparse.ArgumentParser(
        description="Compare multiple Irodori-TTS benchmark result JSON files."
    )
    parser.add_argument(
        "files",
        nargs="+",
        help=("Path to result JSON files (e.g. results/C1-*.json results/C2-*.json)"),
    )
    return parser.parse_args(args)


def main(args=None) -> int:
    """Entry point for compare.py."""
    parsed = parse_args(args)
    file_paths = parsed.files

    results = []
    for fp in file_paths:
        p = Path(fp)
        if not p.is_file():
            sys.stderr.write(f"Warning: File not found: {fp}\n")
            continue
        try:
            results.append(load_result_file(p))
        except (
            OSError,
            json.JSONDecodeError,
            UnicodeDecodeError,
            ValueError,
            TypeError,
        ) as e:
            sys.stderr.write(f"Warning: Failed to load {fp}: {e}\n")

    if not results:
        sys.stderr.write("Error: No valid benchmark result files could be loaded.\n")
        return 1

    table = format_comparison_table(results)
    print(table)
    return 0


if __name__ == "__main__":
    sys.exit(main())
