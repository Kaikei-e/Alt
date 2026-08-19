package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMainWiresPKIBeforeTLSConsumers(t *testing.T) {
	src := readMainSource(t)
	enroll := strings.Index(src, "pki.StartWithRegisterer(")
	tlsLoad := strings.Index(src, "tlsutil.LoadServerConfig(")
	connectListen := strings.Index(src, "ListenAndServeTLS(")
	if enroll < 0 {
		t.Fatal("cmd/server must call pki.StartWithRegisterer")
	}
	if !strings.Contains(src, `"rag-orchestrator"`) {
		t.Fatal("pki start must be called with subject rag-orchestrator")
	}
	if strings.Contains(src, "pki.Start(") {
		t.Fatal("pki.Start uses DefaultRegisterer; rag must use StartWithRegisterer")
	}
	if !strings.Contains(src, "prometheus.NewRegistry()") {
		t.Fatal("PKI metrics must use a private registry")
	}
	if !strings.Contains(src, "pki.NewOpsHandler(") || !strings.Contains(src, "pki.LoadOpsListenAddr(") {
		t.Fatal("PKI metrics must be served on the dedicated ops listener")
	}
	if strings.Contains(src, "NewPromObserver") {
		t.Fatal("composition root must not construct PromObserver against DefaultRegisterer")
	}
	if tlsLoad < 0 || enroll > tlsLoad {
		t.Fatal("enrollment must start before tlsutil.LoadServerConfig")
	}
	if connectListen < 0 || enroll > connectListen {
		t.Fatal("enrollment must start before the Connect-RPC TLS listener")
	}
}

func readMainSource(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), "main.go")
	body, err := os.ReadFile(path) // #nosec G304 -- test fixture path from runtime.Caller
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}
