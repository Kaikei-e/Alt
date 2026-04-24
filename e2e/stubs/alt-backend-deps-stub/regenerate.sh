#!/usr/bin/env bash
# Regenerate Python proto bindings for the alt-backend-deps-stub.
#
# Source of truth: /proto/services/sovereign/v1/sovereign.proto
# Output: ./gen/services/sovereign/v1/sovereign_pb2.py (committed)
#
# The stub needs Python bindings because alt-backend's sovereign_client uses
# connect-go's default codec (application/proto, binary wire format). Without
# real proto decoding/encoding, the stub cannot answer Connect-RPC calls in
# the wire format the prod client negotiates — which is exactly what the
# e2e exercise is meant to faithfully exercise (see 25-knowledge-loop-
# transition.hurl, and the alt-backend sovereign_client.NewClient call site).
#
# Committed rather than generated at image build because:
#   1. No new Docker build dependency (protoc/buf) on the stub host
#   2. Diffs are visible in code review when sovereign.proto evolves
#   3. Matches the pattern used by alt-backend/app/gen/proto/, etc.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/../../.." && pwd)"

if ! command -v protoc >/dev/null; then
  echo "::error:: protoc not found. Install protobuf compiler and retry." >&2
  exit 1
fi

# Pin protoc to the 29.x series so the emitted gencode stays compatible with
# the `protobuf==5.29.x` runtime pinned in the Dockerfile. Without this check,
# a developer with a newer protoc silently produces gencode that exceeds the
# runtime pin and the stub container dies on import with
#   google.protobuf.runtime_version.VersionError: gencode X.Y runtime X.Z
# (which is exactly the regression we caught in alt-deploy run 24886851507).
# Bump both sides together when upgrading — never one without the other.
protoc_ver="$(protoc --version | awk '{print $2}')"
if ! [[ "$protoc_ver" =~ ^29\.[0-9]+$ ]]; then
  echo "::error:: need protoc 29.x (matches protobuf Python 5.29.x pinned in Dockerfile); got '$protoc_ver'." >&2
  echo "::error:: To upgrade: bump Dockerfile 'protobuf==5.29.x' and this regex together." >&2
  exit 1
fi

cd "$REPO/proto"
protoc --python_out="$HERE/gen" services/sovereign/v1/sovereign.proto

# Ensure package imports resolve (protoc does not emit __init__.py).
for d in \
  "$HERE/gen" \
  "$HERE/gen/services" \
  "$HERE/gen/services/sovereign" \
  "$HERE/gen/services/sovereign/v1" \
; do
  : > "$d/__init__.py"
done

echo "Regenerated: $HERE/gen/services/sovereign/v1/sovereign_pb2.py"
