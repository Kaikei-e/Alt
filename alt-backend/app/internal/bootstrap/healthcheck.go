package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

// The `healthcheck` subcommand exists because the runtime image has no shell
// and no wget (alt-backend/Dockerfile.backend drops both, which also means an
// attacker who lands in the container has neither). compose therefore probes
// with `test: ["CMD", "/app-entry", "healthcheck"]`: the same binary, invoked
// a second time, self-probes over loopback and exits 0 or 1.
//
// It runs before MustBoot on purpose. A probe that first opened the database
// pool and initialised OpenTelemetry would report the process unhealthy
// whenever a dependency was slow, and would do it once per interval for the
// life of the container.

// healthcheckArg is the single subcommand this binary accepts. Anything else
// falls through to the server.
const healthcheckArg = "healthcheck"

// healthcheckTimeout bounds the whole probe. It must stay under the tightest
// `timeout:` any compose healthcheck gives it (5s in compose/core.yaml's
// alt-data-hub and compose.staging.yaml's alt-backend) so a slow probe surfaces
// as our error rather than as Docker killing the process mid-request.
const healthcheckTimeout = 3 * time.Second

// IsHealthcheckInvocation reports whether argv asks for the self-probe.
//
// Only argv[1] counts. Accepting the word anywhere in the arguments would mean
// a future flag whose value happened to be "healthcheck" silently turned a
// server start into a probe that exits immediately.
func IsHealthcheckInvocation(args []string) bool {
	return len(args) > 1 && args[1] == healthcheckArg
}

// HealthcheckOptions describes what a healthy process looks like from inside
// its own container.
type HealthcheckOptions struct {
	// OpsAddr is the ops listener's *bind* address (the OPS_LISTEN value).
	// DialAddr turns it into something dialable.
	OpsAddr string
	// TCPTargets are additional listeners that must be accepting connections,
	// given as bind addresses. cmd/datahub passes DATAHUB_LISTEN_ADDR: its
	// mutual-TLS listener cannot be probed with HTTP without minting the probe
	// a client certificate, and a process whose mTLS goroutine has died still
	// answers /health perfectly well (ADR-000784). Connect and close — no
	// handshake, because completing one would mean adding alt-data-hub to its
	// own DATAHUB_ALLOWED_PEERS and muddying what that list means.
	TCPTargets []string
}

// Healthcheck probes the current process and returns nil when it is healthy.
func Healthcheck(ctx context.Context, opts HealthcheckOptions) error {
	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()

	target, err := DialAddr(opts.OpsAddr)
	if err != nil {
		return fmt.Errorf("resolve ops listener address: %w", err)
	}

	if err := probeHealth(ctx, "http://"+target+"/health"); err != nil {
		return err
	}

	var dialer net.Dialer
	for _, bind := range opts.TCPTargets {
		addr, err := DialAddr(bind)
		if err != nil {
			return fmt.Errorf("resolve tcp target %q: %w", bind, err)
		}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("dial %s: %w", addr, err)
		}
		if err := conn.Close(); err != nil {
			return fmt.Errorf("close probe connection to %s: %w", addr, err)
		}
	}

	return nil
}

func probeHealth(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build health request for %s: %w", url, err)
	}

	// A dedicated client, not http.DefaultClient: the probe is a one-shot
	// process and must not inherit a package-global transport's keep-alives or
	// (absent) timeout.
	client := &http.Client{Timeout: healthcheckTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probe %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe %s: status %d, want 200", url, resp.StatusCode)
	}
	return nil
}

// DialAddr converts a listener's bind address into one a client in the same
// netns can dial.
//
// compose sets OPS_LISTEN=":9110" so Prometheus can reach the listener over
// alt-network, and the probe runs inside that same container. "" and "0.0.0.0"
// are bind-side shorthand for "every interface" and are not dialable hosts, so
// they collapse to loopback; "::" likewise becomes "::1". Every other host —
// an explicit loopback IP, a hostname, a routable address — is already a dial
// target and passes through unchanged.
func DialAddr(bind string) (string, error) {
	host, port, err := net.SplitHostPort(bind)
	if err != nil {
		return "", fmt.Errorf("%q is not a valid host:port: %w", bind, err)
	}
	if port == "" {
		return "", errors.New("bind address has no port: " + bind)
	}

	switch host {
	case "", "0.0.0.0":
		host = "127.0.0.1"
	case "::":
		host = "::1"
	}
	return net.JoinHostPort(host, port), nil
}
