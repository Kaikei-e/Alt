#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TARGET="$SCRIPT_DIR/cmd/server/main.go"

if [[ ! -f "$TARGET" ]]; then
  echo "FAIL: missing target file $TARGET" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$TMP_DIR/httpserver_stub"

cat >"$TMP_DIR/go.mod" <<'EOF'
module edgeservercheck

go 1.24

require alt/httpserver v0.0.0

replace alt/httpserver => ./httpserver_stub
EOF

cat >"$TMP_DIR/httpserver_stub/go.mod" <<'EOF'
module alt/httpserver

go 1.24
EOF

cat >"$TMP_DIR/httpserver_stub/server.go" <<'EOF'
package httpserver

import (
	"net/http"
	"time"
)

type Config struct {
	Addr              string
	Handler           http.Handler
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
}

func New(cfg Config) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           cfg.Handler,
		ReadHeaderTimeout: cfg.ReadHeaderTimeout,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
}
EOF

cp "$TARGET" "$TMP_DIR/main.go"

cat >"$TMP_DIR/main_test.go" <<'EOF'
package main

import (
	"testing"
	"time"
)

func TestPlaintextServerTimeouts(t *testing.T) {
	srv := newPlaintextServer(":8080", nil)
	if srv == nil {
		t.Fatal("expected non-nil server")
	}
	if srv.Addr != ":8080" {
		t.Fatalf("Addr=%q, want :8080", srv.Addr)
	}
	if srv.ReadHeaderTimeout != 5*time.Second {
		t.Fatalf("ReadHeaderTimeout=%v, want 5s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 0 {
		t.Fatalf("unexpected IdleTimeout=%v; must remain unset (0)", srv.IdleTimeout)
	}
	if srv.ReadTimeout != 0 || srv.WriteTimeout != 0 {
		t.Fatalf("unexpected ReadTimeout=%v WriteTimeout=%v; must remain unset (0)", srv.ReadTimeout, srv.WriteTimeout)
	}
}
EOF

(
	cd "$TMP_DIR"
	GOPROXY=off GOSUMDB=off go test -count=1 ./...
)
