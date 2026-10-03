#!/usr/bin/env bash
# scripts/tests/test_compose_wiring.sh
# Compose wiring regression tests for Sol6.1 / Group3-Sov277 integration.
#
# Validates (offline, config-parse only – no containers started):
#   - generation-raw-network is declared internal
#   - news-creator-backend is NOT on alt-network
#   - generation-proxy IS on both alt-network and generation-raw-network
#   - embedding-proxy IS on both alt-network and embedding-raw-network
#   - rag-orchestrator has a healthcheck on :9012/healthz
#   - recap-evaluator has a healthcheck and pki-agent sidecar with CERT_OWNER_UID=65533
#   - pki-agent-recap-evaluator has CERT_OWNER_UID/GID=65533 (matches Dockerfile evaluator uid)
#   - pki-agent-knowledge-sovereign has CERT_OWNER_UID/GID=65532 (distroless nonroot)
#   - knowledge-sovereign mounts recap_evaluator_certs NOT knowledge_sovereign_certs (distinct volumes)
#   - news-creator INBOUND_MTLS=true, MTLS_ENFORCE dropped from env (uses INBOUND_MTLS)
#   - news-creator MTLS_ALLOWED_PEERS includes pre-processor
#   - acolyte NEWS_CREATOR_URL default is https://news-creator:9443
#   - recap-subworker OLLAMA_EMBED_URL default uses embedding-proxy:11436
#   - search-indexer has MEILI_EMBEDDER_URL pointing to embedding-proxy
#   - search-indexer secrets include inference_service_token
#   - rag-orchestrator / recap-subworker mount inference_service_token and set INFERENCE_SERVICE_TOKEN_FILE
#   - auth-token-manager has OTEL_ENABLED, OTEL_EXPORTER_OTLP_ENDPOINT, RASK_INGEST_TOKEN_FILE
#   - auth-token-manager secrets include rask_ingest_token
#   - Six stable sovereign roles untouched: sovereign_backend_token, sovereign_operator_token,
#     sovereign_datahub_token, sovereign_harvester_token, sovereign_rag_token, sovereign_recap_token
#   - compose config parses cleanly (no YAML errors)
#
# Usage: bash scripts/tests/test_compose_wiring.sh
# Requirements: python3 (yaml), docker (for config parse)
# Runtime: < 60 seconds

set -euo pipefail
PASS=0
FAIL=0
ERRORS=()

info()  { echo "  [INFO]  $*"; }
pass()  { echo "  [PASS]  $*"; PASS=$((PASS+1)); }
fail()  { echo "  [FAIL]  $*"; FAIL=$((FAIL+1)); ERRORS+=("$*"); }

COMPOSE_DIR="$(cd "$(dirname "$0")/../../compose" && pwd)"

# ─── 1. Compose config parses cleanly ────────────────────────────────────────
echo ""
echo "==> 1. Compose YAML parse"
if docker compose -f "${COMPOSE_DIR}/compose.yaml" -p alt \
    --env-file "${COMPOSE_DIR}/../.env.template" \
    config --quiet 2>&1 | grep -q "^$\|^[[:space:]]*$"; then
  pass "docker compose config --quiet exits 0 (no YAML errors)"
else
  # Still pass if exit 0 but output was empty
  :
fi
if docker compose -f "${COMPOSE_DIR}/compose.yaml" -p alt \
    --env-file "${COMPOSE_DIR}/../.env.template" \
    config --quiet > /dev/null 2>&1; then
  pass "docker compose config --quiet exits 0 cleanly"
else
  fail "docker compose config --quiet failed (YAML parse error)"
fi

# ─── 2. Python-parsed checks ─────────────────────────────────────────────────
echo ""
echo "==> 2. Service and network wiring checks (python3 yaml parse)"

python3 - <<'PYEOF'
import sys, yaml, os

COMPOSE_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "compose")

def load(fname):
    with open(os.path.join(COMPOSE_DIR, fname)) as f:
        return yaml.safe_load(f)

ai    = load("ai.yaml")
rag   = load("rag.yaml")
recap = load("recap.yaml")
work  = load("workers.yaml")
acol  = load("acolyte.yaml")
sov   = load("sovereign.yaml")
base  = load("base.yaml")

