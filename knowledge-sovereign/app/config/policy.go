package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ServicePolicy defines the allowed RPC methods, event types, and scope constraints
// for a single microservice principal.
type ServicePolicy struct {
	Name             string          `json:"name"`
	Token            string          `json:"token,omitempty"`
	TokenFile        string          `json:"token_file,omitempty"`
	AllowedMethods   map[string]bool `json:"-"`
	AllowedEvents    map[string]bool `json:"-"`
	RequireUserToken bool            `json:"require_user_token"`
	// AllowSystemScope declares residual authority for data-plane replication and
	// background projector maintenance. These scopes are intentionally privileged
	// and MUST NOT be claimed as end-user bindings.
	AllowSystemScope bool `json:"allow_system_scope"`

	RawAllowedMethods []string `json:"allowed_methods"`
	RawAllowedEvents  []string `json:"allowed_events"`
}

// AuthPolicy holds the complete set of per-service credential and capability policies.
type AuthPolicy struct {
	Version  int             `json:"version"`
	Services []ServicePolicy `json:"services"`
}

// FindServiceByName looks up a service policy by its unique name.
func (p *AuthPolicy) FindServiceByName(name string) *ServicePolicy {
	for i := range p.Services {
		if p.Services[i].Name == name {
			return &p.Services[i]
		}
	}
	return nil
}

// LoadPolicyFile reads and validates a JSON capability policy file.
// It detects duplicate tokens, empty tokens, and tokens with insufficient entropy (< 24 chars),
// returning sanitized errors that never leak secret token material.
func LoadPolicyFile(path string) (*AuthPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file %s: %w", path, err)
	}

	var pol AuthPolicy
	if err := json.Unmarshal(data, &pol); err != nil {
		return nil, fmt.Errorf("parse policy file %s: %w", path, err)
	}

	if len(pol.Services) == 0 {
		return nil, fmt.Errorf("policy file %s contains no services", path)
	}

	seenServices := make(map[string]bool, len(pol.Services))
	seenTokens := make(map[string]string, len(pol.Services)) // token -> service name

	for i := range pol.Services {
		svc := &pol.Services[i]
		name := strings.TrimSpace(svc.Name)
		if name == "" {
			return nil, fmt.Errorf("policy validation failed: service at index %d has empty name", i)
		}
		if seenServices[name] {
			return nil, fmt.Errorf("policy validation failed: duplicate service name %q", name)
		}
		seenServices[name] = true

		if svc.TokenFile != "" {
			tokData, err := os.ReadFile(svc.TokenFile)
			if err != nil {
				return nil, fmt.Errorf("read token_file for service %q (%s): %w", name, svc.TokenFile, err)
			}
			svc.Token = strings.TrimSpace(string(tokData))
		} else {
			svc.Token = strings.TrimSpace(svc.Token)
		}

		if svc.Token == "" {
			return nil, fmt.Errorf("policy validation failed: service %q has empty token", name)
		}

		if len(svc.Token) < minEventTokenLen {
			return nil, fmt.Errorf("policy validation failed: service %q token is shorter than minimum length %d", name, minEventTokenLen)
		}

		if existingSvc, exists := seenTokens[svc.Token]; exists {
			return nil, fmt.Errorf("policy validation failed: duplicate token detected between services %q and %q", existingSvc, name)
		}
		seenTokens[svc.Token] = name

		svc.AllowedMethods = make(map[string]bool, len(svc.RawAllowedMethods))
		for _, m := range svc.RawAllowedMethods {
			trimmed := strings.TrimSpace(m)
			if trimmed != "" {
				svc.AllowedMethods[trimmed] = true
			}
		}

		svc.AllowedEvents = make(map[string]bool, len(svc.RawAllowedEvents))
		for _, e := range svc.RawAllowedEvents {
			trimmed := strings.TrimSpace(e)
			if trimmed != "" {
				svc.AllowedEvents[trimmed] = true
			}
		}
	}

	return &pol, nil
}
