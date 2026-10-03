#!/usr/bin/env bash
# scripts/tests/test_compose_wiring.sh
# Compose wiring regression tests (offline: config rendering only, no containers).
#
# Raw YAML (compose/*.yaml as written):
#   - generation-raw-network / embedding-raw-network are internal; news-creator-backend
#     is only on generation-raw-network; the proxies bridge alt-network and the raw nets
#   - rag-orchestrator's healthcheck is the native CLI probe (127.0.0.1:9012/healthz by
#     default) and nothing overrides RAG_HEALTH_PORT
#   - rag-orchestrator reaches search-indexer over https://search-indexer:9443 and
#     carries one inference token (no RERANK_INFERENCE_TOKEN_FILE)
#   - recap-evaluator / knowledge-sovereign in-process enrollment (no pki-agent
#     sidecar), news-creator inbound mTLS and peers,
#     acolyte NEWS_CREATOR_URL, auth-token-manager OTEL/RASK enrollment, the six
#     sovereign roles, and the embedding-proxy callers' token mounts
#
# Rendered config (`docker compose config` against COMPOSE_ENV_FILE, default
# .env.template), which is where `${VAR:-default}` and env_file overrides land:
#   - embedding / generation / rerank callers resolve to their proxy hosts, and the
#     frontend's Connect calls resolve to the BFF
#   - every inference caller mounts inference_service_token and none disables it
#   - every redis-streams / redis-cache URL names its ACL user and pairs with that
#     user's password file and secret
#   - an entrypoint mounted from a file-backed config is executable on disk, because
#     compose ignores `configs[].mode` for file sources
#
# Usage (from anywhere): bash scripts/tests/test_compose_wiring.sh
#   COMPOSE_ENV_FILE  env file passed to --env-file (default: <repo>/.env.template)
# Requirements: python3 with PyYAML, docker compose, and <repo>/.env present for the
# services' `env_file: ../.env` (CI stages it from .env.template).
# Rendered values are written to a 0600 temp file and never printed.

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
COMPOSE_DIR="$REPO_ROOT/compose"
ENV_FILE="${COMPOSE_ENV_FILE:-$REPO_ROOT/.env.template}"
export REPO_ROOT COMPOSE_DIR

STATUS=0

# ─── 1. Render the production stack ──────────────────────────────────────────
echo ""
echo "==> 1. Render compose/compose.yaml against ${ENV_FILE#"$REPO_ROOT"/}"
RENDERED_JSON="$(mktemp)"
chmod 600 "$RENDERED_JSON"
trap 'rm -f "$RENDERED_JSON"' EXIT
export RENDERED_JSON
if docker compose --env-file "$ENV_FILE" -f "$COMPOSE_DIR/compose.yaml" \
    config --format json > "$RENDERED_JSON"; then
  echo "  [PASS]  docker compose config renders"
  RENDERED=1
else
  echo "  [FAIL]  docker compose config failed (missing $REPO_ROOT/.env? CI stages it from .env.template)"
  STATUS=1
  RENDERED=0
fi

# ─── 2. Raw YAML checks ──────────────────────────────────────────────────────
echo ""
echo "==> 2. Service and network wiring checks (raw YAML)"

if ! python3 - <<'PYEOF'
import os
import sys

import yaml

COMPOSE_DIR = os.environ["COMPOSE_DIR"]

def load(fname):
    with open(os.path.join(COMPOSE_DIR, fname)) as f:
        return yaml.safe_load(f)

def env_dict(service):
    values = service.get("environment", {})
    if isinstance(values, dict):
        return {k: str(v) for k, v in values.items()}
    return dict(str(e).split("=", 1) if "=" in str(e) else (str(e), "") for e in values)

ai    = load("ai.yaml")
rag   = load("rag.yaml")
recap = load("recap.yaml")
work  = load("workers.yaml")
acol  = load("acolyte.yaml")
sov   = load("sovereign.yaml")

