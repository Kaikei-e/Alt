#!/usr/bin/env python3
"""Production compose upstream wiring and env-template drift.

Static checks only (PyYAML, no docker): the URLs that production compose
hands to a client must reach a listener that serves the call, over mTLS
where the client refuses anything else, and the operator env templates must
not override a compose default with a value that no longer works.

Run:
    python3 scripts/tests/test-compose-prod-env-wiring.py
"""

from __future__ import annotations

import pathlib
import re
import sys
from collections.abc import Callable

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

from compose_include import load_yaml, production_compose_files, production_services  # noqa: E402

PASS = 0
FAIL = 0

INTERP_DEFAULT = re.compile(r"^\$\{(?P<var>[A-Za-z_][A-Za-z0-9_]*):?-(?P<default>[^}]*)\}$")


def check(name: str, condition: Callable[[], bool]) -> None:
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
        print(f"  FAIL  {name}")
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


def effective(value: str) -> str:
    """What the container sees when the operator leaves the variable unset."""
    m = INTERP_DEFAULT.match(value)
    return m.group("default") if m else value


def peers(svc: dict) -> set[str]:
    raw = effective(env_map(svc).get("MTLS_ALLOWED_PEERS", ""))
    return {p.strip() for p in raw.split(",") if p.strip()}


def dotenv(path: pathlib.Path) -> dict[str, str]:
    out: dict[str, str] = {}
    for line in path.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        out[key.strip()] = value.strip().strip('"').strip("'")
    return out


def compose_defaults() -> dict[str, set[str]]:
    """{VAR: {default, ...}} for every `${VAR:-default}` env entry in production."""
    found: dict[str, set[str]] = {}
    for path in production_compose_files():
        for svc in (load_yaml(path).get("services") or {}).values():
            if not isinstance(svc, dict):
                continue
            for value in env_map(svc).values():
                m = INTERP_DEFAULT.match(value)
                if m:
                    found.setdefault(m.group("var"), set()).add(m.group("default"))
    return found


services = production_services()
backend = env_map(services["alt-backend"])
acolyte = env_map(services["acolyte-orchestrator"])
search_indexer = services["search-indexer"]
pre_processor = services["pre-processor"]

print("alt-backend upstreams (the clients refuse non-https and exit the process)")
for var, want in (
    ("SEARCH_INDEXER_CONNECT_URL", "https://search-indexer:9443"),
    ("PRE_PROCESSOR_URL", "https://pre-processor:9443"),
    ("PRE_PROCESSOR_CONNECT_URL", "https://pre-processor:9443"),
):
    check(f"alt-backend {var} is {want}", lambda var=var, want=want: effective(backend.get(var, "")) == want)
for name, svc in (("search-indexer", search_indexer), ("pre-processor", pre_processor)):
    check(
        f"{name} serves mTLS on :9443",
        lambda svc=svc: env_map(svc).get("MTLS_PORT") == "9443"
        and effective(env_map(svc).get("MTLS_LISTEN", "")) == "true",
    )
    check(f"{name} admits the alt-backend leaf", lambda svc=svc: "alt-backend" in peers(svc))
check(
    "alt-backend presents a client leaf",
    lambda: all(backend.get(k) for k in ("MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE")),
)

print("acolyte-orchestrator search (search-indexer :9300 is health-only)")
check(
    "acolyte SEARCH_INDEXER_URL defaults to https://search-indexer:9443",
    lambda: effective(acolyte.get("SEARCH_INDEXER_URL", "")) == "https://search-indexer:9443",
)
check(
    "acolyte presents its leaf by default",
    lambda: effective(acolyte.get("MTLS_ENFORCE", "")) == "true",
)
check(
    "search-indexer admits the acolyte-orchestrator leaf",
    lambda: "acolyte-orchestrator" in peers(search_indexer),
)

print("kratos cookie domain: empty is host-only, unset is a misconfiguration")
for name in ("kratos", "kratos-migrate"):
    check(
        f"{name} requires KRATOS_COOKIE_DOMAIN to be set (empty allowed)",
        lambda name=name: re.fullmatch(
            r"\$\{KRATOS_COOKIE_DOMAIN\?[^}]+\}",
            env_map(services[name]).get("KRATOS_COOKIE_DOMAIN", ""),
        ) is not None,
    )

defaults = compose_defaults()
for template in (".env.template", ".env.example"):
    values = dotenv(ROOT / template)
    print(f"{template} does not override compose with dead values")
    check(f"{template} declares KRATOS_COOKIE_DOMAIN", lambda values=values: "KRATOS_COOKIE_DOMAIN" in values)
    check(
        f"{template} never points at knowledge-embedder-local (embedding-raw-network only)",
        lambda values=values: not [k for k, v in values.items() if "knowledge-embedder-local" in v],
    )
    check(
        f"{template} does not downgrade an https compose default to http",
        lambda values=values: not [
            k for k, v in values.items()
            if v.startswith("http://")
            and any(d.startswith("https://") for d in defaults.get(k, ()))
        ],
    )
    check(
        f"{template} does not pin an mTLS peer list that differs from compose",
        lambda values=values: not [
            k for k, v in values.items()
            if k.endswith("_MTLS_ALLOWED_PEERS") and v not in defaults.get(k, set())
        ],
    )

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
