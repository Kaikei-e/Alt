#!/usr/bin/env bash
# e2e/hurl/alt-data-hub/run.sh
#
# Brings up the alt-data-hub slice of the alt-staging stack (Postgres +
# Atlas migrator + alt-backend-deps-stub + alt-data-hub), runs the Hurl
# suite inside the alt-staging network, and tears the stack down.
#
# alt-data-hub is the data plane of the alt-backend split:
# alt.datahub.v1.DataHubService, served over mTLS on :9443 and nothing
# else. (The admin Connect services did *not* move here — they stay on
# alt-backend's loopback operator listener; see di/datahub/container.go.)
# It publishes no host port at all, which forces the same shape as the
# mq-hub / knowledge-sovereign / alt-backend suites: Hurl runs in a container
# attached to the staging network, because that is the only place the service
# is addressable.
#
# What is different here is the credential. There is no JWT: the client
# certificate is the caller's identity, so this script mints a throwaway
# PKI before bringing the stack up and hands the leaf to Hurl on the
# command line. Certificates are minted per run and never committed.
#
# Environment overrides:
#   DATA_HUB_URL           — alt-data-hub mTLS URL as seen from the Hurl
#                            container (default: https://alt-data-hub:9443)
#   OPS_URL                — alt-data-hub operator listener, plaintext and
#                            unpublished (default: http://alt-data-hub:9110)
#   ALLOWED_PEER           — CN/SAN of the leaf used by the positive suite;
#                            must appear in alt-data-hub's MTLS_ALLOWED_PEERS
#                            (default: pre-processor)
#   DENIED_PEER            — CN/SAN of the leaf used by the negative probe;
#                            must NOT appear there (default: rogue-peer)
#   HURL_IMAGE             — Hurl container image (default: ghcr.io/orange-opensource/hurl:7.1.0)
#   IMAGE_TAG              — Docker tag for the alt-data-hub image (default: ci)
#   GHCR_OWNER             — GHCR namespace (default: kaikei-e)
#   RUN_ID                 — unique run identifier (default: $(date +%s))
#   STAGING_PROJECT_NAME   — compose project + network name (default: alt-staging).
#                            CI sets alt-staging-alt-data-hub so parallel matrix
#                            jobs on the shared daemon don't collide.
#   KEEP_STACK=1           — do not tear the stack down on exit (for debugging)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
cd "$ROOT"

: "${DATA_HUB_URL:=https://alt-data-hub:9443}"
: "${OPS_URL:=http://alt-data-hub:9110}"
: "${ALLOWED_PEER:=pre-processor}"
: "${DENIED_PEER:=rogue-peer}"
: "${HURL_IMAGE:=ghcr.io/orange-opensource/hurl:7.1.0}"
: "${IMAGE_TAG:=ci}"
: "${GHCR_OWNER:=kaikei-e}"
: "${RUN_ID:=$(date +%s)}"
: "${STAGING_PROJECT_NAME:=alt-staging}"

# Per-service image tags, mirroring the alt-backend suite: compose.staging
# .yaml keys each GHCR image off its own `<SERVICE>_IMAGE_TAG` env var so
# unrelated dependency services stay on the last successful main build.
: "${ALT_DATA_HUB_IMAGE_TAG:=$IMAGE_TAG}"
: "${ALT_BACKEND_DEPS_STUB_IMAGE_TAG:=$IMAGE_TAG}"
# ALLOWED_PEER is exported so `docker compose config` in render-slice.sh
# resolves DATAHUB_ALLOWED_PEERS to the same name mint_leaf uses below. If the
# two ever drifted, every positive scenario would fail in the TLS handshake
# with no assertion to point at the cause.
export IMAGE_TAG GHCR_OWNER STAGING_PROJECT_NAME ALT_DATA_HUB_IMAGE_TAG ALT_BACKEND_DEPS_STUB_IMAGE_TAG ALLOWED_PEER

# shellcheck source=../_lib/render-slice.sh
source "$ROOT/e2e/hurl/_lib/render-slice.sh"
render_slice alt-data-hub

# shellcheck source=../_lib/reclaim-network-pool.sh
source "$ROOT/e2e/hurl/_lib/reclaim-network-pool.sh"
reclaim_network_pool

# shellcheck source=../_lib/compose-up-with-retry.sh
source "$ROOT/e2e/hurl/_lib/compose-up-with-retry.sh"

