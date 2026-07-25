package adr

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "000001.md", "---\ntitle: First decision\ndate: 2026-01-01\nstatus: superseded\n---\nbody\n")
	writeFixture(t, dir, "000002.md", "---\ntitle: Second decision\ndate: 2026-01-02\nstatus: accepted\nsupersedes: [\"000001\"]\n---\nbody\n")
	writeFixture(t, dir, "000003.md", "---\ntitle: Third decision\ndate: 2026-01-03\nstatus: accepted\nsupersedes:\n  - \"ADR-2\"\n---\nbody\n")
	// non-6-digit stems must be excluded, exactly like adr_graph.py's ^\d{6}$ filter
	writeFixture(t, dir, "template.md", "---\ntitle: template\nstatus: proposed\n---\n")
	writeFixture(t, dir, "notes.txt", "not an adr")

	adrs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	want := map[string]ADR{
		"000001": {ID: "000001", Title: "First decision", Status: "superseded", Supersedes: []string{}},
		"000002": {ID: "000002", Title: "Second decision", Status: "accepted", Supersedes: []string{"000001"}},
		// supersedes ids are normalized to 6 digits like adr_graph.py's normalize_adr_id
		"000003": {ID: "000003", Title: "Third decision", Status: "accepted", Supersedes: []string{"000002"}},
	}
	if !reflect.DeepEqual(adrs, want) {
		t.Errorf("LoadDir = %#v, want %#v", adrs, want)
	}
}

func TestLoadDirEmptyStub(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "000001.md", "---\ntitle: t\nstatus: accepted\nsupersedes:\n  -\n---\nbody\n")
	adrs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	got, ok := adrs["000001"]
	if !ok {
		t.Fatal("000001 not loaded")
	}
	if !got.EmptySupersedesStub {
		t.Error("EmptySupersedesStub = false, want true")
	}
}

func TestLoadDirScalarSupersedes(t *testing.T) {
	// adr_graph.py wraps a scalar supersedes value into a one-element list
	dir := t.TempDir()
	writeFixture(t, dir, "000002.md", "---\ntitle: t\nstatus: accepted\nsupersedes: \"000001\"\n---\nbody\n")
	adrs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if want := []string{"000001"}; !reflect.DeepEqual(adrs["000002"].Supersedes, want) {
		t.Errorf("Supersedes = %v, want %v", adrs["000002"].Supersedes, want)
	}
}

func TestLoadDirCRLF(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "000001.md", "---\r\ntitle: CRLF decision\r\nstatus: accepted\r\nsupersedes: [\"000002\"]\r\n---\r\nbody\r\n")
	adrs, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	want := ADR{ID: "000001", Title: "CRLF decision", Status: "accepted", Supersedes: []string{"000002"}}
	if !reflect.DeepEqual(adrs["000001"], want) {
		t.Errorf("LoadDir CRLF = %#v, want %#v", adrs["000001"], want)
	}
}

func TestLoadDirEmpty(t *testing.T) {
	adrs, err := LoadDir(t.TempDir())
	if err != nil {
		t.Fatalf("LoadDir on empty dir: %v", err)
	}
	if len(adrs) != 0 {
		t.Errorf("LoadDir on empty dir = %v, want empty map", adrs)
	}
}

func TestLoadDirMissing(t *testing.T) {
	if _, err := LoadDir(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("LoadDir on missing dir: want error, got nil")
	}
}

func TestSupersedesGraph(t *testing.T) {
	adrs := map[string]ADR{
		"000001": {ID: "000001", Status: "superseded", Supersedes: []string{}},
		"000002": {ID: "000002", Status: "accepted", Supersedes: []string{"000001"}},
	}
	want := map[string][]string{"000001": {}, "000002": {"000001"}}
	if got := SupersedesGraph(adrs); !reflect.DeepEqual(got, want) {
		t.Errorf("SupersedesGraph = %v, want %v", got, want)
	}
}

func TestSortedIDs(t *testing.T) {
	adrs := map[string]ADR{"000003": {}, "000001": {}, "000002": {}}
	want := []string{"000001", "000002", "000003"}
	if got := SortedIDs(adrs); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedIDs = %v, want %v", got, want)
	}
}