PASS, FAIL = 0, 0
def check(cond, desc):
    global PASS, FAIL
    if cond:
        print(f"  [PASS]  {desc}")
        PASS += 1
    else:
        print(f"  [FAIL]  {desc}")
        FAIL += 1

# ── 2a. generation-raw-network declared internal ──────────────────────────────
ai_nets = ai.get("networks", {})
check("generation-raw-network" in ai_nets,
      "generation-raw-network is declared in ai.yaml")
check(ai_nets.get("generation-raw-network", {}).get("internal") is True,
      "generation-raw-network.internal=true")

# ── 2b. news-creator-backend NOT on alt-network ───────────────────────────────
ncb_nets = ai["services"]["news-creator-backend"].get("networks", [])
check("alt-network" not in ncb_nets,
      "news-creator-backend NOT on alt-network")
check("generation-raw-network" in ncb_nets,
      "news-creator-backend IS on generation-raw-network")

# ── 2c. news-creator-backend has no host port publish ────────────────────────
ncb_ports = ai["services"]["news-creator-backend"].get("ports", [])
check(not any("11435" in str(p) for p in ncb_ports),
      "news-creator-backend has no host port 11435 published")

# ── 2d. generation-proxy on both networks ─────────────────────────────────────
gp_nets = ai["services"]["generation-proxy"].get("networks", [])
check("alt-network" in gp_nets and "generation-raw-network" in gp_nets,
      "generation-proxy on alt-network + generation-raw-network")

# ── 2e. embedding-proxy networks ─────────────────────────────────────────────
rag_nets = rag.get("networks", {})
check("embedding-raw-network" in rag_nets,
      "embedding-raw-network declared in rag.yaml")
check(rag_nets.get("embedding-raw-network", {}).get("internal") is True,
      "embedding-raw-network.internal=true")
ep_nets = rag["services"]["embedding-proxy"].get("networks", [])
check("alt-network" in ep_nets and "embedding-raw-network" in ep_nets,
      "embedding-proxy on alt-network + embedding-raw-network")

# ── 2f. rag-orchestrator healthcheck is the native CLI probe ─────────────────
# `/rag-orchestrator healthcheck` GETs 127.0.0.1:<RAG_HEALTH_PORT or 9012>/healthz
# (cmd/server/main.go); the listener reads the same variable, so overriding it
# on one side only would split them.
ro = rag["services"]["rag-orchestrator"]
ro_env = env_dict(ro)
check(ro.get("healthcheck", {}).get("test") == ["CMD", "/rag-orchestrator", "healthcheck"],
      "rag-orchestrator healthcheck is the native CLI probe (127.0.0.1:9012/healthz)")
check("RAG_HEALTH_PORT" not in ro_env,
      "rag-orchestrator does not override RAG_HEALTH_PORT (probe and listener share :9012)")

# ── 2g. rag-orchestrator upstream contract ───────────────────────────────────
check(ro_env.get("SEARCH_INDEXER_URL") == "https://search-indexer:9443",
      "rag-orchestrator SEARCH_INDEXER_URL=https://search-indexer:9443 "
      "(requires the https-only rag-orchestrator config)")
check("RERANK_INFERENCE_TOKEN_FILE" not in ro_env,
      "rag-orchestrator has no RERANK_INFERENCE_TOKEN_FILE; INFERENCE_SERVICE_TOKEN_FILE "
      "covers embedder, rerank and Augur (requires the single-token rag-orchestrator config)")

# ── 2h. recap-evaluator has healthcheck ───────────────────────────────────────
re_hc = recap["services"]["recap-evaluator"].get("healthcheck", {})
check(bool(re_hc),
      "recap-evaluator has a healthcheck")
check("8080" in str(re_hc.get("test", [])),
      "recap-evaluator healthcheck targets :8080")