# shellcheck source=../_lib/assert-transport-refused.sh
source "$ROOT/e2e/hurl/_lib/assert-transport-refused.sh"

REPORT_DIR="$ROOT/e2e/reports/alt-data-hub-$RUN_ID"
mkdir -p "$REPORT_DIR"

# Throwaway PKI for this run. It has to sit under e2e/fixtures/ so compose
# can bind-mount it into the container, which also puts private keys one
# `git add e2e/` away from a commit — hence the entry in e2e/.gitignore.
# The trap wipes it on the way out, except under KEEP_STACK=1: the
# certificates are bind-mounted into a container that is still running, so
# deleting them would break the very stack the flag exists to inspect.
PKI_DIR="$ROOT/e2e/fixtures/alt-data-hub/pki"

cleanup() {
  local exit_code=$?
  if [[ "$exit_code" -ne 0 && "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> dumping $STAGING_PROJECT_NAME container logs (exit=$exit_code)" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      logs --no-color --tail=500 >&2 2>&1 || true
  fi
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    echo "==> tearing down $STAGING_PROJECT_NAME stack" >&2
    docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" \
      down -v --remove-orphans >/dev/null 2>&1 || true
    rm -rf "$PKI_DIR"
  else
    echo "==> KEEP_STACK=1 — leaving $STAGING_PROJECT_NAME stack up" >&2
    echo "==> KEEP_STACK=1 — leaving $PKI_DIR in place (mounted by the container)" >&2
  fi
  # $SLICE_DIR is under mktemp -d; always clean up, even when KEEP_STACK=1,
  # so resolved compose config doesn't linger (render-slice.sh, audit F-005).
  rm -rf "$SLICE_DIR"
}
trap cleanup EXIT

command -v openssl >/dev/null 2>&1 || {
  echo "openssl not found — required to mint the throwaway mTLS fixtures" >&2
  exit 1
}

# mint_leaf <name> <serverAuth|clientAuth>
#
# CN *and* DNS SAN are both set to <name>: tlsutil.WithAllowedPeers accepts
# either, and pinning both keeps this fixture honest whichever one the
# allowlist ends up reading. The server leaf's SAN doubles as the hostname
# Hurl verifies, so <name> must equal the compose service name.
mint_leaf() {
  local name="$1" eku="$2"
  openssl req -newkey rsa:2048 -nodes \
    -subj "/CN=$name" \
    -keyout "$PKI_DIR/$name-key.pem" \
    -out "$PKI_DIR/$name.csr" >/dev/null 2>&1
  openssl x509 -req -days 1 -sha256 \
    -in "$PKI_DIR/$name.csr" \
    -CA "$PKI_DIR/ca.pem" -CAkey "$PKI_DIR/ca-key.pem" -CAcreateserial \
    -extfile <(printf 'subjectAltName=DNS:%s\nextendedKeyUsage=%s\nbasicConstraints=CA:FALSE\n' "$name" "$eku") \
    -out "$PKI_DIR/$name.pem" >/dev/null 2>&1
  rm -f "$PKI_DIR/$name.csr"
}

echo "==> minting throwaway mTLS fixtures under e2e/fixtures/alt-data-hub/pki" >&2
rm -rf "$PKI_DIR"
mkdir -p "$PKI_DIR"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
  -subj "/CN=alt-e2e-ca" \
  -keyout "$PKI_DIR/ca-key.pem" -out "$PKI_DIR/ca.pem" >/dev/null 2>&1
mint_leaf alt-data-hub serverAuth
mint_leaf "$ALLOWED_PEER" clientAuth
mint_leaf "$DENIED_PEER" clientAuth
# Second names for the server leaf and the root, matching the filenames the
# pki-agent sidecar publishes in production (compose/core.yaml points
# alt-data-hub at /certs/svc-cert.pem, /certs/svc-key.pem and
# /trust/ca-bundle.pem). Mounting this directory at both paths then works
# with the production environment values unchanged, so the staging slice
# does not need its own TLS wiring to drift from.
cp "$PKI_DIR/alt-data-hub.pem"     "$PKI_DIR/svc-cert.pem"
cp "$PKI_DIR/alt-data-hub-key.pem" "$PKI_DIR/svc-key.pem"
cp "$PKI_DIR/ca.pem"               "$PKI_DIR/ca-bundle.pem"
# The container runs as a non-root uid and mounts this directory read-only.
# These are one-day throwaway keys for an ephemeral network, so world-read
# is the right trade against a chown that would need root on the host.
chmod 0644 "$PKI_DIR"/*.pem

echo "==> bringing up alt-data-hub slice ($STAGING_PROJECT_NAME)" >&2
if ! compose_up_with_retry \
    alt-backend-db \
    alt-backend-db-migrator \
    alt-backend-deps-stub \
    alt-data-hub; then
  echo "==> compose up failed — dumping logs before trap cleanup" >&2
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" ps -a >&2 2>&1 || true
  docker compose -f "$SLICE" -p "$STAGING_PROJECT_NAME" logs --no-color --tail=500 >&2 2>&1 || true
  exit 1
fi

hurl_run() {
  docker run --rm \
    --network "$STAGING_PROJECT_NAME" \
    -v "$ROOT:$ROOT" \
    -w "$ROOT" \
    "$HURL_IMAGE" \
    "$@"
}

common_vars=(
  --variable "data_hub_url=$DATA_HUB_URL"
  --variable "ops_url=$OPS_URL"
  --variable "run_id=$RUN_ID"
)

# Client identity for the positive suite. Passed on the command line rather
# than in per-entry [Options] so no scenario file can silently fall back to
# an anonymous connection and still report green.
allowed_peer_tls=(
  --cacert "$PKI_DIR/ca.pem"
  --cert   "$PKI_DIR/$ALLOWED_PEER.pem"
  --key    "$PKI_DIR/$ALLOWED_PEER-key.pem"
)

echo "==> running Hurl setup (serial; readiness probe over mTLS)" >&2
hurl_run --test \
  --file-root "$ROOT" \
  "${allowed_peer_tls[@]}" \
  "${common_vars[@]}" \
  e2e/hurl/alt-data-hub/00-setup.hurl

echo "==> asserting the mTLS boundary refuses everything else" >&2

# No client certificate. MTLS_CLIENT_AUTH=require_and_verify makes Go reject
# this during the handshake, before any HTTP is spoken — which is the point:
# an unauthenticated caller never reaches a route, so there is no route-level
# mistake that could expose one.
assert_transport_refused "anonymous TLS client (no certificate)" \
  --file-root "$ROOT" \
  --cacert "$PKI_DIR/ca.pem" \
  --variable "probe_url=$DATA_HUB_URL/health" \
  e2e/hurl/_lib/probe-transport-refused.hurl

# Valid chain, wrong identity. tlsutil.WithAllowedPeers installs a
# VerifyConnection callback, so this is also a handshake failure rather than
# a 403 — service B cannot impersonate service A even holding a leaf from
# the same internal CA.
assert_transport_refused "peer outside MTLS_ALLOWED_PEERS ($DENIED_PEER)" \
  --file-root "$ROOT" \
  --cacert "$PKI_DIR/ca.pem" \
  --cert   "$PKI_DIR/$DENIED_PEER.pem" \
  --key    "$PKI_DIR/$DENIED_PEER-key.pem" \
  --variable "probe_url=$DATA_HUB_URL/health" \
  e2e/hurl/_lib/probe-transport-refused.hurl

# "mTLS only" means there is no second door. These are the ports the
# monolith listened on; nothing may be bound to them in this container, so
# a plaintext client gets connection-refused rather than a service.
for plain_port in 9000 9101 9102; do
  assert_transport_refused "no plaintext listener on :$plain_port" \
    --file-root "$ROOT" \
    --variable "probe_url=http://alt-data-hub:$plain_port/health" \
    e2e/hurl/_lib/probe-transport-refused.hurl
done

echo "==> running Hurl suite" >&2
# --jobs 1: the suite is read-only, but keeping it serial makes the mTLS
# handshake failures in a broken run readable in order.
shopt -s nullglob
suite_files=(
  e2e/hurl/alt-data-hub/0[1-9]-*.hurl
  e2e/hurl/alt-data-hub/[1-9][0-9]-*.hurl
)
shopt -u nullglob

hurl_run --test \
  --jobs 1 \
  --file-root "$ROOT" \
  "${allowed_peer_tls[@]}" \
  "${common_vars[@]}" \
  --report-junit "$REPORT_DIR/junit.xml" \
  --report-html  "$REPORT_DIR/html" \
  "${suite_files[@]}"

echo "==> suite passed. reports: $REPORT_DIR" >&2
