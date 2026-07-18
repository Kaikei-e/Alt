package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// Config holds the service configuration.
type Config struct {
	DatabaseURL string
	ListenAddr  string
	MetricsAddr string
	// AdminToken, if set, is required as a Bearer token on the mutating
	// /admin/* endpoints (snapshots/retention/storage) served on MetricsAddr.
	// Empty means admin auth is explicitly disabled — main.go logs this
	// loudly at startup so "forgot to set it" and "intentionally open" are
	// never indistinguishable (Rule 8).
	AdminToken string

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

	return &Config{
		DatabaseURL:                  dbURL,
		ListenAddr:                   listenAddr,
		MetricsAddr:                  metricsAddr,
		AdminToken:                   os.Getenv("ADMIN_TOKEN"),
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