# ── 2i/2j. recap-evaluator and knowledge-sovereign enroll in-process ────────
# The parent writes 0400 keys as its runtime user, so pre_start must chown the
# cert volume to that uid: evaluator 65533, distroless nonroot 65532.
def chowns_certs_to(service, uid):
    return any(f"chown -R {uid}:{uid} /certs" in " ".join(map(str, h.get("command") or []))
               for h in service.get("pre_start") or [])

for svc_name, stack, uid in [("recap-evaluator", recap, "65533"),
                             ("knowledge-sovereign", sov, "65532")]:
    svc = stack["services"][svc_name]
    check(env_dict(svc).get("PKI_ENROLLMENT") == "enabled",
          f"{svc_name} PKI_ENROLLMENT=enabled (in-process enrollment)")
    check(chowns_certs_to(svc, uid),
          f"{svc_name} pre_start chowns /certs to {uid}:{uid}")
    check(f"pki-agent-{svc_name}" not in stack["services"],
          f"pki-agent-{svc_name} is not declared (it would be a dual writer)")

# ── 2k. news-creator INBOUND_MTLS=true, MTLS_ALLOWED_PEERS includes pre-processor
nc_env = ai["services"]["news-creator"].get("environment", [])
nc_env_str = " ".join(str(e) for e in nc_env)
check("INBOUND_MTLS=true" in nc_env_str,
      "news-creator INBOUND_MTLS=true")
mtls_peers_entries = [e for e in nc_env if "MTLS_ALLOWED_PEERS" in str(e)]
check(len(mtls_peers_entries) == 1,
      f"news-creator has exactly 1 MTLS_ALLOWED_PEERS entry (got {len(mtls_peers_entries)})")
check("pre-processor" in str(mtls_peers_entries[0]) if mtls_peers_entries else False,
      "news-creator MTLS_ALLOWED_PEERS includes pre-processor")
check("recap-evaluator" in str(mtls_peers_entries[0]) if mtls_peers_entries else False,
      "news-creator MTLS_ALLOWED_PEERS includes recap-evaluator")

# ── 2l. acolyte NEWS_CREATOR_URL default is https://news-creator:9443 ────────
acol_env = acol["services"]["acolyte-orchestrator"].get("environment", [])
nc_url_entry = next((e for e in acol_env if "NEWS_CREATOR_URL" in str(e)), "")
check("https://news-creator:9443" in str(nc_url_entry),
      f"acolyte NEWS_CREATOR_URL default is https://news-creator:9443 (got: {nc_url_entry})")

# ── 2m. recap-subworker OLLAMA_EMBED_URL default uses embedding-proxy ────────
rs_env = recap["services"]["recap-subworker"].get("environment", [])
embed_entry = next((e for e in rs_env if "RECAP_SUBWORKER_OLLAMA_EMBED_URL" in str(e)), "")
check("embedding-proxy" in str(embed_entry),
      f"recap-subworker OLLAMA_EMBED_URL default uses embedding-proxy (got: {embed_entry})")

# ── 2n. search-indexer MEILI_EMBEDDER_URL uses embedding-proxy ───────────────
si_env = work["services"]["search-indexer"].get("environment", [])
meili_embed_entry = next((e for e in si_env if "MEILI_EMBEDDER_URL" in str(e)), "")
check("embedding-proxy" in str(meili_embed_entry),
      f"search-indexer MEILI_EMBEDDER_URL uses embedding-proxy (got: {meili_embed_entry})")
check("11436" in str(meili_embed_entry),
      "search-indexer MEILI_EMBEDDER_URL uses port 11436")

# ── 2o. search-indexer secrets include inference_service_token ───────────────
si_secrets = work["services"]["search-indexer"].get("secrets", [])
check("inference_service_token" in si_secrets,
      "search-indexer secrets include inference_service_token")

# ── 2p. auth-token-manager OTEL/RASK enrollment ──────────────────────────────
atm_env = work["services"]["auth-token-manager"].get("environment", [])
atm_env_str = " ".join(str(e) for e in atm_env)
check("OTEL_ENABLED=true" in atm_env_str,
      "auth-token-manager OTEL_ENABLED=true")
