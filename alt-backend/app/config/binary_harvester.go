package config

import (
	"fmt"
	"log/slog"
)

// ValidateHarvesterConfig checks the upstreams the scheduled jobs call.
//
// SOVEREIGN_URL is required in every environment, unlike the backend's
// production-only check: the outbox worker appends knowledge events through
// the sovereign client, and a disabled client no-ops every append while the
// worker still marks the outbox row PROCESSED. That silently violates the
// append-first invariant instead of failing visibly.
func ValidateHarvesterConfig(cfg *Config) error {
	required := []struct {
		env   string
		value string
	}{
		{"SOVEREIGN_URL", cfg.Sovereign.URL},
		{"RAG_ORCHESTRATOR_URL", cfg.Rag.OrchestratorURL},
	}
	if err := requireAll("harvester", required); err != nil {
		return err
	}
	if err := ValidateRAGAuthConfig(&cfg.Rag, cfg.AppEnv); err != nil {
		return fmt.Errorf("harvester config: %w", err)
	}
	if cfg.Rag.APIToken != "" {
		slog.Info("rag_api_auth_enabled", "binary", "harvester")
	} else {
		slog.Warn("rag_api_auth_disabled", "binary", "harvester", "reason", "RAG_API_AUTH=disabled (explicit opt-out)")
	}
	return nil
}
