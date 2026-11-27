#!/usr/bin/env bash
set -euo pipefail

WORKSPACE_DIR="${WORKSPACE_DIR:-$(pwd)}"
TARGET="$WORKSPACE_DIR/pkg/middleware/errors.go"

if [[ ! -f "$TARGET" ]]; then
  echo "FAIL: missing target file $TARGET" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cat >"$TMP_DIR/go.mod" <<'EOF'
module middlewarecheck

go 1.24
EOF

cp "$TARGET" "$TMP_DIR/errors.go"

cat >"$TMP_DIR/errors_test.go" <<'EOF'
package middleware

import (
	"errors"
	"testing"
)

func TestRecordServerErrorThreshold(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantTrip bool
	}{
		{name: "nil error", err: nil, wantTrip: false},
		{name: "plain error", err: errors.New("boom"), wantTrip: false},
		{name: "client 400", err: &HTTPError{Code: 400, Message: "bad request"}, wantTrip: false},
		{name: "client 404", err: &HTTPError{Code: 404, Message: "not found"}, wantTrip: false},
		{name: "client 499", err: &HTTPError{Code: 499, Message: "closed"}, wantTrip: false},
		{name: "server 500", err: &HTTPError{Code: 500, Message: "internal"}, wantTrip: true},
		{name: "server 503", err: &HTTPError{Code: 503, Message: "unavailable"}, wantTrip: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			trips := 0
			recordServerError(tc.err, func() { trips++ })
			if tc.wantTrip && trips != 1 {
				t.Fatalf("expected 1 recorded failure for %s, got %d", tc.name, trips)
			}
			if !tc.wantTrip && trips != 0 {
				t.Fatalf("expected 0 recorded failures for %s, got %d", tc.name, trips)
			}
		})
	}
}
EOF

(
	cd "$TMP_DIR"
	GOPROXY=off GOSUMDB=off go test -count=1 ./...
)
