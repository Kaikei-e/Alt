package config

import (
	"fmt"
	"strings"
)

// ValidateNotifierConfig rejects a dispatcher that could start but not deliver.
//
// Every value here fails silently at runtime if it is absent. A missing signing
// key produces a 401 from every push service; a missing or malformed subject
// produces a 403 from Apple alone, which reads as "iOS stopped working". None
// of it raises an error anyone sees, and the queue drains into nothing while
// every dashboard stays green — so this is a startup failure instead
// (CLAUDE.md rule 9).
func ValidateNotifierConfig(cfg *Config) error {
	required := []struct {
		env   string
		value string
	}{
		{"VAPID_PRIVATE_KEY", cfg.WebPush.PrivateKey},
		{"VAPID_SUBJECT", cfg.WebPush.Subject},
	}
	if err := requireAll("notifier", required); err != nil {
		return err
	}

	subject := strings.TrimSpace(cfg.WebPush.Subject)
	// RFC 8292 §2: `sub` SHOULD be a contact URI. Apple enforces it, and does
	// so only on its own endpoints, so an unchecked value here becomes a
	// partial outage that looks platform-specific rather than a config error.
	if !strings.HasPrefix(subject, "mailto:") && !strings.HasPrefix(subject, "https://") {
		return fmt.Errorf(
			"VAPID_SUBJECT must be a mailto: or https: URI (RFC 8292 §2); Apple rejects anything else and Google and Mozilla do not, so this fails on iOS only")
	}

	return nil
}
