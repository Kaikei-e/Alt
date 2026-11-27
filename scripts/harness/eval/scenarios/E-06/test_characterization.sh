#!/usr/bin/env bash
set -euo pipefail

WORKSPACE_DIR="${WORKSPACE_DIR:-$(pwd)}"
TARGET="$WORKSPACE_DIR/internal/openapi/server.gen.go"

if [[ ! -f "$TARGET" ]]; then
  echo "FAIL: missing target file $TARGET" >&2
  exit 1
fi

if ! grep -q 'oapi-codegen/v2 version v2\.5\.1' "$TARGET"; then
  echo "FAIL: expected pinned oapi-codegen v2.5.1 header in $TARGET" >&2
  exit 1
fi

if grep -q 'github\.com/labstack/echo/v5' "$TARGET"; then
  echo "FAIL: generated file with v2.5.1 header was hand-edited to echo/v5" >&2
  exit 1
fi

if ! grep -q 'github\.com/labstack/echo/v4' "$TARGET"; then
  echo "FAIL: expected echo/v4 import matching v2.5.1 generator output" >&2
  exit 1
fi

if grep -q '\*echo\.Context' "$TARGET"; then
  echo "FAIL: generated file signature was hand-patched to *echo.Context under v2.5.1 header" >&2
  exit 1
fi