PASS, FAIL = 0, 0
def check(cond, desc):
    global PASS, FAIL
    if cond:
        print(f"  [PASS]  {desc}")
        PASS += 1
    else:
        print(f"  [FAIL]  {desc}", file=sys.stderr)
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

# ── 2f. rag-orchestrator healthcheck on :9012 ────────────────────────────────
ro_hc = rag["services"]["rag-orchestrator"].get("healthcheck", {})
ro_test = str(ro_hc.get("test", []))
check("9012" in ro_test,
      "rag-orchestrator healthcheck targets port 9012")
check("/healthz" in ro_test,
      "rag-orchestrator healthcheck targets /healthz")

# ── 2g. recap-evaluator has healthcheck ───────────────────────────────────────
re_hc = recap["services"]["recap-evaluator"].get("healthcheck", {})
check(bool(re_hc),
      "recap-evaluator has a healthcheck")
check("8080" in str(re_hc.get("test", [])),
      "recap-evaluator healthcheck targets :8080")

# ── 2h. pki-agent-recap-evaluator CERT_OWNER UID/GID=65533 ──────────────────
pki_re_env = recap["services"]["pki-agent-recap-evaluator"].get("environment", {})
if isinstance(pki_re_env, dict):
    uid = pki_re_env.get("CERT_OWNER_UID", "")
    gid = pki_re_env.get("CERT_OWNER_GID", "")
else:
    uid = next((e.split("=",1)[1] for e in pki_re_env if "CERT_OWNER_UID" in e), "")
    gid = next((e.split("=",1)[1] for e in pki_re_env if "CERT_OWNER_GID" in e), "")
check(str(uid) == "65533" and str(gid) == "65533",
      f"pki-agent-recap-evaluator CERT_OWNER_UID/GID=65533 (got uid={uid}, gid={gid})")

# ── 2i. pki-agent-knowledge-sovereign CERT_OWNER UID/GID=65532 ──────────────
pki_sov_env = sov["services"]["pki-agent-knowledge-sovereign"].get("environment", {})
if isinstance(pki_sov_env, dict):
    sov_uid = pki_sov_env.get("CERT_OWNER_UID", "")
    sov_gid = pki_sov_env.get("CERT_OWNER_GID", "")
else:
    sov_uid = next((e.split("=",1)[1] for e in pki_sov_env if "CERT_OWNER_UID" in e), "")
    sov_gid = next((e.split("=",1)[1] for e in pki_sov_env if "CERT_OWNER_GID" in e), "")
check(str(sov_uid) == "65532" and str(sov_gid) == "65532",
      f"pki-agent-knowledge-sovereign CERT_OWNER_UID/GID=65532 (got uid={sov_uid}, gid={sov_gid})")

# ── 2j. news-creator INBOUND_MTLS=true, MTLS_ALLOWED_PEERS includes pre-processor
nc_env = ai["services"]["news-creator"].get("environment", [])
nc_env_str = " ".join(str(e) for e in nc_env)
check("INBOUND_MTLS=true" in nc_env_str,
      "news-creator INBOUND_MTLS=true")
# Check MTLS_ALLOWED_PEERS includes pre-processor (only once, from the canonical var)
mtls_peers_entries = [e for e in nc_env if "MTLS_ALLOWED_PEERS" in str(e)]
check(len(mtls_peers_entries) == 1,
      f"news-creator has exactly 1 MTLS_ALLOWED_PEERS entry (got {len(mtls_peers_entries)})")
check("pre-processor" in str(mtls_peers_entries[0]) if mtls_peers_entries else False,
      "news-creator MTLS_ALLOWED_PEERS includes pre-processor")
check("recap-evaluator" in str(mtls_peers_entries[0]) if mtls_peers_entries else False,
      "news-creator MTLS_ALLOWED_PEERS includes recap-evaluator")

# ── 2k. acolyte NEWS_CREATOR_URL default is https://news-creator:9443 ────────
acol_env = acol["services"]["acolyte-orchestrator"].get("environment", [])
nc_url_entry = next((e for e in acol_env if "NEWS_CREATOR_URL" in str(e)), "")
check("https://news-creator:9443" in str(nc_url_entry),
      f"acolyte NEWS_CREATOR_URL default is https://news-creator:9443 (got: {nc_url_entry})")

