package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the service configuration.
type Config struct {
	DatabaseURL string
	ListenAddr  string
	MetricsAddr string
	// AdminToken is required as a Bearer token on the /admin/* endpoints
	// (snapshots/retention/storage) served on MetricsAddr. It is only empty
	// when AdminAuthEnabled is false, which requires ADMIN_AUTH=disabled to
	// be set explicitly: an unset token is a startup failure, not an open
	// door (Rule 9).
	AdminToken string
	// AdminAuthEnabled reports whether the Bearer gate is active. main.go
	// logs it loudly at startup so "forgot to set it" and "intentionally
	// open" are never indistinguishable (Rule 8).
	AdminAuthEnabled bool

	// EventToken is required as a Bearer token on the :9500 Connect-RPC listener
	// (services.sovereign.v1). It is only empty when EventAuthEnabled is false,
	// which requires EVENT_AUTH=disabled to be set explicitly: an unset token
	// is a startup failure, not an open door (Rule 9).
	EventToken string
	// EventAuthEnabled reports whether the Bearer gate is active on :9500. main.go
	// logs it loudly at startup so "forgot to set it" and "intentionally
	// open" are never indistinguishable (Rule 8).
	EventAuthEnabled bool

	// Snapshot / retention filesystem paths and build identity.
	SnapshotDir   string
	ArchiveDir    string
	BuildRef      string
	SchemaVersion string

	// Projector / planner tick intervals and batch sizes.
	ProjectorTickInterval        time.Duration
	BranchPlannerTickInterval    time.Duration
	ProjectionHealthTickInterval time.Duration
	TrailProjectorBatchSize      int
	TrailProjectorMaxBatches     int
	HomeProjectorBatchSize       int
	HomeProjectorMaxBatches      int
	TrailMaxBranchesPerUser      int
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	listenAddr := os.Getenv("LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":9500"
	}

	metricsAddr := os.Getenv("METRICS_ADDR")
	if metricsAddr == "" {
		metricsAddr = ":9501"
	}

	snapshotDir := os.Getenv("SNAPSHOT_DIR")
	if snapshotDir == "" {
		snapshotDir = "/data/snapshots"
	}
	archiveDir := os.Getenv("ARCHIVE_DIR")
	if archiveDir == "" {
		archiveDir = "/tmp/archives"
	}
	buildRef := os.Getenv("BUILD_REF")
	if buildRef == "" {
		buildRef = "dev"
	}

	adminToken, adminAuthEnabled, err := loadAdminAuth()
	if err != nil {
		return nil, err
	}

	eventToken, eventAuthEnabled, err := loadEventAuth()
	if err != nil {
		return nil, err
	}

	return &Config{
		DatabaseURL:                  dbURL,
		ListenAddr:                   listenAddr,
		MetricsAddr:                  metricsAddr,
		AdminToken:                   adminToken,
		AdminAuthEnabled:             adminAuthEnabled,
		EventToken:                   eventToken,
		EventAuthEnabled:             eventAuthEnabled,
		SnapshotDir:                  snapshotDir,
		ArchiveDir:                   archiveDir,
		BuildRef:                     buildRef,
		SchemaVersion:                "00009",
		ProjectorTickInterval:        parseDurationEnv("KNOWLEDGE_SOVEREIGN_PROJECTOR_TICK_INTERVAL", 5*time.Second),
		BranchPlannerTickInterval:    parseDurationEnv("KNOWLEDGE_SOVEREIGN_BRANCH_PLANNER_TICK_INTERVAL", 30*time.Second),
		ProjectionHealthTickInterval: parseDurationEnv("KNOWLEDGE_SOVEREIGN_PROJECTION_HEALTH_TICK_INTERVAL", 60*time.Second),
		TrailProjectorBatchSize:      parseIntEnv("KNOWLEDGE_SOVEREIGN_TRAIL_PROJECTOR_BATCH_SIZE", 500),
		TrailProjectorMaxBatches:     parseIntEnv("KNOWLEDGE_SOVEREIGN_TRAIL_PROJECTOR_MAX_BATCHES_PER_TICK", 4),
		HomeProjectorBatchSize:       parseIntEnv("KNOWLEDGE_SOVEREIGN_HOME_PROJECTOR_BATCH_SIZE", 500),
		HomeProjectorMaxBatches:      parseIntEnv("KNOWLEDGE_SOVEREIGN_HOME_PROJECTOR_MAX_BATCHES_PER_TICK", 4),
		TrailMaxBranchesPerUser:      parseIntEnv("KNOWLEDGE_SOVEREIGN_TRAIL_MAX_BRANCHES_PER_USER", 5),
	}, nil
}

