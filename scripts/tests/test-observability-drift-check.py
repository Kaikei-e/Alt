#!/usr/bin/env python3
"""Tests for the Alertmanager half of scripts/observability-drift-check.py.

The check used to compare the bytes of `observability/alertmanager/alertmanager.yml`
against the `config.original` string from `/api/v2/status`. Despite the name,
that string is not the file: Alertmanager parses the config and marshals it
back out, which drops every comment and fills in every unset global default.
The two can never be equal, so the check reported drift on every run — after a
successful reload, on a host with no drift at all. A check that is always red
is a check nobody reads, and it hid whether Alertmanager had really picked up
a change.

Alertmanager already publishes what is needed: `alertmanager_config_hash` is
derived from the raw bytes of the file it loaded, so hashing the file on disk
the same way answers "is the running config this file?" exactly.

Run:
    python3 scripts/tests/test-observability-drift-check.py
Exit 0 on green, non-zero on red.
"""

import importlib.util
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]

spec = importlib.util.spec_from_file_location(
    "drift_check", ROOT / "scripts" / "observability-drift-check.py"
)
assert spec is not None, "drift check script not importable"
assert spec.loader is not None, "drift check script not importable"
drift_check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(drift_check)


class _Results:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0


RESULTS = _Results()


def check(name: str, condition: bool) -> None:
    if condition:
        sys.stdout.write(f"  PASS  {name}\n")
        RESULTS.passed += 1
    else:
        sys.stdout.write(f"  FAIL  {name}\n")
        RESULTS.failed += 1


# Values produced by Alertmanager's own md5HashAsMetricValue: the first 6 bytes
# of the md5 digest read as a little-endian uint64. Pinned rather than
# recomputed, so a rewrite of the helper has something external to agree with.
SAMPLE = b"global:\n  resolve_timeout: 10m\n"
SAMPLE_HASH = 63657913343414.0
EMPTY_HASH = 617830161876.0


def metrics(hash_value: float, reload_ok: int = 1) -> str:
    """A slice of Alertmanager's /metrics, in the order it publishes them."""
    return (
        "# HELP alertmanager_config_hash Hash of the currently loaded config.\n"
        "# TYPE alertmanager_config_hash gauge\n"
        f"alertmanager_config_hash {hash_value}\n"
        "# TYPE alertmanager_config_last_reload_successful gauge\n"
        f"alertmanager_config_last_reload_successful {reload_ok}\n"
    )


sys.stdout.write("[hash] reproduces Alertmanager's md5HashAsMetricValue\n")
check(
    "hashes a sample config to the value Alertmanager publishes",
    drift_check.alertmanager_config_hash(SAMPLE) == SAMPLE_HASH,
)
check(
    "hashes an empty file without special-casing it",
    drift_check.alertmanager_config_hash(b"") == EMPTY_HASH,
)
check(
    "a one-byte change changes the hash",
    drift_check.alertmanager_config_hash(SAMPLE + b" ") != SAMPLE_HASH,
)

sys.stdout.write("\n[drift] the loaded config is compared by hash, not by text\n")
check(
    "no drift when the running config is this file",
    drift_check.check_alertmanager_config(SAMPLE, metrics(SAMPLE_HASH)) is None,
)
check(
    "drift when the running config is a different file",
    drift_check.check_alertmanager_config(SAMPLE, metrics(EMPTY_HASH)) is not None,
)

sys.stdout.write("\n[regression] comments and expanded defaults are not drift\n")
# The exact shape that made the old check permanently red: the file on disk
# opens with a comment block and omits the defaults, while what Alertmanager
# reports back has neither property. Only the bytes it hashed are the same.
COMMENTED = b"# Alertmanager configuration for the Alt platform.\n" + SAMPLE
check(
    "a commented file is not drift when its own hash matches",
    drift_check.check_alertmanager_config(
        COMMENTED, metrics(drift_check.alertmanager_config_hash(COMMENTED))
    )
    is None,
)

sys.stdout.write("\n[drift] a failed reload is drift even when the file matches\n")
# The file reached the container and Alertmanager rejected it. The bytes agree;
# the process is still running the previous config. Reporting "in sync" here
# would be the same lie in the opposite direction.
check(
    "reports drift when the last reload failed",
    drift_check.check_alertmanager_config(SAMPLE, metrics(SAMPLE_HASH, reload_ok=0)) is not None,
)

sys.stdout.write("\n[drift] an unreadable exposition is reported, not assumed healthy\n")
check(
    "reports when the hash metric is absent",
    drift_check.check_alertmanager_config(SAMPLE, "# no metrics here\n") is not None,
)

sys.stdout.write(f"\nTotal: PASS={RESULTS.passed} FAIL={RESULTS.failed}\n")
sys.exit(1 if RESULTS.failed else 0)