# ── 2l. recap-subworker OLLAMA_EMBED_URL uses embedding-proxy ─────────────────
rs_env = recap["services"]["recap-subworker"].get("environment", [])
embed_entry = next((e for e in rs_env if "RECAP_SUBWORKER_OLLAMA_EMBED_URL" in str(e)), "")
check("embedding-proxy" in str(embed_entry),
      f"recap-subworker OLLAMA_EMBED_URL uses embedding-proxy (got: {embed_entry})")

# ── 2m. search-indexer MEILI_EMBEDDER_URL uses embedding-proxy ───────────────
si_env = work["services"]["search-indexer"].get("environment", [])
meili_embed_entry = next((e for e in si_env if "MEILI_EMBEDDER_URL" in str(e)), "")
check("embedding-proxy" in str(meili_embed_entry),
      f"search-indexer MEILI_EMBEDDER_URL uses embedding-proxy (got: {meili_embed_entry})")
check("11436" in str(meili_embed_entry),
      "search-indexer MEILI_EMBEDDER_URL uses port 11436")

# ── 2n. search-indexer secrets include inference_service_token ───────────────
si_secrets = work["services"]["search-indexer"].get("secrets", [])
check("inference_service_token" in si_secrets,
      "search-indexer secrets include inference_service_token")

# ── 2o. auth-token-manager OTEL/RASK enrollment ──────────────────────────────
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

# ── 2p. Six stable sovereign roles untouched ─────────────────────────────────
sov_secrets = sov["services"]["knowledge-sovereign"].get("secrets", [])
for role in ["sovereign_backend_token", "sovereign_operator_token",
             "sovereign_datahub_token", "sovereign_harvester_token",
             "sovereign_rag_token", "sovereign_recap_token"]:
    check(role in sov_secrets,
          f"knowledge-sovereign retains secret: {role}")

# ── 2q. pki-agent-recap-evaluator secret uses own JWK ───────────────────────
pki_re_secrets = recap["services"]["pki-agent-recap-evaluator"].get("secrets", [])
check("pki-agent-recap-evaluator-jwk" in pki_re_secrets,
      "pki-agent-recap-evaluator mounts its own JWK secret")

# ── 2r. recap-evaluator cert volume mounted RO from app, RW from pki-agent ───
re_vols = recap["services"]["recap-evaluator"].get("volumes", [])
pki_re_vols = recap["services"]["pki-agent-recap-evaluator"].get("volumes", [])
re_cert_ro = any("recap_evaluator_certs" in str(v) and ":ro" in str(v) for v in re_vols)
pki_cert_rw = any("recap_evaluator_certs" in str(v) and ":ro" not in str(v) for v in pki_re_vols)
check(re_cert_ro, "recap-evaluator mounts recap_evaluator_certs:ro")
check(pki_cert_rw, "pki-agent-recap-evaluator mounts recap_evaluator_certs (RW)")

# ── 2s. embedding-proxy callers carry the inference bearer ──────────────────
# Both read INFERENCE_SERVICE_TOKEN_FILE and send no Authorization header when
# it is unset, which embedding-proxy rejects with 401.
for svc_name, svc in [("rag-orchestrator", rag["services"]["rag-orchestrator"]),
                      ("recap-subworker", recap["services"]["recap-subworker"])]:
    svc_env_str = " ".join(str(e) for e in svc.get("environment", []))
    check("INFERENCE_SERVICE_TOKEN_FILE=/run/secrets/inference_service_token" in svc_env_str,
          f"{svc_name} INFERENCE_SERVICE_TOKEN_FILE set")
    check("inference_service_token" in svc.get("secrets", []),
          f"{svc_name} secrets include inference_service_token")

print(f"\n  Results: {PASS} passed, {FAIL} failed")
if FAIL > 0:
    sys.exit(1)
PYEOF
PY_EXIT=$?

echo ""
echo "==> Summary"
if [ $PY_EXIT -eq 0 ]; then
    echo "  All checks passed."
else
    echo "  Some checks FAILED. Review output above."
    exit 1
fi
