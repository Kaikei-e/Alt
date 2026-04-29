#!/usr/bin/env bash
# One-shot bootstrap of a Pact Broker `record-deployment` row for a pacticipant
# that was just promoted into c2quay.yml `production.services` but has never
# been deployed under that pacticipant identity yet. Without this, the very
# first run of `c2quay deploy` aborts at the can-i-deploy gate because every
# downstream consumer's pact resolves the new provider's deployed version to
# `unknown` (broker has no record-deployment row to anchor the matrix on).
#
# This is a sibling of `record-remote-pacticipant.sh` but with a different
# responsibility: the remote variant exists for pacticipants that live off
# this host (tts-speaker on a separate GPU host) so c2quay never gets to
# them; this one exists for pacticipants that DO run on this host but were
# only just registered so c2quay's own record-deployment hasn't happened yet.
# Once seeded, c2quay picks up `record-deployment` on subsequent rolls and
# this script is no longer needed.
#
# Usage:
#   PACTICIPANT=knowledge-sovereign \
#   VERSION=$(git rev-parse --short HEAD) \
#   ENVIRONMENT=production \
#     ./scripts/seed-pacticipant-deployment.sh
#
# Env:
#   PACTICIPANT             (required) pacticipant name as it appears in c2quay.yml
#   VERSION                 (required) version string already published to the broker
#                                       (consumer pact ref OR provider verification ref)
#   ENVIRONMENT             (default: production)
#   PACT_BROKER_BIN         (default: pact-broker-cli)
#   PACT_BROKER_BASE_URL    (default: http://localhost:9292)
#   PACT_BROKER_USERNAME    (default: pact)
#   PACT_BROKER_PASSWORD    (default: read from secrets/pact_broker_basic_auth_password.txt)
#
# Idempotency:
#   `pact-broker-cli record-deployment` for the same pacticipant + version +
#   environment is a no-op on the broker side (it returns the existing
#   deployed-version row). Running this script twice is safe.
set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -z "${PACTICIPANT:-}" ]]; then
  echo "ERROR: PACTICIPANT env var is required" >&2
  exit 2
fi
if [[ -z "${VERSION:-}" ]]; then
  echo "ERROR: VERSION env var is required" >&2
  exit 2
fi

ENVIRONMENT="${ENVIRONMENT:-production}"
PACT_BROKER_BIN="${PACT_BROKER_BIN:-pact-broker-cli}"
PACT_BROKER_BASE_URL="${PACT_BROKER_BASE_URL:-http://localhost:9292}"
PACT_BROKER_USERNAME="${PACT_BROKER_USERNAME:-pact}"

if [[ -z "${PACT_BROKER_PASSWORD:-}" ]] && [[ -r "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt" ]]; then
  PACT_BROKER_PASSWORD="$(tr -d '\n' < "$REPO_ROOT/secrets/pact_broker_basic_auth_password.txt")"
fi
if [[ -z "${PACT_BROKER_PASSWORD:-}" ]]; then
  echo "ERROR: PACT_BROKER_PASSWORD not set and secrets/pact_broker_basic_auth_password.txt not readable" >&2
  exit 2
fi

echo "==> record-deployment ${PACTICIPANT} @ ${VERSION} -> ${ENVIRONMENT}"
"$PACT_BROKER_BIN" record-deployment \
  --pacticipant "$PACTICIPANT" \
  --version "$VERSION" \
  --environment "$ENVIRONMENT" \
  --broker-base-url "$PACT_BROKER_BASE_URL" \
  --broker-username "$PACT_BROKER_USERNAME" \
  --broker-password "$PACT_BROKER_PASSWORD"
