#!/usr/bin/env python3
"""knowledge-sovereign compose environment knob contract.

Every KNOWLEDGE_SOVEREIGN_* variable configured on the knowledge-sovereign
service in compose/sovereign.yaml and compose/compose.staging.yaml must be
read by the Go service under knowledge-sovereign/app (non-test .go sources),
and any ${VAR:-default} fallback in compose/sovereign.yaml must match the
default in knowledge-sovereign/app/config/config.go.

Run:
    python3 scripts/tests/test-compose-sovereign-knobs.py
"""

from __future__ import annotations

import pathlib
import re
import sys
from collections.abc import Callable

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

from compose_include import load_yaml  # noqa: E402

SERVICE = "knowledge-sovereign"
PREFIX = "KNOWLEDGE_SOVEREIGN_"
COMPOSE_TARGETS = (
    ROOT / "compose" / "sovereign.yaml",
    ROOT / "compose" / "compose.staging.yaml",
)
APP_DIR = ROOT / "knowledge-sovereign" / "app"
CONFIG_GO = APP_DIR / "config" / "config.go"

INTERP_RE = re.compile(
    r"\$\{(?P<var>[A-Za-z_][A-Za-z0-9_]*)(?::?-(?P<default>[^}]*))?\}"
)
PARSE_INT_RE = re.compile(
    r'parseIntEnv\(\s*"(?P<var>KNOWLEDGE_SOVEREIGN_[A-Za-z0-9_]+)"\s*,\s*(?P<default>\d+)\s*\)'
)

PASS = 0
FAIL = 0


def check(name: str, condition: Callable[[], bool], detail: str = "") -> None:
    global PASS, FAIL
    try:
        ok = bool(condition())
    except Exception as exc:
        print(f"  FAIL  {name} ({type(exc).__name__}: {exc})")
        FAIL += 1
        return
    if ok:
        print(f"  PASS  {name}")
        PASS += 1
    else:
        suffix = f" — {detail}" if detail else ""
        print(f"  FAIL  {name}{suffix}")
        FAIL += 1


def env_map(svc: dict) -> dict[str, str]:
    env = svc.get("environment") or {}
    if isinstance(env, dict):
        return {str(k): "" if v is None else str(v) for k, v in env.items()}
    out: dict[str, str] = {}
    for item in env:
        key, _, value = str(item).partition("=")
        out[key] = value
    return out


def non_test_go_sources(app_dir: pathlib.Path) -> str:
    files = sorted(
        p for p in app_dir.rglob("*.go") if not p.name.endswith("_test.go")
    )
    return "\n".join(p.read_text(encoding="utf-8") for p in files)


def sovereign_env_vars(env: dict[str, str]) -> set[str]:
    names = {k for k in env if k.startswith(PREFIX)}
    for value in env.values():
        for match in INTERP_RE.finditer(value):
            var = match.group("var")
            if var.startswith(PREFIX):
                names.add(var)
    return names


def config_int_defaults(config_go: pathlib.Path) -> dict[str, str]:
    text = config_go.read_text(encoding="utf-8")
    return {
        m.group("var"): m.group("default") for m in PARSE_INT_RE.finditer(text)
    }


go_text = non_test_go_sources(APP_DIR)
int_defaults = config_int_defaults(CONFIG_GO)

print("knowledge-sovereign compose KNOWLEDGE_SOVEREIGN_* knobs are read by the service")
for compose_path in COMPOSE_TARGETS:
    rel = compose_path.relative_to(ROOT)
    services = load_yaml(compose_path).get("services") or {}
    svc = services.get(SERVICE) or {}
    env = env_map(svc)
    declared = sorted(sovereign_env_vars(env))
    check(
        f"{rel} declares {PREFIX}* environment knobs on {SERVICE}",
        lambda declared=declared: len(declared) > 0,
    )
    dead = [
        name
        for name in declared
        if f'"{name}"' not in go_text and f"`{name}`" not in go_text
    ]
    check(
        f"{rel} {SERVICE} has no dead {PREFIX}* knobs",
        lambda dead=dead: not dead,
        detail=f"dead knobs: {', '.join(dead)}" if dead else "",
    )

prod_env = env_map(
    (load_yaml(ROOT / "compose" / "sovereign.yaml").get("services") or {}).get(
        SERVICE
    )
    or {}
)
mismatched_defaults: list[str] = []
for key, raw_val in sorted(prod_env.items()):
    if not key.startswith(PREFIX):
        continue
    m = INTERP_RE.fullmatch(raw_val)
    if m and key in int_defaults:
        compose_def = m.group("default") or ""
        code_def = int_defaults[key]
        if compose_def != code_def:
            mismatched_defaults.append(
                f"{key} (compose={compose_def!r}, config.go={code_def!r})"
            )

check(
    "compose/sovereign.yaml ${VAR:-default} fallbacks match config.go defaults",
    lambda: not mismatched_defaults,
    detail=", ".join(mismatched_defaults),
)

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
