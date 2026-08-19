#!/usr/bin/env python3
"""Measure whether this Compose build can host P2-13 (`pre_start`, create_host_path).

`pre_start` shipped in Compose 5.4.0. This host's `docker compose version`
is recorded, not guessed. `create_host_path: false` is probed with a
disposable project: missing file AND missing directory must be refused
without creating an empty directory.

Usage:
  python3 scripts/compose-feature-gate.py --gate-report
  python3 scripts/compose-feature-gate.py --live --write-report PATH
--gate-report loads the committed JSON and exits 0 only when
pre_start.ok, create_host_path_false.ok, noop=false, live is not null,
and compose_version_tuple >= 5.4.0. Live probes need Docker; --live is
off by default so CI does not pull images.
"""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path

PRE_START_MIN = (5, 4, 0)
VERSION_RE = re.compile(r"v?(\d+)\.(\d+)\.(\d+)")
COMMITTED_REPORT = Path(__file__).resolve().parent / "compose-feature-gate.report.json"


def parse_compose_version(text: str) -> tuple[int, int, int]:
    match = VERSION_RE.search(text)
    if match is None:
        raise ValueError(f"cannot parse compose version from {text!r}")
    return int(match.group(1)), int(match.group(2)), int(match.group(3))


def pre_start_supported(version: tuple[int, int, int]) -> bool:
    return version >= PRE_START_MIN


def committed_report_violations(report: object) -> list[str]:
    """Return why a committed --gate-report JSON must not exit 0.

    Live probes are skipped in CI; this is the only evidence that
    pre_start and create_host_path:false actually work. A report
    produced with live=None, ok=false, noop=true, or Compose <5.4
    is refused.
    """
    if not isinstance(report, dict):
        return ["report is not a JSON object"]
    found: list[str] = []
    raw_tuple = report.get("compose_version_tuple")
    if not isinstance(raw_tuple, list) or len(raw_tuple) != 3:
        found.append("compose_version_tuple missing")
    else:
        try:
            version = (int(raw_tuple[0]), int(raw_tuple[1]), int(raw_tuple[2]))
        except (TypeError, ValueError):
            found.append("compose_version_tuple is not three integers")
        else:
            if version < PRE_START_MIN:
                found.append(
                    f"compose_version_tuple {list(version)} is below 5.4.0"
                )
    pre = report.get("pre_start")
    if not isinstance(pre, dict):
        found.append("pre_start missing")
    else:
        if pre.get("ok") is not True:
            found.append("pre_start.ok is not true")
        live = pre.get("live")
        if live is None:
            found.append("pre_start.live is null (unprobed); refuse --gate-report")
        elif not isinstance(live, dict):
            found.append("pre_start.live is not an object")
        elif live.get("supported") is not True:
            found.append("pre_start.live.supported is not true")
    chp = report.get("create_host_path_false")
    if not isinstance(chp, dict):
        found.append("create_host_path_false missing")
    else:
        if chp.get("ok") is not True:
            found.append("create_host_path_false.ok is not true")
        if chp.get("noop") is not False:
            found.append("create_host_path_false.noop is not false")
        for key in ("missing_file", "missing_directory"):
            if not isinstance(chp.get(key), dict):
                found.append(
                    f"create_host_path_false.{key} missing (unprobed live=null)"
                )
    return found


def gate_report(
    version_text: str,
    live: dict | None = None,
) -> dict:
    version = parse_compose_version(version_text)
    by_version = pre_start_supported(version)
    pre_live = (live or {}).get("pre_start_up_no_deps") if live else None
    if live is None:
        pre_ok = False
        pre_detail = (
            None
            if by_version
            else (
                f"Compose {version_text} is below 5.4.0; `pre_start` is "
                "rejected at validation time"
            )
        )
        # Without a live probe, do not claim create_host_path works.
        chp: dict = {
            "ok": False,
            "noop": True,
            "detail": "not probed on this run",
        }
    else:
        pre_ok = by_version and bool((pre_live or {}).get("supported"))
        pre_detail = (pre_live or {}).get("detail")
        file_p = live.get("create_host_path_false_missing_file") or {}
        dir_p = live.get("create_host_path_false_missing_directory") or {}
        refuses = bool(file_p.get("refuses")) and bool(dir_p.get("refuses"))
        created = bool(file_p.get("created_empty_dir")) or bool(
            dir_p.get("created_empty_dir")
        )
        chp = {
            "ok": refuses and not created,
            "noop": (not refuses) or created,
            "missing_file": file_p,
            "missing_directory": dir_p,
        }

    return {
        "compose_version": version_text,
        "compose_version_tuple": list(version),
        "pre_start": {
            "min_version": list(PRE_START_MIN),
            "ok": pre_ok,
            "supported_by_version": by_version,
            "live": pre_live,
            "detail": pre_detail,
        },
        "create_host_path_false": chp,
    }


def _compose_version_text() -> str:
    proc = subprocess.run(
        ["docker", "compose", "version"],
        capture_output=True,
        text=True,
        check=False,
    )
    if proc.returncode != 0:
        raise SystemExit(proc.stderr or proc.stdout or "docker compose version failed")
    return (proc.stdout or proc.stderr).strip()


