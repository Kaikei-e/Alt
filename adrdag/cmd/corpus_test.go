//go:build corpus

package cmd

import (
	"os"
	"regexp"
	"testing"

	"github.com/alt-project/adrdag/internal/adr"
)

const realCorpus = "../../docs/ADR"

// TestRealCorpusLoad asserts LoadDir count against a count derived from the
// directory listing itself — no hardcoded corpus size (the adversarial design
// review refuted a `>= 951` magic-number baseline: template.md is excluded by
// the NNNNNN stem filter, so raw file count and loaded count always differ).
func TestRealCorpusLoad(t *testing.T) {
	entries, err := os.ReadDir(realCorpus)
	if err != nil {
		t.Skipf("real corpus not available: %v", err)
	}
	stemRE := regexp.MustCompile(`^\d{6}\.md$`)
	wantCount := 0
	for _, e := range entries {
		if !e.IsDir() && stemRE.MatchString(e.Name()) {
			wantCount++
		}
	}
	adrs, err := adr.LoadDir(realCorpus)
	if err != nil {
		t.Fatal(err)
	}
	if len(adrs) != wantCount {
		t.Errorf("loaded %d ADRs, want %d (= NNNNNN.md files in %s)", len(adrs), wantCount, realCorpus)
	}
}

// TestRealCorpusStructuralInvariants asserts only structural invariants, not
// counts: the live corpus must stay free of cycles, dangling refs, empty
// stubs, and status drift. WARN findings (orphan superseded) are allowed.
func TestRealCorpusStructuralInvariants(t *testing.T) {
	adrs, err := adr.LoadDir(realCorpus)
	if err != nil {
		t.Skipf("real corpus not available: %v", err)
	}
	report := runCheck(adrs)
	for _, f := range report.Findings {
		if f.Severity == "error" {
			t.Errorf("real corpus check error: %s", f.Detail)
		}
	}
	if !report.OK {
		t.Error("real corpus check failed")
	}
}

// TestRealCorpusBindingPartition: every ADR is either binding, or
// non-accepted, or accepted-with-inbound (drift, which the invariant test
// above already rejects) — the binding set plus superseded/proposed statuses
// must exactly partition the corpus.
func TestRealCorpusBindingPartition(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", realCorpus, "binding")
	if code != exitOK {
		t.Fatalf("binding exit = %d", code)
	}
	lineRE := regexp.MustCompile(`(?m)^\d{6}\t`)
	got := len(lineRE.FindAllString(stdout, -1))

	adrs, err := adr.LoadDir(realCorpus)
	if err != nil {
		t.Fatal(err)
	}
	accepted := 0
	for _, a := range adrs {
		if a.Status == "accepted" {
			accepted++
		}
	}
	// with zero drift (structural invariant), every accepted ADR is binding
	if got != accepted {
		t.Errorf("binding count %d != accepted count %d (implies status drift)", got, accepted)
	}
}
