package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	minRAGAPITokenLen  = 24
	ragAPIAuthEnv      = "RAG_API_AUTH"
	ragAPITokenFileEnv = "RAG_API_TOKEN_FILE"
	ragAPITokenEnv     = "RAG_API_TOKEN"
	ragAPIAuthDisabled = "disabled"
)

// LoadRAGAuth resolves the bearer token for rag-orchestrator's :9010 listener.
// Absence is a startup failure unless RAG_API_AUTH=disabled is explicitly set.
func LoadRAGAuth() (token string, enabled bool, err error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(ragAPIAuthEnv)), ragAPIAuthDisabled) {
		return "", false, nil
	}

	if path := strings.TrimSpace(os.Getenv(ragAPITokenFileEnv)); path != "" {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", false, fmt.Errorf("read %s: %w", ragAPITokenFileEnv, readErr)
		}
		t := strings.TrimSpace(string(data))
		if len(t) < minRAGAPITokenLen {
			return "", false, fmt.Errorf("rag api token from %s must be at least %d characters", ragAPITokenFileEnv, minRAGAPITokenLen)
		}
		return t, true, nil
	}

	if t := strings.TrimSpace(os.Getenv(ragAPITokenEnv)); t != "" {
		if len(t) < minRAGAPITokenLen {
			return "", false, fmt.Errorf("%s must be at least %d characters", ragAPITokenEnv, minRAGAPITokenLen)
		}
		return t, true, nil
	}

	return "", false, fmt.Errorf(
		"%s or %s is required; set %s=%s to call rag-orchestrator without authentication",
		ragAPITokenFileEnv, ragAPITokenEnv, ragAPIAuthEnv, ragAPIAuthDisabled)
}

// ValidateRAGAuthConfig validates and resolves the RAG API authentication configuration.
// If OrchestratorURL is set, a non-empty token is required unless RAG_API_AUTH=disabled.
func ValidateRAGAuthConfig(cfg *RAGConfig, appEnv string) error {
	if strings.TrimSpace(cfg.OrchestratorURL) == "" {
		return nil
	}

	if strings.EqualFold(strings.TrimSpace(cfg.APIAuth), ragAPIAuthDisabled) {
		if appEnv == "production" {
			return fmt.Errorf("RAG_API_AUTH=disabled is not permitted in production: rag-orchestrator caller authentication is mandatory")
		}
		cfg.APIToken = ""
		return nil
	}

	if cfg.APITokenFile != "" {
		if cfg.APIToken == "" {
			data, err := os.ReadFile(cfg.APITokenFile)
			if err != nil {
				return fmt.Errorf("read %s: %w", ragAPITokenFileEnv, err)
			}
			cfg.APIToken = strings.TrimSpace(string(data))
		}
		if len(cfg.APIToken) < minRAGAPITokenLen {
			return fmt.Errorf("rag api token from %s must be at least %d characters", ragAPITokenFileEnv, minRAGAPITokenLen)
		}
		return nil
	}

	cfg.APIToken = strings.TrimSpace(cfg.APIToken)
	if cfg.APIToken != "" {
		if len(cfg.APIToken) < minRAGAPITokenLen {
			return fmt.Errorf("%s must be at least %d characters", ragAPITokenEnv, minRAGAPITokenLen)
		}
		return nil
	}

	return fmt.Errorf(
		"%s or %s is required when RAG_ORCHESTRATOR_URL is set; set %s=%s to call rag-orchestrator without authentication",
		ragAPITokenFileEnv, ragAPITokenEnv, ragAPIAuthEnv, ragAPIAuthDisabled)
}
