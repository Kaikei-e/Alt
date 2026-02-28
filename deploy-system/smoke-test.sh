#!/bin/bash
# Alt スモークテスト - デプロイ後の全サービスヘルスチェック
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

echo "=== Alt Smoke Tests ==="
echo "Host: $RUNTIME_HOST"
echo ""

check "http://$RUNTIME_HOST:9000/v1/health"    "Backend (REST)"
check "http://$RUNTIME_HOST:4173/sv/health"     "Frontend SV"
check "http://$RUNTIME_HOST:7700/health"        "Meilisearch"

echo ""
echo "Results: $PASS passed, $FAIL failed"

if [ "$FAIL" -gt 0 ]; then
    echo "Some smoke tests failed!"
    exit 1
fi

echo "All smoke tests passed."
