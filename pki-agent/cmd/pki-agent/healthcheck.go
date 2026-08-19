package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// runHealthcheck is the logical body of the `pki-agent healthcheck` Docker
// command. It is testable in isolation (healthcheck() wraps it with
// os.Exit). Probes run in order:
//
//  1. GET http://<metricsAddr>/healthz — verifies the rotator's own view is
//     green. This is the pre-existing behaviour.
//  2. When proxyListen is non-empty, TCP-dial 127.0.0.1:<port> to catch the
//     reverse-proxy goroutine silently dying while the outer process stays
//     alive. Wave 4 Pattern B cutover left no compose service setting
//     PROXY_LISTEN; this branch is retained for the binary's proxy mode.
//
// Returns nil when every probe succeeds; the first error otherwise.
func runHealthcheck(metricsAddr, proxyListen string) error {
	if err := probeMetrics(metricsAddr); err != nil {
		return err
	}
	if proxyListen != "" {
		if err := probeProxy(proxyListen); err != nil {
			return err
		}
	}
	return nil
}

func probeMetrics(addr string) error {
	if addr == "" {
		addr = ":9510"
	}
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	url := "http://" + addr + "/healthz"
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("metrics /healthz: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("metrics /healthz: status %d", resp.StatusCode)
	}
	return nil
}

func probeProxy(listen string) error {
	addr := listen
	if addr[0] == ':' {
		addr = "127.0.0.1" + addr
	}
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return fmt.Errorf("proxy listener %s: %w", addr, err)
	}
	_ = conn.Close()
	return nil
}
