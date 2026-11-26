#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TARGET="$SCRIPT_DIR/server.go"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$TMP_DIR/echo_stub/middleware"

cat >"$TMP_DIR/go.mod" <<'EOF'
module c15char

go 1.24

require github.com/labstack/echo/v5 v5.0.0

replace github.com/labstack/echo/v5 => ./echo_stub
EOF

cat >"$TMP_DIR/echo_stub/go.mod" <<'EOF'
module github.com/labstack/echo/v5

go 1.24
EOF

cat >"$TMP_DIR/echo_stub/echo.go" <<'EOF'
package echo

type Context struct {
	BodyLen int64
	Status  int
}

type HandlerFunc func(*Context) error
type MiddlewareFunc func(HandlerFunc) HandlerFunc

type Route struct {
	Method  string
	Path    string
	Handler HandlerFunc
}

type Group struct {
	prefix      string
	middlewares []MiddlewareFunc
	echo        *Echo
}

type Echo struct {
	routes []Route
}

func New() *Echo { return &Echo{} }

func (e *Echo) Group(prefix string) *Group {
	return &Group{prefix: prefix, echo: e}
}

func (g *Group) Use(mw ...MiddlewareFunc) {
	g.middlewares = append(g.middlewares, mw...)
}

func (g *Group) POST(path string, h HandlerFunc) {
	wrapped := h
	for i := len(g.middlewares) - 1; i >= 0; i-- {
		wrapped = g.middlewares[i](wrapped)
	}
	g.echo.routes = append(g.echo.routes, Route{
		Method:  "POST",
		Path:    g.prefix + path,
		Handler: wrapped,
	})
}

func (e *Echo) ServePOST(path string, bodyLen int64) int {
	for _, r := range e.routes {
		if r.Method == "POST" && r.Path == path {
			c := &Context{BodyLen: bodyLen, Status: 200}
			_ = r.Handler(c)
			return c.Status
		}
	}
	return 404
}
EOF

cat >"$TMP_DIR/echo_stub/middleware/body_limit.go" <<'EOF'
package middleware

import "github.com/labstack/echo/v5"

func BodyLimit(limit int64) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if c.BodyLen > limit {
				c.Status = 413
				return nil
			}
			return next(c)
		}
	}
}
EOF

cp "$TARGET" "$TMP_DIR/server.go"

cat >"$TMP_DIR/server_characterization_test.go" <<'EOF'
package bootstrap

import (
	"testing"

	"github.com/labstack/echo/v5"
)

func TestBodyLimitDecimalOneMegabyteBoundary(t *testing.T) {
	e := echo.New()
	registerSummarizeGroup(e, func(c *echo.Context) error {
		c.Status = 200
		return nil
	})

	if got := e.ServePOST("/api/v1/summarize", 1_000_000); got != 200 {
		t.Fatalf("1,000,000 byte body status=%d, want 200", got)
	}
	if got := e.ServePOST("/api/v1/summarize", 1_000_001); got != 413 {
		t.Fatalf("1,000,001 byte body status=%d, want 413 (decimal 1M boundary drifted)", got)
	}
}
EOF

(
	cd "$TMP_DIR"
	GOPROXY=off GOSUMDB=off go test -count=1 ./...
)
