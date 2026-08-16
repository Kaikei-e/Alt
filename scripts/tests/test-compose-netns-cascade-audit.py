#!/usr/bin/env python3
"""Tests for the parsers behind scripts/compose-netns-cascade-audit.py.

The audit can only require a sidecar it can see. A parser that stops
understanding either side of the comparison — the compose `network_mode`
key or the NETNS_SIDECARS array in the shell script — turns the audit
green for every sidecar at once, which is indistinguishable from "the
lists agree" and reopens the netns-orphan class the audit exists to close.
So the parsers are pinned here, including the two ways they must refuse to
be silent: a renamed array and a malformed row both raise instead of
yielding nothing.

Run:
    python3 scripts/tests/test-compose-netns-cascade-audit.py
Exit 0 on green, non-zero on red.
"""

import importlib.util
import pathlib
import sys

ROOT = pathlib.Path(__file__).resolve().parents[2]

spec = importlib.util.spec_from_file_location(
    "netns_cascade_audit", ROOT / "scripts" / "compose-netns-cascade-audit.py"
)
assert spec is not None and spec.loader is not None, "audit script not importable"
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)

PASS = 0
FAIL = 0


def check(name, condition):
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}")
        FAIL += 1


def raises(fn):
    try:
        fn()
    except SystemExit:
        return True
    return False


print("parse_cascade_rows")

ARRAY = """\
# compose_service : sidecar_container_name : parent_container_name
NETNS_SIDECARS=(
  "pki-agent-tag-generator:alt-pki-agent-tag-generator-1:alt-tag-generator-1"
  "pki-agent-news-creator:alt-pki-agent-news-creator-1:news-creator"
)

for entry in "${NETNS_SIDECARS[@]}"; do
"""

rows = audit.parse_cascade_rows(ARRAY)
check(
    "reads every row and splits the triplet",
    rows
    == [
        (
            "pki-agent-tag-generator",
            "alt-pki-agent-tag-generator-1",
            "alt-tag-generator-1",
        ),
        ("pki-agent-news-creator", "alt-pki-agent-news-creator-1", "news-creator"),
    ],
)
check(
    "stops at the closing paren, so the loop body is not read as a row",
    all(row[0].startswith("pki-agent-") for row in rows),
)

COMMENTED = """\
NETNS_SIDECARS=(
  # "pki-agent-retired:alt-pki-agent-retired-1:alt-retired-1"
  "pki-agent-live:alt-pki-agent-live-1:alt-live-1"  # keeps the :9443 proxy
)
"""
check(
    "a commented-out row is not covered, a trailing comment does not hide one",
    audit.parse_cascade_rows(COMMENTED)
    == [("pki-agent-live", "alt-pki-agent-live-1", "alt-live-1")],
)
check(
    "a renamed array raises instead of reporting full coverage",
    raises(lambda: audit.parse_cascade_rows('SIDECARS=(\n  "a:b:c"\n)\n')),
)
check(
    "a row missing the parent field raises",
    raises(lambda: audit.parse_cascade_rows('NETNS_SIDECARS=(\n  "a:b"\n)\n')),
)

print("netns_sidecars")

SERVICES = {
    "pki-agent-tag-generator": {"network_mode": "service:tag-generator"},
    "pki-agent-news-creator": {"network_mode": " service:news-creator "},
    "pki-agent-alt-backend": {"networks": ["alt-network"]},
    "rask-log-forwarder": {"network_mode": "host"},
    "tag-generator": {"ports": ["127.0.0.1:9400:9400"]},
}
check(
    "only `service:<parent>` counts, not host mode or a plain network",
    audit.netns_sidecars(SERVICES)
    == {
        "pki-agent-tag-generator": "tag-generator",
        "pki-agent-news-creator": "news-creator",
    },
)

print("container_name")

NAMED = {
    "news-creator": {"container_name": "news-creator"},
    "tag-generator": {},
    "pki-agent-tag-generator": {},
}
check(
    "an explicit container_name wins",
    audit.container_name("news-creator", NAMED) == "news-creator",
)
check(
    "otherwise compose derives <project>-<service>-1",
    audit.container_name("tag-generator", NAMED) == "alt-tag-generator-1",
)
check(
    "a sidecar inherits the derived form from the pki-agent anchor",
    audit.container_name("pki-agent-tag-generator", NAMED)
    == "alt-pki-agent-tag-generator-1",
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
