#!/bin/bash
# Alt スモークテスト - デプロイ後の補足ヘルスチェック
#
# c2quay's own smoke step (deploy.smoke in c2quay.yml -> scripts/smoke.sh)
# already checks http://localhost/health (nginx), :9000/v1/health (backend),
# :9250/health (BFF) and :7700/health (Meilisearch) on every c2quay deploy.
# This script only runs the checks c2quay's smoke does NOT cover — currently
# the frontend-sv service reached directly (bypassing nginx) — so
# deploy-local.sh doesn't double-run the same probes after c2quay deploy.
set -e

RUNTIME_HOST="${ALT_RUNTIME_HOST:-localhost}"
TIMEOUT="${ALT_SMOKE_TIMEOUT:-5}"
PASS=0
FAIL=0

check() {
    local url="$1"
    local name="$2"
    if curl -sf --max-time "$TIMEOUT" "$url" > /dev/null 2>&1; then
        echo "  OK: $name ($url)"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $name ($url)"
        FAIL=$((FAIL + 1))
    fi
}

echo "=== Alt Supplementary Smoke Tests ==="
echo "Host: $RUNTIME_HOST"
echo ""

check "http://$RUNTIME_HOST:4173/sv/health"     "Frontend SV (direct, bypassing nginx)"

echo ""
echo "Results: $PASS passed, $FAIL failed"

if [ "$FAIL" -gt 0 ]; then
    echo "Some smoke tests failed!"
    exit 1
fi

echo "All smoke tests passed."