def _run_compose(project: str, compose_file: Path, *args: str) -> subprocess.CompletedProcess[str]:
    return subprocess.run(
        ["docker", "compose", "-p", project, "-f", str(compose_file), *args],
        capture_output=True,
        text=True,
        check=False,
    )


def _probe_pre_start(tmpdir: Path) -> dict:
    compose = tmpdir / "pre_start.yaml"
    compose.write_text(
        """\
name: alt-wave0-prestart
services:
  probe:
    image: alpine:3.21
    command: ["sleep", "4"]
    pre_start:
      - command: ["echo", "pre_start-ran"]
""",
        encoding="utf-8",
    )
    up = _run_compose(
        "alt-wave0-prestart",
        compose,
        "up",
        "-d",
        "--no-deps",
        "--force-recreate",
        "probe",
    )
    _run_compose("alt-wave0-prestart", compose, "down", "-v", "--remove-orphans")
    err = (up.stderr or up.stdout or "").strip()
    supported = up.returncode == 0 and "not allowed" not in err
    detail = err.splitlines()[-1] if err else "up succeeded"
    # Drop the throwaway project path so a committed report has no host paths.
    detail = detail.replace(str(tmpdir), "<tmpdir>")
    return {
        "supported": supported,
        "exit_code": up.returncode,
        "detail": detail,
    }


def _probe_create_host_path(tmpdir: Path, kind: str) -> dict:
    project = f"alt-wave0-chp-{kind}"
    missing = tmpdir / f"missing-{kind}" / (
        "missing.conf" if kind == "file" else "missing-dir"
    )
    missing.parent.mkdir(parents=True, exist_ok=True)
    if missing.exists():
        if missing.is_dir():
            missing.rmdir()
        else:
            missing.unlink()
    target = "/mnt/missing.conf" if kind == "file" else "/mnt/missing-dir"
    compose = tmpdir / f"chp-{kind}.yaml"
    compose.write_text(
        f"""\
name: {project}
services:
  probe:
    image: alpine:3.21
    command: ["sleep", "2"]
    volumes:
      - type: bind
        source: {missing}
        target: {target}
        bind:
          create_host_path: false
""",
        encoding="utf-8",
    )
    up = _run_compose(project, compose, "up", "-d", "--force-recreate", "probe")
    _run_compose(project, compose, "down", "-v", "--remove-orphans")
    created_dir = missing.is_dir()
    exists = missing.exists()
    if exists:
        if created_dir:
            missing.rmdir()
        else:
            missing.unlink()
    refuses = up.returncode != 0 and not exists
    err = (up.stderr or up.stdout or "").strip()
    detail = err.splitlines()[-1] if err else ""
    detail = detail.replace(str(tmpdir), "<tmpdir>")
    return {
        "refuses": refuses,
        "created_empty_dir": created_dir,
        "path_exists_after": exists,
        "exit_code": up.returncode,
        "detail": detail,
    }


def run_live_probes() -> dict:
    with tempfile.TemporaryDirectory(prefix="alt-wave0-gate-") as raw:
        tmpdir = Path(raw)
        return {
            "pre_start_up_no_deps": _probe_pre_start(tmpdir),
            "create_host_path_false_missing_file": _probe_create_host_path(
                tmpdir, "file"
            ),
            "create_host_path_false_missing_directory": _probe_create_host_path(
                tmpdir, "dir"
            ),
        }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--gate-report", action="store_true", default=True)
    parser.add_argument("--live", action="store_true")
    parser.add_argument("--write-report", type=Path)
    parser.add_argument("--committed-report", default=str(COMMITTED_REPORT))
    args = parser.parse_args()

    if args.live:
        version_text = _compose_version_text()
        live = run_live_probes()
        report = gate_report(version_text, live)
        text = json.dumps(report, indent=2) + "\n"
        dest = args.write_report or COMMITTED_REPORT
        dest.write_text(text, encoding="utf-8")
        sys.stdout.write(text)
        found = committed_report_violations(report)
        if found:
            sys.stderr.write("compose-feature-gate live probe FAILED:\n")
            for item in found:
                sys.stderr.write(f"  - {item}\n")
            return 1
        return 0

    path = Path(args.committed_report)
    if not path.is_file():
        sys.stderr.write(
            f"missing committed gate report: {path}\n"
            "Re-run with --live --write-report on the operator host.\n"
        )
        return 2
    raw = path.read_text(encoding="utf-8")
    try:
        report = json.loads(raw)
    except json.JSONDecodeError as exc:
        sys.stderr.write(f"invalid committed gate report JSON: {exc}\n")
        return 1
    sys.stdout.write(raw if raw.endswith("\n") else raw + "\n")
    found = committed_report_violations(report)
    if found:
        sys.stderr.write("compose-feature-gate --gate-report FAILED:\n")
        for item in found:
            sys.stderr.write(f"  - {item}\n")
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
