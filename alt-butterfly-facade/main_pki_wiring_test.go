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
	body, err := os.ReadFile(strings.TrimSuffix(thisFile, "_pki_wiring_test.go") + ".go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	mainIdx := strings.Index(src, "func main()")
	if mainIdx < 0 {
		t.Fatal("main.go missing func main")
	}
	mainSrc := src[mainIdx:]
	pkiIdx := strings.Index(mainSrc, "pki.Start(")
	if pkiIdx < 0 {
		t.Fatal("main.go must call pki.Start before TLS init")
	}
	clientIdx := strings.Index(mainSrc, "newMTLSBackendTransport()")
	serverIdx := strings.Index(mainSrc, "tlsutil.LoadServerConfig(")
	if clientIdx >= 0 && pkiIdx > clientIdx {
		t.Fatal("pki.Start must run before newMTLSBackendTransport")
	}
	if serverIdx >= 0 && pkiIdx > serverIdx {
		t.Fatal("pki.Start must run before tlsutil.LoadServerConfig")
	}
	if !strings.Contains(mainSrc, `"alt-butterfly-facade"`) {
		t.Fatal("pki.Start must be called with CERT_SUBJECT alt-butterfly-facade")
	}
	if !strings.Contains(mainSrc, "pki.ListenOps(") {
		t.Fatal("main.go must start the dedicated PKI ops listener")
	}
	if !strings.Contains(mainSrc, "pki.ShutdownOps(") {
		t.Fatal("main.go must shut down the dedicated PKI ops listener")
	}
	if strings.Contains(mainSrc, "prometheus.DefaultRegisterer") {
		t.Fatal("PKI metrics must not use prometheus.DefaultRegisterer")
	}
}
