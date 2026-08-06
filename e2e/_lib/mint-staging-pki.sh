#!/usr/bin/env bash
# e2e/_lib/mint-staging-pki.sh
#
# Mint a throwaway CA plus one leaf per peer for the staging slices that speak
# to alt-data-hub over mutual TLS.
#
# Why this exists as a lib rather than inline in one run.sh
# --------------------------------------------------------
# After ADR-000954 Wave 3, alt-data-hub owns alt_db and every other binary in
# the family reaches it over mTLS. `config.LoadDataHubClientConfig`
# (alt-backend / alt-harvester) and `config.loadDataHub` (rag-orchestrator)
# both fail-fast on missing certificate material — deliberately, since there is
# no plaintext route to fall back to (CLAUDE.md rules 8/9). That makes a client
# leaf a *boot* requirement of four staging slices, not just of the alt-data-hub
# suite, so the minting has to be shared.
#
# Usage
# -----
#   source "$ROOT/e2e/_lib/mint-staging-pki.sh"
#   mint_staging_pki "$PKI_DIR" alt-data-hub alt-backend alt-harvester
#
# The first peer name is the *server*: it gets a serverAuth leaf, and its
# key pair is additionally copied to svc-cert.pem / svc-key.pem so
# alt-data-hub can keep the production DATAHUB_TLS_* paths verbatim
# (compose/core.yaml points them at /certs/svc-cert.pem, /certs/svc-key.pem,
# /trust/ca-bundle.pem). Staging TLS wiring that differs from production is
# TLS wiring that can rot.
#
# Every remaining name gets a clientAuth leaf at <name>.pem / <name>-key.pem.
# CN *and* DNS SAN are both set to the name: tlsutil.WithAllowedPeers accepts
# either, and pinning both keeps the fixture honest whichever one the
# allowlist ends up reading.
#
# Security
# --------
# These are one-day keys for an ephemeral, `internal: true` network. The
# directory has to sit under e2e/fixtures/ so compose can bind-mount it, which
# puts private keys one `git add e2e/` away from a commit — hence the entries
# in e2e/.gitignore. Callers wipe the directory in their EXIT trap.

set -euo pipefail

# mint_staging_leaf <pki_dir> <name> <serverAuth|clientAuth>
mint_staging_leaf() {
  local dir="$1" name="$2" eku="$3"
  openssl req -newkey rsa:2048 -nodes \
    -subj "/CN=$name" \
    -keyout "$dir/$name-key.pem" \
    -out "$dir/$name.csr" >/dev/null 2>&1
  openssl x509 -req -days 1 -sha256 \
    -in "$dir/$name.csr" \
    -CA "$dir/ca.pem" -CAkey "$dir/ca-key.pem" -CAcreateserial \
    -extfile <(printf 'subjectAltName=DNS:%s\nextendedKeyUsage=%s\nbasicConstraints=CA:FALSE\n' "$name" "$eku") \
    -out "$dir/$name.pem" >/dev/null 2>&1
  rm -f "$dir/$name.csr"
}

# mint_staging_pki <pki_dir> <server_name> [client_name...]
mint_staging_pki() {
  local dir="$1" server="$2"
  shift 2

  command -v openssl >/dev/null 2>&1 || {
    echo "openssl not found — required to mint the throwaway mTLS fixtures" >&2
    return 1
  }

  echo "==> minting throwaway mTLS fixtures under $dir" >&2
  rm -rf "$dir"
  mkdir -p "$dir"
  openssl req -x509 -newkey rsa:2048 -nodes -days 1 -sha256 \
    -subj "/CN=alt-e2e-ca" \
    -keyout "$dir/ca-key.pem" -out "$dir/ca.pem" >/dev/null 2>&1

  mint_staging_leaf "$dir" "$server" serverAuth
  local peer
  for peer in "$@"; do
    mint_staging_leaf "$dir" "$peer" clientAuth
  done

  # Production filenames for the server leaf and the root (see the header).
  cp "$dir/$server.pem"     "$dir/svc-cert.pem"
  cp "$dir/$server-key.pem" "$dir/svc-key.pem"
  cp "$dir/ca.pem"          "$dir/ca-bundle.pem"

  # The containers run as a non-root uid and mount this directory read-only.
  # These are one-day throwaway keys for an ephemeral network, so world-read
  # is the right trade against a chown that would need root on the host.
  chmod 0644 "$dir"/*.pem
}
