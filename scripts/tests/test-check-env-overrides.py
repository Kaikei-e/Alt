#!/usr/bin/env python3
"""Tests for scripts/check-env-overrides.py.

A host .env outlives the compose change that retired one of its values:
compose keeps reading `${VAR:-default}` from it, so a stale value silently
beats the fixed default. The checker applies the deploy/host-prereqs.yaml
env rules to a host env file and reports key names and rule ids only, since
the file it reads is the production one.

Run:
    python3 scripts/tests/test-check-env-overrides.py
"""

from __future__ import annotations

import importlib.util
import pathlib
import subprocess
import sys
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "scripts" / "check-env-overrides.py"
MANIFEST = ROOT / "deploy" / "host-prereqs.yaml"

spec = importlib.util.spec_from_file_location("check_env_overrides", SCRIPT)
assert spec is not None and spec.loader is not None
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)

PASS = 0
FAIL = 0


def check(name: str, condition: bool, detail: str = "") -> None:
    global PASS, FAIL
    if condition:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        print(f"  FAIL  {name}" + (f" ({detail})" if detail else ""))
        FAIL += 1


def pairs(findings) -> set[tuple[str, str, str]]:
    return {(f.severity, f.key, f.rule) for f in findings}


def run_cli(*env_texts: str, extra_args: tuple[str, ...] = ()) -> subprocess.CompletedProcess:
    with tempfile.TemporaryDirectory() as tmp:
        args = [sys.executable, str(SCRIPT)]
        for i, text in enumerate(env_texts):
            path = pathlib.Path(tmp) / f"host-{i}.env"
            path.write_text(text, encoding="utf-8")
            args += ["--env-file", str(path)]
        return subprocess.run([*args, *extra_args], capture_output=True, text=True, check=False)


rules = checker.load_rules(MANIFEST)

print("parse_env_file")
parsed = checker.parse_env_file(
    "\n".join(
        [
            "# comment",
            "",
            "PLAIN=value",
            "export EXPORTED=exported-value",
            'DOUBLE="double quoted # not a comment"',
            "SINGLE='single quoted'",
            "INLINE=bare # trailing comment",
            "EMPTY=",
            "  SPACED = spaced value  ",
            "not a pair",
        ]
    )
)
check(
    "reads plain, exported, quoted, inline-commented, empty and spaced pairs",
    parsed
    == {
        "PLAIN": "value",
        "EXPORTED": "exported-value",
        "DOUBLE": "double quoted # not a comment",
        "SINGLE": "single quoted",
        "INLINE": "bare",
        "EMPTY": "",
        "SPACED": "spaced value",
    },
    repr(parsed),
)

print("manifest rules")
check(
    "rule ids are unique across retired_env and dead_env_values",
    len(rules.rule_ids()) == len(set(rules.rule_ids())),
)
check(
    "every rule carries a reason the report can print instead of the value",
    all(rules.reason(rule_id) for rule_id in rules.rule_ids()),
)
check(
    "AUTH_HUB_MTLS_ALLOWED_PEERS requires the peers auth-hub serves today",
    {"alt-backend", "alt-data-hub", "search-indexer", "knowledge-sovereign"}
    <= set(rules.allowlists.get("AUTH_HUB_MTLS_ALLOWED_PEERS", ())),
)

print("evaluate: the pre-rollout host .env")
STALE = {
    "AUTH_HUB_INTERNAL_URL": "http://auth-hub:8888",
    "OLLAMA_EMBED_URL": "http://knowledge-embedder-local:11434",
    "EMBEDDER_EXTERNAL": "http://knowledge-embedder-local:11434",
    "BACKEND_CONNECT_URL": "http://alt-backend:9101",
    "ACOLYTE_SEARCH_URL": "http://search-indexer:9300",
    "AUTH_HUB_MTLS_ALLOWED_PEERS": "alt-backend,alt-butterfly-facade,alt-data-hub",
    "AUGUR_EXTERNAL": "http://news-creator-backend:11435",
    "QUERY_EXPANSION_URL": "http://news-creator:11434",
}
found = pairs(checker.evaluate(STALE, rules))
expected = {
    ("error", "AUTH_HUB_INTERNAL_URL", "auth-hub-health-only-port"),
    ("error", "AUTH_HUB_INTERNAL_URL", "https-only-upstream"),
    ("error", "OLLAMA_EMBED_URL", "embedder-raw-network"),
    ("error", "EMBEDDER_EXTERNAL", "embedder-raw-network"),
    ("error", "BACKEND_CONNECT_URL", "frontend-connect-bypasses-bff"),
    ("error", "ACOLYTE_SEARCH_URL", "https-only-upstream"),
    ("error", "AUTH_HUB_MTLS_ALLOWED_PEERS", "allowlist-missing-peer"),
    ("warning", "AUTH_HUB_MTLS_ALLOWED_PEERS", "allowlist-extra-peer"),
    ("warning", "AUGUR_EXTERNAL", "rag-orchestrator-pinned-upstreams"),
    ("warning", "QUERY_EXPANSION_URL", "rag-orchestrator-pinned-upstreams"),
}
check("flags every stale override with its rule", found == expected, f"missing={expected - found} extra={found - expected}")

