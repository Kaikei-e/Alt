package config

import (
	"fmt"
	"os"
	"strings"
)

const (
	minSovereignOperatorTokenLen  = 24
	sovereignOperatorTokenFileEnv = "SOVEREIGN_OPERATOR_TOKEN_FILE"
	sovereignOperatorTokenEnv     = "SOVEREIGN_OPERATOR_TOKEN"
)

// LoadSovereignOperatorToken resolves the Bearer token for knowledge-sovereign's operator methods.
// When sovereignURL is unset/empty, it returns ("", false, nil).
// When sovereignURL is set, a non-empty token of at least 24 characters is strictly required
// from SOVEREIGN_OPERATOR_TOKEN_FILE or SOVEREIGN_OPERATOR_TOKEN.
func LoadSovereignOperatorToken(sovereignURL string) (token string, enabled bool, err error) {
	if strings.TrimSpace(sovereignURL) == "" {
		return "", false, nil
	}

	if path := strings.TrimSpace(os.Getenv(sovereignOperatorTokenFileEnv)); path != "" {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", false, fmt.Errorf("read %s: %w", sovereignOperatorTokenFileEnv, readErr)
		}
		t := strings.TrimSpace(string(data))
		if len(t) < minSovereignOperatorTokenLen {
			return "", false, fmt.Errorf("sovereign operator token from %s must be at least %d characters", sovereignOperatorTokenFileEnv, minSovereignOperatorTokenLen)
		}
		return t, true, nil
	}

	if t := strings.TrimSpace(os.Getenv(sovereignOperatorTokenEnv)); t != "" {
		if len(t) < minSovereignOperatorTokenLen {
			return "", false, fmt.Errorf("%s must be at least %d characters", sovereignOperatorTokenEnv, minSovereignOperatorTokenLen)
		}
		return t, true, nil
	}

	return "", false, fmt.Errorf(
		"%s or %s is required when SOVEREIGN_URL is set (privileged operator client)",
		sovereignOperatorTokenFileEnv, sovereignOperatorTokenEnv,
	)
}

// IsPrivilegedSovereignOperatorEnabled reports whether the privileged Sovereign operator
// client token is configured via environment variables.
func IsPrivilegedSovereignOperatorEnabled() bool {
	return strings.TrimSpace(os.Getenv(sovereignOperatorTokenFileEnv)) != "" ||
		strings.TrimSpace(os.Getenv(sovereignOperatorTokenEnv)) != ""
}