// minAdminTokenLen keeps the Bearer token out of guessable range; the admin
// surface exports partitions and writes snapshots to a host bind mount.
const minAdminTokenLen = 24

// minEventTokenLen enforces the same entropy requirement on the event-log
// listener, which accepts durable state mutations.
const minEventTokenLen = 24

// loadAdminAuth resolves the admin Bearer token from ADMIN_TOKEN_FILE (docker
// secret) or ADMIN_TOKEN. Absence is a startup failure: the only way to run
// without the gate is ADMIN_AUTH=disabled, spelled out by an operator.
func loadAdminAuth() (string, bool, error) {
	if os.Getenv("ADMIN_AUTH") == "disabled" {
		return "", false, nil
	}

	if path := os.Getenv("ADMIN_TOKEN_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false, fmt.Errorf("read ADMIN_TOKEN_FILE: %w", err)
		}
		token := strings.TrimSpace(string(data))
		if len(token) < minAdminTokenLen {
			return "", false, fmt.Errorf("admin token from ADMIN_TOKEN_FILE must be at least %d characters", minAdminTokenLen)
		}
		return token, true, nil
	}

	if token := strings.TrimSpace(os.Getenv("ADMIN_TOKEN")); token != "" {
		if len(token) < minAdminTokenLen {
			return "", false, fmt.Errorf("ADMIN_TOKEN must be at least %d characters", minAdminTokenLen)
		}
		return token, true, nil
	}

	return "", false, fmt.Errorf(
		"ADMIN_TOKEN or ADMIN_TOKEN_FILE is required; set ADMIN_AUTH=disabled to run the /admin/* endpoints without authentication")
}

// loadEventAuth resolves the event-log Bearer token from EVENT_TOKEN_FILE
// (docker secret) or EVENT_TOKEN. Absence is a startup failure: the only way to run
// without the gate is EVENT_AUTH=disabled, spelled out by an operator.
func loadEventAuth() (string, bool, error) {
	if os.Getenv("EVENT_AUTH") == "disabled" {
		return "", false, nil
	}

	if path := os.Getenv("EVENT_TOKEN_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", false, fmt.Errorf("read EVENT_TOKEN_FILE: %w", err)
		}
		token := strings.TrimSpace(string(data))
		if len(token) < minEventTokenLen {
			return "", false, fmt.Errorf("event token from EVENT_TOKEN_FILE must be at least %d characters", minEventTokenLen)
		}
		return token, true, nil
	}

	if token := strings.TrimSpace(os.Getenv("EVENT_TOKEN")); token != "" {
		if len(token) < minEventTokenLen {
			return "", false, fmt.Errorf("EVENT_TOKEN must be at least %d characters", minEventTokenLen)
		}
		return token, true, nil
	}

	return "", false, fmt.Errorf(
		"EVENT_TOKEN or EVENT_TOKEN_FILE is required; set EVENT_AUTH=disabled to run the event-log RPC listener without authentication")
}

// parseDurationEnv reads a duration from env, falling back to the supplied
// default. Negative or unparseable values fall back without error so a
// misconfigured operator override does not crash the service.
func parseDurationEnv(name string, fallback time.Duration) time.Duration {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		slog.Warn("invalid duration env, using fallback", "env", name, "value", v, "fallback", fallback.String())
		return fallback
	}
	return d
}

func parseIntEnv(name string, fallback int) int {
	v := os.Getenv(name)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil || i <= 0 {
		slog.Warn("invalid int env, using fallback", "env", name, "value", v, "fallback", fallback)
		return fallback
	}
	return i
}
