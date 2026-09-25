package config

import (
	"net"
	"os"
	"strings"
)

// opsListenEnv names the listener every alt-backend binary opens.
const opsListenEnv = "OPS_LISTEN"

// defaultOpsListenAddr keeps a `go run ./cmd/...` on a laptop off the LAN.
// compose overrides it with ":9110" so Prometheus can scrape over alt-network.
const defaultOpsListenAddr = "127.0.0.1:9110"

// LoadOpsListenAddr returns the bind address of the ops listener shared by
// cmd/backend, cmd/harvester and cmd/datahub.
//
// One port, one shape, three binaries: /health for the compose probe and the
// healthcheck subcommand, /metrics for the three scrape jobs in
// observability/prometheus/prometheus.yml. Nothing else is mounted on it, so
// unlike the operator listener there is no admin surface for a wide bind to
// expose — ":9110" is the normal deployment value, not an accident.
//
// It is also why data-hub can be scraped at all: its data plane speaks mTLS,
// and giving Prometheus a client certificate would put a monitoring identity
// in DATAHUB_ALLOWED_PEERS.
func LoadOpsListenAddr() (string, error) {
	addr := strings.TrimSpace(os.Getenv(opsListenEnv))
	if addr == "" {
		return defaultOpsListenAddr, nil
	}
	if err := validateHostPort(opsListenEnv, addr); err != nil {
		return "", err
	}
	return addr, nil
}

// ListenReach describes how far a bind address reaches, for the startup log.
type ListenReach string

const (
	// ReachLoopback: only a process inside this network namespace can connect.
	ReachLoopback ListenReach = "loopback_only"
	// ReachNetwork: anything that can route to the container can connect —
	// every other container on alt-network, and the host if the port is
	// published.
	ReachNetwork ListenReach = "container_network"
)

// ListenAddrReach classifies a bind address. An address it cannot parse is
// reported as ReachNetwork: the log line exists to warn, and guessing
// "loopback" for something unrecognised would make it lie in the one direction
// that matters.
func ListenAddrReach(addr string) ListenReach {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil {
		return ReachNetwork
	}
	if host == "" {
		return ReachNetwork
	}
	if host == "localhost" {
		return ReachLoopback
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ReachLoopback
	}
	return ReachNetwork
}
