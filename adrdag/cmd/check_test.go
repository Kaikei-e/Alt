package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCheckOK(t *testing.T) {
	stdout, stderr, code := run(t, "--adr-dir", fixture("ok"), "check")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	// exact parity with adr_graph.py's OK line
	if want := "OK: 3 ADRs, 2 supersedes edges, no cycles, status aligned\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

func TestCheckDangling(t *testing.T) {
	_, stderr, code := run(t, "--adr-dir", fixture("dangling"), "check")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if want := "ERROR: 000002 supersedes unknown ADR 000009\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
}

func TestCheckCycle(t *testing.T) {
	_, stderr, code := run(t, "--adr-dir", fixture("cycle"), "check")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if want := "ERROR: cycle detected in supersedes graph: 000001 --> 000002 --> 000001\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
}

func TestCheckEmptyStub(t *testing.T) {
	_, stderr, code := run(t, "--adr-dir", fixture("empty-stub"), "check")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if want := "ERROR: 000001 has empty supersedes stub (omit the key or use a real id list)\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
}

func TestCheckStatusDrift(t *testing.T) {
	_, stderr, code := run(t, "--adr-dir", fixture("status-drift"), "check")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	if want := "ERROR: 000001 status=accepted but superseded by 000002 (set status: superseded)\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
}

func TestCheckOrphanSupersededWarnsButPasses(t *testing.T) {
	stdout, stderr, code := run(t, "--adr-dir", fixture("orphan-superseded"), "check")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "WARN: 000001 status=superseded with no inbound supersedes (withdrawn/deprecated? do not invent an edge)\n"; !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want contains %q", stderr, want)
	}
	if want := "OK: 2 ADRs, 0 supersedes edges, no cycles, status aligned\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestCheckJSON(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", fixture("status-drift"), "check", "--format", "json")
	if code != exitFailure {
		t.Fatalf("exit = %d, want %d", code, exitFailure)
	}
	var doc struct {
		ADRCount  int  `json:"adr_count"`
		EdgeCount int  `json:"edge_count"`
		OK        bool `json:"ok"`
		Findings  []struct {
			Severity string `json:"severity"`
			Rule     string `json:"rule"`
			ID       string `json:"id"`
			Detail   string `json:"detail"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout)
	}
	if doc.OK {
		t.Error("ok = true, want false")
	}
	if doc.ADRCount != 2 || doc.EdgeCount != 1 {
		t.Errorf("adr_count/edge_count = %d/%d, want 2/1", doc.ADRCount, doc.EdgeCount)
	}
	found := false
	for _, f := range doc.Findings {
		if f.Rule == "status_drift" && f.ID == "000001" && f.Severity == "error" {
			found = true
		}
	}
	if !found {
		t.Errorf("findings missing status_drift error for 000001: %+v", doc.Findings)
	}
}

func TestCheckMissingDirIsIOError(t *testing.T) {
	_, _, code := run(t, "--adr-dir", fixture("does-not-exist"), "check")
	if code != exitIO {
		t.Fatalf("exit = %d, want %d", code, exitIO)
	}
}

func TestCheckBadFormatIsUsageError(t *testing.T) {
	_, _, code := run(t, "--adr-dir", fixture("ok"), "check", "--format", "yaml")
	if code != exitUsage {
		t.Fatalf("exit = %d, want %d", code, exitUsage)
	}
}

func TestAdrDirEnvFallback(t *testing.T) {
	t.Setenv("ADRDAG_ADR_DIR", fixture("ok"))
	stdout, _, code := run(t, "check")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if want := "OK: 3 ADRs, 2 supersedes edges, no cycles, status aligned\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestAdrDirFlagBeatsEnv(t *testing.T) {
	// standard flag semantics: an explicit --adr-dir always wins over env
	t.Setenv("ADRDAG_ADR_DIR", fixture("status-drift"))
	stdout, _, code := run(t, "--adr-dir", fixture("ok"), "check")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (flag should override env)", code, exitOK)
	}
	if want := "OK: 3 ADRs, 2 supersedes edges, no cycles, status aligned\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestCheckEmptyDir(t *testing.T) {
	stdout, stderr, code := run(t, "--adr-dir", t.TempDir(), "check")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	if want := "OK: 0 ADRs, 0 supersedes edges, no cycles, status aligned\n"; stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

func TestBindingEmptyDir(t *testing.T) {
	stdout, _, code := run(t, "--adr-dir", t.TempDir(), "binding")
	if code != exitOK {
		t.Fatalf("exit = %d, want %d", code, exitOK)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}
