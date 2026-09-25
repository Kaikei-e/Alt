package config

import (
	"fmt"
	"os"
	"strings"
)

// operatorAuthEnv, operatorTokenFileEnv and operatorTokenEnv name the bearer
// token cmd/backend's operator listener requires on every RPC, mirroring the
// ADMIN_AUTH / ADMIN_TOKEN_FILE / ADMIN_TOKEN shape knowledge-sovereign uses
// for its own admin surface.
const (
	operatorAuthEnv      = "OPERATOR_AUTH"
	operatorTokenFileEnv = "OPERATOR_TOKEN_FILE"
	operatorTokenEnv     = "OPERATOR_TOKEN"
	operatorAuthDisabled = "disabled"
	minOperatorTokenLen  = 24
)

// LoadOperatorAuth resolves the bearer token cmd/backend's operator listener
// interceptor compares every Authorization header against.
//
// Absence is a startup failure (CLAUDE.md rule 9): the admin RPCs mounted
// here (KnowledgeHomeAdminService, AdminMonitorService) mutate durable state
// and are reachable from any container on alt-network once the loopback bind
// is widened for altctl/the BFF, so an unset token must never be silently
// read as "run open". The only way to run without the gate is
// OPERATOR_AUTH=disabled, spelled out by an operator — cmd/backend logs
// operator_auth_enabled/operator_auth_disabled at startup either way
// (rule 8).
func LoadOperatorAuth() (token string, enabled bool, err error) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv(operatorAuthEnv)), operatorAuthDisabled) {
		return "", false, nil
	}

	if path := strings.TrimSpace(os.Getenv(operatorTokenFileEnv)); path != "" {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return "", false, fmt.Errorf("read %s: %w", operatorTokenFileEnv, readErr)
		}
		token := strings.TrimSpace(string(data))
		if len(token) < minOperatorTokenLen {
			return "", false, fmt.Errorf("operator token from %s must be at least %d characters", operatorTokenFileEnv, minOperatorTokenLen)
		}
		return token, true, nil
	}

	if token := strings.TrimSpace(os.Getenv(operatorTokenEnv)); token != "" {
		if len(token) < minOperatorTokenLen {
			return "", false, fmt.Errorf("%s must be at least %d characters", operatorTokenEnv, minOperatorTokenLen)
		}
		return token, true, nil
	}

	return "", false, fmt.Errorf(
		"%s or %s is required; set %s=%s to run the operator listener without authentication",
		operatorTokenFileEnv, operatorTokenEnv, operatorAuthEnv, operatorAuthDisabled)
}
