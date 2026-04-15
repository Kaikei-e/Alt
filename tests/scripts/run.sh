#!/usr/bin/env bash
# Runs every test_*.sh in this directory. Zero external deps.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
FAIL=0

for t in "$HERE"/test_*.sh; do
  echo ""
  echo "================================================"
  echo " $(basename "$t")"
  echo "================================================"
  if ! bash "$t"; then
    FAIL=$((FAIL+1))
  fi
done

echo ""
if (( FAIL > 0 )); then
  echo "===> $FAIL test file(s) failed"
  exit 1
fi
echo "===> All test files passed"
