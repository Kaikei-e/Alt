package main

import (
	"errors"
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
//  2. When proxyListen is non-empty, probeNetns checks for at least one
//     non-loopback interface that is UP with a non-loopback address. This
//     catches the netns-orphan failure mode (ADR-000782): the sidecar's
//     parent was force-recreated, the sidecar still holds the stale
//     container id in HostConfig.NetworkMode, and eth0 has been torn down
//     inside the shared netns. A bare TCP probe to 127.0.0.1 misses this
//     because the sidecar's own listener is still alive in the orphaned
//     netns — the detector has to look at interfaces, not at the port.
//  3. When proxyListen is non-empty, TCP-dial 127.0.0.1:<port> to catch the
//     reverse-proxy goroutine silently dying while the outer process stays
//     alive. This is the ADR-000784 probe; it runs after the netns check
//     so a more specific error is surfaced when the root cause is orphan
//     rather than listener death.
//
// Returns nil when every probe succeeds; the first error otherwise.
func runHealthcheck(metricsAddr, proxyListen string) error {
	if err := probeMetrics(metricsAddr); err != nil {
		return err
	}
	if proxyListen != "" {
		if err := probeNetns(); err != nil {
			return err
		}
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

// ifaceInfo is a test-friendly projection of net.Interface. The net package
// exposes Addrs() as a method on the concrete Interface struct, which means
// the only seam we can replace in tests is the whole enumeration step —
// hence this intermediate value type.
type ifaceInfo struct {
	Name     string
	IsUp     bool
	Loopback bool
	Addrs    []net.IP
}

// listInterfaces enumerates the current netns. Swap this var in tests to
// simulate orphaned netns, downed eth0, or kernel enumeration failures.
var listInterfaces = func() ([]ifaceInfo, error) {
	raw, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]ifaceInfo, 0, len(raw))
	for _, i := range raw {
		info := ifaceInfo{
			Name:     i.Name,
			IsUp:     i.Flags&net.FlagUp != 0,
			Loopback: i.Flags&net.FlagLoopback != 0,
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				info.Addrs = append(info.Addrs, ipn.IP)
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// probeNetns returns nil when the sidecar's netns has at least one
// non-loopback interface that is UP and carries a non-loopback address.
//
// Background: pki-agent sidecars use `network_mode: service:<parent>` to
// share the parent container's netns, which is how the reverse-proxy
// terminates TLS on `:9443` from inside that netns. When the parent is
// force-recreated (docker compose up -d with an image diff), the sidecar
// keeps the stale parent container id in HostConfig.NetworkMode and ends
// up in a netns where eth0 has been torn down — `ip -4 addr` shows only
// `lo`. The reverse-proxy is still listening on 127.0.0.1:9443 inside
// that dead netns, but no traffic can reach it because there's no route
// to the sidecar from the rest of the alt-network. A TCP probe to
// 127.0.0.1 from the sidecar itself would succeed, so the netns check has
// to happen at a higher level: enumerate interfaces and require at least
// one routable path out.
//
// Detection is local (no outbound I/O, no DNS, no exec) so the probe works
// identically in tests and never produces new attack surface.
func probeNetns() error {
	ifs, err := listInterfaces()
	if err != nil {
		return fmt.Errorf("netns interfaces: %w", err)
	}
	for _, i := range ifs {
		if i.Loopback {
			continue
		}
		if !i.IsUp {
			continue
		}
		for _, ip := range i.Addrs {
			if ip == nil {
				continue
			}
			if ip.IsLoopback() || ip.IsUnspecified() {
				continue
			}
			return nil
		}
	}
	return errors.New("netns orphan: no non-loopback interface up with a routable address")
}
