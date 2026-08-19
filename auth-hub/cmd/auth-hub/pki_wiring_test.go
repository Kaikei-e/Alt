package main

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestMainWiresPKIStartBeforeTLS(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	body, err := os.ReadFile(strings.TrimSuffix(thisFile, "pki_wiring_test.go") + "main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	pkiIdx := strings.Index(src, "pki.Start(")
	if pkiIdx < 0 {
		t.Fatal("main.go must call pki.Start before TLS init")
	}
	serverIdx := strings.Index(src, "tlsutil.LoadServerConfig(")
	if serverIdx >= 0 && pkiIdx > serverIdx {
		t.Fatal("pki.Start must run before tlsutil.LoadServerConfig")
	}
	if !strings.Contains(src, `"auth-hub"`) {
		t.Fatal("pki.Start must be called with CERT_SUBJECT auth-hub")
	}
	if !strings.Contains(src, "pki.ListenOps(") {
		t.Fatal("main.go must start the dedicated PKI ops listener")
	}
	if !strings.Contains(src, "pki.ShutdownOps(") {
		t.Fatal("main.go must shut down the dedicated PKI ops listener")
	}
	if strings.Contains(src, "mountOpsMetrics") || strings.Contains(src, `e.GET("/metrics"`) {
		t.Fatal("auth-hub must not expose PKI metrics on the application mux")
	}
	if strings.Contains(src, "prometheus.DefaultRegisterer") {
		t.Fatal("PKI metrics must not use prometheus.DefaultRegisterer")
	}
}
