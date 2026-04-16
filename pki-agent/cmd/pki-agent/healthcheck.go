package main

import (
	"fmt"
	"net"
	"net/http"
	"time"
)

// runHealthcheck is the logical body of the `pki-agent healthcheck` Docker
// command. It is testable in isolation (healthcheck() wraps it with
// os.Exit). Two probes run in order:
//
//  1. GET http://<metricsAddr>/healthz — verifies the rotator's own view is
//     green. This is the pre-existing behaviour.
//  2. When proxyListen is non-empty, TCP-dial 127.0.0.1:<port>. This is the
//     net-new probe that catches the reverse-proxy goroutine silently dying
//     while the outer process (and therefore the container) stays alive.
//     Without it, a dead :9443 listener leaves the sidecar marked healthy
//     and Docker never restarts it — exactly the failure mode that bounced
//     AcolyteService/ListReports to 502 in production.
//
// Returns nil when both probes succeed; the first error otherwise.
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
	defer resp.Body.Close()
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
