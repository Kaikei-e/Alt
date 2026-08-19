#!/usr/bin/env python3
"""Guard: user-journey SLO counters are wired in production BFF Go.

promtool fixtures can go green on synthetic series. This script must not.
It reads only production Go under alt-butterfly-facade (no tests, no
observability YAML, no frontend). It proves:

  1. alt_user_journey_requests_total is registered in production Go
  2. Metrics.Handler records via Record
  3. Handler is mounted on the real mux in server.go (REST /v1/ and Connect /)

Run:
    python3 scripts/tests/test-user-journey-slo-counters.py
Exit 0 on green, non-zero on red.
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]
BFF = ROOT / "alt-butterfly-facade"

METRIC = "alt_user_journey_requests_total"
JOURNEYS = ("feeds", "login", "search")
STATUSES = ("ok", "error")
SKIP_DIR_NAMES = {
    "node_modules",
    "vendor",
    ".git",
    "dist",
    "gen",
    "__pycache__",
    ".venv",
}

PASS = 0
FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
        return
    print(f"  FAIL  {name}")
    if detail:
        print(f"        {detail}")
    FAIL += 1


def iter_production_go() -> list[pathlib.Path]:
    files: list[pathlib.Path] = []
    if not BFF.is_dir():
        return files
    for path in BFF.rglob("*.go"):
        if any(part in SKIP_DIR_NAMES for part in path.parts):
            continue
        if path.name.endswith("_test.go"):
            continue
        files.append(path)
    return files


print("user-journey SLO counters (production BFF Go only)")

prod_files = iter_production_go()
check(
    "production BFF Go exists",
    len(prod_files) > 0,
    f"no .go files under {BFF.relative_to(ROOT)} excluding tests",
)

metric_hits = [
    p for p in prod_files if METRIC in p.read_text(encoding="utf-8", errors="replace")
]
check(
    f"{METRIC} is registered in production BFF Go",
    len(metric_hits) > 0,
    "counter must be defined in alt-butterfly-facade production Go, not promtool fixtures",
)

corpus = "\n".join(
    p.read_text(encoding="utf-8", errors="replace") for p in metric_hits
) if metric_hits else ""

for journey in JOURNEYS:
    labeled = (
        f'"{journey}"' in corpus
        or re.search(rf'Journey\w+\s*=\s*"{journey}"', corpus) is not None
    )
    check(
        f"production Go names journey {journey!r}",
        labeled,
        f"BFF production Go must record journey={journey!r}",
    )

for status in STATUSES:
    labeled = (
        f'Status{"OK" if status == "ok" else "Error"}' in corpus
        and f'"{status}"' in corpus
    )
    check(
        f"production Go names status {status!r}",
        labeled,
        f"BFF production Go must record status={status!r}",
    )

middleware = BFF / "internal" / "metrics" / "middleware.go"
mw = middleware.read_text(encoding="utf-8", errors="replace") if middleware.is_file() else ""
check(
    "Metrics.Handler is defined in production middleware.go",
    "func (m *Metrics) Handler(" in mw,
    f"{middleware.relative_to(ROOT)} must define Metrics.Handler",
)
check(
    "Handler calls Record on the completed request",
    re.search(r"m\.Record\(\s*r\.URL\.Path", mw) is not None,
    "middleware Handler must call Record(path, status) so the mux wrap is not a no-op",
)

server = BFF / "internal" / "server" / "server.go"
sv = server.read_text(encoding="utf-8", errors="replace") if server.is_file() else ""
check(
    "server.go wires Handler onto REST /v1/",
    "mux.Handle(\"/v1/\"" in sv and "journeyMetrics.Handler(restProxy)" in sv,
    "REST allowlist mux must wrap restProxy with journeyMetrics.Handler",
)
check(
    "server.go wires Handler onto Connect catch-all /",
    "mux.Handle(\"/\"" in sv and "journeyMetrics.Handler(mainHandler)" in sv,
    "Connect catch-all mux must wrap mainHandler with journeyMetrics.Handler",
)
check(
    "server.go does not wrap only a test mux",
    "func NewServer(" in sv and "journeyMetrics := metrics.Default()" in sv,
    "wiring must be in NewServer production constructor, not a test helper",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