check("rask-log-aggregator:4318" in atm_env_str,
      "auth-token-manager OTEL_EXPORTER_OTLP_ENDPOINT=http://rask-log-aggregator:4318")
check("RASK_INGEST_TOKEN_FILE=/run/secrets/rask_ingest_token" in atm_env_str,
      "auth-token-manager RASK_INGEST_TOKEN_FILE set")
atm_secrets = work["services"]["auth-token-manager"].get("secrets", [])
check("rask_ingest_token" in atm_secrets,
      "auth-token-manager secrets include rask_ingest_token")

# ── 2q. Six stable sovereign roles untouched ─────────────────────────────────
sov_secrets = sov["services"]["knowledge-sovereign"].get("secrets", [])
for role in ["sovereign_backend_token", "sovereign_operator_token",
             "sovereign_datahub_token", "sovereign_harvester_token",
             "sovereign_rag_token", "sovereign_recap_token"]:
    check(role in sov_secrets,
          f"knowledge-sovereign retains secret: {role}")

# ── 2r. recap-evaluator enrolls with its own JWK ───────────────────────────
re_svc = recap["services"]["recap-evaluator"]
check("pki-agent-recap-evaluator-jwk" in re_svc.get("secrets", []),
      "recap-evaluator mounts its own JWK secret")

# ── 2s. recap-evaluator is the writer of its cert volume ─────────────────────
re_vols = [str(v) for v in re_svc.get("volumes", [])]
check("recap_evaluator_certs:/certs" in re_vols,
      "recap-evaluator mounts recap_evaluator_certs:/certs read-write")

# ── 2t. embedding-proxy callers carry the inference bearer ──────────────────
for svc_name, svc in [("rag-orchestrator", ro),
                      ("recap-subworker", recap["services"]["recap-subworker"])]:
    check(env_dict(svc).get("INFERENCE_SERVICE_TOKEN_FILE") == "/run/secrets/inference_service_token",
          f"{svc_name} INFERENCE_SERVICE_TOKEN_FILE set")
    check("inference_service_token" in svc.get("secrets", []),
          f"{svc_name} secrets include inference_service_token")