print("evaluate: overrides that still work")
CURRENT = {
    "AUTH_HUB_INTERNAL_URL": "https://auth-hub:8443",
    "OLLAMA_EMBED_URL": "http://embedding-proxy:11436",
    "EMBEDDER_EXTERNAL": "http://embedding-proxy:11436",
    "BACKEND_CONNECT_URL": "http://alt-butterfly-facade:9250",
    "ACOLYTE_SEARCH_URL": "https://search-indexer:9443",
    "AUTH_HUB_MTLS_ALLOWED_PEERS": "knowledge-sovereign,search-indexer,alt-data-hub,alt-backend",
    "RECAP_DB_DSN": "postgres://${RECAP_DB_USER}:${RECAP_DB_PASSWORD}@${RECAP_DB_HOST}:${RECAP_DB_PORT}/${RECAP_DB_NAME}",
    "MEILI_MASTER_KEY": "not-a-url",
}
check("current values, reordered peer lists and DSNs pass", checker.evaluate(CURRENT, rules) == [])
check(
    "an empty value leaves the compose default in charge",
    checker.evaluate({"BACKEND_CONNECT_URL": "", "AUTH_HUB_MTLS_ALLOWED_PEERS": ""}, rules) == [],
)
check(
    "a raw-embedder host is dead under any key and without a scheme",
    pairs(checker.evaluate({"SOME_NEW_URL": "knowledge-embedder-local:11434"}, rules))
    == {("error", "SOME_NEW_URL", "embedder-raw-network")},
)
check(
    "a retired key is reported even when empty",
    pairs(checker.evaluate({"RERANK_URL": ""}, rules))
    == {("warning", "RERANK_URL", "rag-orchestrator-pinned-upstreams")},
)
check(
    "an https-only upstream without a scheme is dead",
    pairs(checker.evaluate({"ACOLYTE_SEARCH_URL": "search-indexer:9443"}, rules))
    == {("error", "ACOLYTE_SEARCH_URL", "https-only-upstream")},
)

print("CLI: prints key names and rule ids, never values")
MARKED = "\n".join(
    [
        "AUTH_HUB_INTERNAL_URL=http://auth-hub:8888/leak-marker-a1",
        "OLLAMA_EMBED_URL=http://knowledge-embedder-local:11434/leak-marker-b2",
        "BACKEND_CONNECT_URL=http://alt-backend:9101/leak-marker-c3",
        "AUTH_HUB_MTLS_ALLOWED_PEERS=alt-backend,leak-marker-d4",
        "AUGUR_EXTERNAL=http://news-creator-backend:11435/leak-marker-e5",
        "MEILI_MASTER_KEY=leak-marker-f6",
    ]
)
cli = run_cli(MARKED)
output = cli.stdout + cli.stderr
check("exits 1 when an override is dead", cli.returncode == 1, f"rc={cli.returncode}")
check("never prints a value from the env file", "leak-marker" not in output)
check(
    "names each offending key with its rule id",
    all(
        key in output and rule in output
        for key, rule in (
            ("AUTH_HUB_INTERNAL_URL", "auth-hub-health-only-port"),
            ("OLLAMA_EMBED_URL", "embedder-raw-network"),
            ("BACKEND_CONNECT_URL", "frontend-connect-bypasses-bff"),
            ("AUTH_HUB_MTLS_ALLOWED_PEERS", "allowlist-missing-peer"),
            ("AUGUR_EXTERNAL", "rag-orchestrator-pinned-upstreams"),
        )
    ),
    output,
)
check("does not mention keys no rule matched", "MEILI_MASTER_KEY" not in output)

warn_only = run_cli("QUERY_EXPANSION_URL=http://news-creator:11434\n")
check(
    "exits 0 when only warnings remain, still naming the key",
    warn_only.returncode == 0 and "QUERY_EXPANSION_URL" in warn_only.stdout,
    f"rc={warn_only.returncode}",
)

clean = run_cli("AUTH_HUB_INTERNAL_URL=https://auth-hub:8443\nMEILI_MASTER_KEY=leak-marker-g7\n")
check(
    "exits 0 on a clean env file without echoing it",
    clean.returncode == 0 and "leak-marker" not in clean.stdout + clean.stderr,
    f"rc={clean.returncode}",
)

both = run_cli("AUTH_HUB_INTERNAL_URL=https://auth-hub:8443\n", "EMBEDDER_EXTERNAL=http://knowledge-embedder-local:11434\n")
check(
    "checks every --env-file it is given",
    both.returncode == 1 and "EMBEDDER_EXTERNAL" in both.stdout,
    f"rc={both.returncode}",
)

missing = subprocess.run(
    [sys.executable, str(SCRIPT), "--env-file", str(ROOT / "does-not-exist.env")],
    capture_output=True,
    text=True,
    check=False,
)
check("exits 2 when the env file is missing", missing.returncode == 2, f"rc={missing.returncode}")

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
