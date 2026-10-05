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

import importlib.util
import pathlib
import re
import sys
from collections.abc import Callable

ROOT = pathlib.Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts"))

from compose_include import load_yaml, production_compose_files, production_services  # noqa: E402

MANIFEST = ROOT / "deploy" / "host-prereqs.yaml"

# The host .env checker owns the dead-value rules; the templates must pass the
# same rules the alt-deploy host preflight applies to a production .env.
_spec = importlib.util.spec_from_file_location("check_env_overrides", ROOT / "scripts" / "check-env-overrides.py")
assert _spec is not None and _spec.loader is not None
overrides = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(overrides)

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

print("alt-butterfly-facade never calls auth-hub")
check(
    "alt-butterfly-facade carries no AUTH_HUB_INTERNAL_URL (the BFF has no auth-hub client)",
    lambda: "AUTH_HUB_INTERNAL_URL" not in env_map(services["alt-butterfly-facade"]),
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
manifest = load_yaml(MANIFEST)
required_env = manifest.get("required_env") or []
rules = overrides.load_rules(MANIFEST)
compose_text = "\n".join(path.read_text(encoding="utf-8") for path in production_compose_files())

print("deploy/host-prereqs.yaml env rules stay in lockstep with compose")
check(
    "https-only-upstream covers exactly the keys compose defaults to an https URL",
    lambda: set(rules.keys_for("https-only-upstream"))
    == {k for k, ds in defaults.items() if any(d.startswith("https://") for d in ds)},
)
check(
    "env_allowlists require exactly the peers of every non-empty compose allowlist default",
    lambda: {k: set(v) for k, v in rules.allowlists.items()}
    == {
        k: {p for d in ds for p in d.split(",") if p}
        for k, ds in defaults.items()
        if k.endswith("ALLOWED_PEERS") and any(ds)
    },
)
check(
    "retired_env keys are not interpolated by production compose",
    lambda: not [
        k for k in rules.retired_keys() if re.search(r"\$\{" + re.escape(k) + r"\b", compose_text)
    ],
)
check(
    "every key a dead_env_values rule names is a compose `${VAR:-default}`",
    lambda: not [k for k in rules.named_dead_value_keys() if k not in defaults],
)
check(
    "no compose default trips a dead_env_values rule",
    lambda: not [f for k, ds in defaults.items() for d in ds for f in overrides.evaluate({k: d}, rules)],
)

for template in (".env.template", ".env.example", "compose/.env.example"):
    values = overrides.parse_env_file((ROOT / template).read_text(encoding="utf-8"))
    print(f"{template} does not override compose with dead values")
    check(
        f"{template} trips no deploy/host-prereqs.yaml env rule (scripts/check-env-overrides.py)",
        lambda values=values: overrides.evaluate(values, rules) == [],
    )
    if template == "compose/.env.example":
        continue
    check(f"{template} declares KRATOS_COOKIE_DOMAIN", lambda values=values: "KRATOS_COOKIE_DOMAIN" in values)
    check(
        f"{template} declares every deploy/host-prereqs.yaml required_env key",
        lambda values=values: not [key for key in required_env if key not in values],
    )

print(f"\n{PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
