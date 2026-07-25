package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestResolveWalksChain(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "resolve", "000001")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "000003\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestResolveNormalizesID(t *testing.T) {
	// adr_graph.py's normalize_adr_id accepts "1", "ADR-1", "000001"
	for _, id := range []string{"1", "ADR-1", "000001"} {
		stdout, _, code := run(t, "--adr-dir", fixture("ok"), "resolve", id)
		if code != exitOK {
			t.Fatalf("resolve %q: exit = %d, want %d", id, code, exitOK)
		}
		if want := "000003\n"; stdout != want {
			t.Errorf("resolve %q: stdout = %q, want %q", id, stdout, want)
		}
	}
}

func TestResolveFanIn(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("fan-in"), "resolve", "000001")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "000002\n000003\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestResolveTerminalIsSelf(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "resolve", "000003")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "000003\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestResolveUnknown(t *testing.T) {
	_, stderr, code := run(t, "--adr-dir", fixture("ok"), "resolve", "000099")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if want := "ERROR: unknown ADR 000099\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
}

func TestResolveJSON(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("fan-in"), "resolve", "000001", "--format", "json")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	var doc struct {
		ID        string   `json:"id"`
		Effective []string `json:"effective"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if doc.ID != "000001" {
		t.Errorf("id = %q, want 000001", doc.ID)
	}
	if want := []string{"000002", "000003"}; !reflect.DeepEqual(doc.Effective, want) {
		t.Errorf("effective = %v, want %v", doc.Effective, want)
	}
}

func TestResolveCyclicChainIsDomainFailure(t *testing.T) {
	// python crashes with RecursionError (exit 1) on a cyclic chain; adrdag
	// must not silently exit 0 with empty output — that would hand scripts a
	// false success on a corrupt corpus. It reports the cycle and exits 1.
	_, stderr, code := run(t, "--adr-dir", fixture("cycle"), "resolve", "000001")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if want := "ERROR: supersedes chain from 000001 never reaches a terminal ADR (cycle)\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
}

func TestResolveNoArgsIsUsageError(t *testing.T) {
	_, _, code := run(t, "--adr-dir", fixture("ok"), "resolve")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}

func TestResolveTooManyArgsIsUsageError(t *testing.T) {
	_, _, code := run(t, "--adr-dir", fixture("ok"), "resolve", "000001", "000002")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}
