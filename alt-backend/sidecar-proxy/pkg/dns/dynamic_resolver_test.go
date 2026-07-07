package dns

import (
	"regexp"
	"testing"
	"time"
)

func staticPatterns(literals ...string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(literals))
	for _, l := range literals {
		patterns = append(patterns, regexp.MustCompile("^"+regexp.QuoteMeta(l)+"$"))
	}
	return patterns
}

// TestIsDomainAllowed_AutoLearnRestrictedToSubdomainsOfStaticAllowlist guards
// against the regression where shouldLearnDomain auto-allowed any domain
// ending in a common TLD (.com/.org/.net/.ai/.co/.dev/.io) or starting with
// a common prefix (api./cdn./feeds.), which defeated the CONNECT /
// persistent-tunnel allowlist entirely.
func TestIsDomainAllowed_AutoLearnRestrictedToSubdomainsOfStaticAllowlist(t *testing.T) {
	resolver := NewDynamicResolver(staticPatterns("zenn.dev", "github.com"), []string{"8.8.8.8:53"}, 5*time.Minute, 100)

	cases := []struct {
		name        string
		domain      string
		wantAllowed bool
		wantLearned bool
	}{
		{"exact static match", "zenn.dev", true, false},
		{"genuine subdomain of statically allowed domain", "api.zenn.dev", true, true},
		{"arbitrary .com domain must not be auto-learned", "totally-unrelated-site.com", false, false},
		{"arbitrary .dev domain must not be auto-learned", "evil.dev", false, false},
		{"generic api. prefix must not be auto-learned on its own", "api.evil-tracker.net", false, false},
		{"generic feeds. prefix must not be auto-learned on its own", "feeds.attacker.io", false, false},
		{"lookalike suffix is not a subdomain", "notzenn.dev", false, false},
		{"appended suffix is not a subdomain", "zenn.dev.evil.com", false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			allowed, learned := resolver.IsDomainAllowed(tc.domain)
			if allowed != tc.wantAllowed || learned != tc.wantLearned {
				t.Errorf("IsDomainAllowed(%q) = (allowed=%v, learned=%v), want (allowed=%v, learned=%v)",
					tc.domain, allowed, learned, tc.wantAllowed, tc.wantLearned)
			}
		})
	}
}

func TestShouldLearnDomain_NoStaticAllowlist(t *testing.T) {
	resolver := NewDynamicResolver(nil, []string{"8.8.8.8:53"}, 5*time.Minute, 100)

	if resolver.shouldLearnDomain("anything.com") {
		t.Error("shouldLearnDomain() = true with no static allowlist configured, want false")
	}
}