print(f"\n  Results: {PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
PYEOF
then
  STATUS=1
fi

# ─── 3. Rendered-config checks ───────────────────────────────────────────────
echo ""
echo "==> 3. URL and credential wiring checks (rendered config)"

if [ "$RENDERED" -eq 1 ]; then
  if ! python3 - <<'PYEOF'
import json
import os
import sys
from urllib.parse import urlsplit

with open(os.environ["RENDERED_JSON"]) as f:
    cfg = json.load(f)
services = cfg["services"]
TOKEN = "/run/secrets/inference_service_token"

PASS, FAIL = 0, 0
def check(cond, desc):
    global PASS, FAIL
    if cond:
        print(f"  [PASS]  {desc}")
        PASS += 1
    else:
        print(f"  [FAIL]  {desc}")
        FAIL += 1

def env(name):
    return services[name].get("environment") or {}

def secret_sources(name):
    return {s["source"] for s in services[name].get("secrets") or []}

def endpoint(name, variable):
    """host:port/path of a rendered URL — never the userinfo or query."""
    parts = urlsplit(env(name).get(variable) or "")
    return f"{parts.hostname}:{parts.port}{parts.path}"

# ── 3a. Callers resolve to the auth proxies, not the raw backends ────────────
# `${VAR:-default}` in the YAML says nothing about the value deploy uses: an
# env file entry for VAR replaces the default wholesale.
for name, variable, expected in [
    ("rag-orchestrator", "EMBEDDER_EXTERNAL", "embedding-proxy:11436"),
    ("recap-subworker", "RECAP_SUBWORKER_OLLAMA_EMBED_URL", "embedding-proxy:11436"),
    ("search-indexer", "MEILI_EMBEDDER_URL", "embedding-proxy:11436/api/embed"),
    ("news-creator", "LLM_SERVICE_URL", "generation-proxy:11436"),
    ("rag-orchestrator", "RERANK_URL", "rerank-local:8080"),
    # The frontend reaches alt-backend only through the BFF.
    ("alt-frontend-sv", "BACKEND_CONNECT_URL", "alt-butterfly-facade:9250"),
]:
    got = endpoint(name, variable)
    check(got.startswith(expected),
          f"{name} {variable} resolves to {expected} (got {got})")

# ── 3b. Inference callers and servers share one mounted token ────────────────
for name in ("rag-orchestrator", "recap-subworker", "news-creator", "search-indexer"):
    check(env(name).get("INFERENCE_SERVICE_TOKEN_FILE") == TOKEN,
          f"{name} INFERENCE_SERVICE_TOKEN_FILE={TOKEN}")
    check("inference_service_token" in secret_sources(name),
          f"{name} mounts inference_service_token")
    check((env(name).get("INFERENCE_AUTH") or "").lower() != "disabled",
          f"{name} does not run with INFERENCE_AUTH=disabled")
for name in ("embedding-proxy", "generation-proxy"):
    command = services[name].get("command") or []
    flags = dict(zip(command[::2], command[1::2]))
    check(flags.get("--token-file") == TOKEN and "inference_service_token" in secret_sources(name),
          f"{name} enforces the mounted inference_service_token")
check(env("rerank-local").get("INFERENCE_SERVICE_TOKEN_FILE") == TOKEN
      and "inference_service_token" in secret_sources("rerank-local"),
      "rerank-local enforces the mounted inference_service_token")

# ── 3c. Redis clients authenticate as the ACL users the entrypoint defines ───
ACL_USERS = {"redis-streams": {"streams", "limiter"}, "redis-cache": {"cache"}}
for name, svc in sorted(services.items()):
    for variable, value in sorted((svc.get("environment") or {}).items()):
        if not isinstance(value, str) or not value.startswith(("redis://", "rediss://")):
            continue
        parts = urlsplit(value)
        if parts.hostname not in ACL_USERS:
            continue
        user = parts.username
        password_var = variable[: -len("_URL")] + "_PASSWORD_FILE" if variable.endswith("_URL") else ""
        if password_var not in (svc.get("environment") or {}):
            password_var = "REDIS_PASSWORD_FILE"
        expected_file = f"/run/secrets/redis_{user}_password"
        check(user in ACL_USERS[parts.hostname] and parts.password is None,
              f"{name} {variable} names an ACL user of {parts.hostname} with no inline password (got user={user})")
        check(env(name).get(password_var) == expected_file
              and f"redis_{user}_password" in secret_sources(name),
              f"{name} {password_var} reads the mounted redis_{user}_password")

# ── 3d. Config-mounted entrypoints are executable on disk ────────────────────
configs = cfg.get("configs") or {}
for name, svc in sorted(services.items()):
    entrypoint = svc.get("entrypoint") or []
    for mount in svc.get("configs") or []:
        source = configs.get(mount["source"], {}).get("file")
        if source and entrypoint and entrypoint[0] == mount.get("target"):
            rel = os.path.relpath(source, os.environ["REPO_ROOT"])
            check(os.access(source, os.X_OK),
                  f"{name} entrypoint {rel} is executable "
                  "(compose ignores configs.mode for file sources)")

print(f"\n  Results: {PASS} passed, {FAIL} failed")
sys.exit(1 if FAIL else 0)
PYEOF
  then
    STATUS=1
  fi
else
  echo "  [SKIP]  render failed; see step 1"
fi

echo ""
echo "==> Summary"
if [ "$STATUS" -eq 0 ]; then
  echo "  All checks passed."
else
  echo "  Some checks FAILED. Review output above."
fi
exit "$STATUS"
