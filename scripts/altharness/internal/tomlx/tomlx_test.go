package tomlx

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAndUnmarshalBasicFeatures(t *testing.T) {
	src := `
# comment
version = 1
enabled = true
ratio = 3.14
hex = 0x1a
oct = 0o755
bin = 0b1010
big = 1_000_000
msg = "hello\nworld\t\"quoted\"\\end\u0041"
lit = 'c:\raw\path'
multi_basic = """
line1
line2 \
  joined"""
multi_lit = '''
raw1
raw2'''
arr = [
  "a",
  "b", # trailing comma
]
inline = { k1 = "v1", k2 = 42 }

[targets."alt-backend/app"]
kind = "go"

[targets."alt-backend/app".scanners]
golangci-lint = { status = "canary", canary = "go-canary" }
gosec = { status = "exempt", reason = "covered by golangci-lint" }

[[fingerprints]]
rule = "F401"
file = "a.py"
line_sha256 = "abc"
count = 2

[[fingerprints]]
rule = "S608"
file = "b.py"
line_sha256 = "def"
count = 1
`
	var raw map[string]any
	if err := Unmarshal([]byte(src), &raw); err != nil {
		t.Fatalf("Unmarshal map failed: %v", err)
	}
	if raw["version"] != int64(1) || raw["enabled"] != true || raw["hex"] != int64(0x1a) || raw["oct"] != int64(0o755) || raw["bin"] != int64(0b1010) || raw["big"] != int64(1000000) {
		t.Fatalf("unexpected scalars: %#v", raw)
	}
	if raw["msg"] != "hello\nworld\t\"quoted\"\\endA" {
		t.Fatalf("unexpected basic string: %q", raw["msg"])
	}
	if raw["lit"] != `c:\raw\path` {
		t.Fatalf("unexpected literal string: %q", raw["lit"])
	}
	if raw["multi_basic"] != "line1\nline2 joined" {
		t.Fatalf("unexpected multi_basic: %q", raw["multi_basic"])
	}
	if raw["multi_lit"] != "raw1\nraw2" {
		t.Fatalf("unexpected multi_lit: %q", raw["multi_lit"])
	}
	fps, ok := raw["fingerprints"].([]any)
	if !ok || len(fps) != 2 {
		t.Fatalf("expected 2 fingerprints, got %#v", raw["fingerprints"])
	}

	type scannerEntry struct {
		Status string `toml:"status"`
		Canary string `toml:"canary"`
		Reason string `toml:"reason"`
	}
	type targetEntry struct {
		Kind     string                  `toml:"kind"`
		Scanners map[string]scannerEntry `toml:"scanners"`
	}
	type doc struct {
		Version int                    `toml:"version"`
		Targets map[string]targetEntry `toml:"targets"`
	}
	var typed doc
	if err := Unmarshal([]byte(src), &typed); err != nil {
		t.Fatalf("Unmarshal struct failed: %v", err)
	}
	if typed.Version != 1 {
		t.Fatalf("version=%d, want 1", typed.Version)
	}
	tgt, ok := typed.Targets["alt-backend/app"]
	if !ok || tgt.Kind != "go" {
		t.Fatalf("unexpected target: %#v", tgt)
	}
	if tgt.Scanners["golangci-lint"].Canary != "go-canary" || tgt.Scanners["gosec"].Reason != "covered by golangci-lint" {
		t.Fatalf("unexpected scanners: %#v", tgt.Scanners)
	}
}

func TestUnmarshalRejectsInvalidTOML(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"missing equals", "foo bar\n"},
		{"unclosed string", "a = \"hello\n"},
		{"unclosed array", "a = [1, 2\n"},
		{"duplicate key", "a = 1\na = 2\n"},
		{"duplicate table", "[a]\nx = 1\n[a]\ny = 2\n"},
		{"trailing garbage", "a = 1 extra\n"},
		{"bad utf8", "a = \"\xff\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc map[string]any
			if err := Unmarshal([]byte(tc.src), &doc); err == nil {
				t.Fatalf("expected error for %s, got nil (%#v)", tc.name, doc)
			}
		})
	}
}

func TestUnmarshalRejectsTypeMismatch(t *testing.T) {
	type entry struct {
		Status string `toml:"status"`
	}
	type doc struct {
		Scanners map[string]entry `toml:"scanners"`
	}
	var d doc
	if err := Unmarshal([]byte("[scanners]\nruff = \"not-a-table\"\n"), &d); err == nil {
		t.Fatal("expected error decoding string into struct")
	}
}

func TestParsesRealRepoTOMLFiles(t *testing.T) {
	// Walk up from internal/tomlx to repo root.
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	relPaths := []string{
		"scripts/lint/canary_ledger.toml",
		"scripts/lint/empty_file_allowlist.toml",
		"scripts/lint/ratchet/suppressions.toml",
		"scripts/lint/ratchet/rule_baselines.toml",
		"scripts/lint/suppression_allowlist.toml",
		"scripts/pyproject.toml",
	}
	for _, rel := range relPaths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		var doc map[string]any
		if err := Unmarshal(data, &doc); err != nil {
			t.Fatalf("Unmarshal(%s) failed: %v", rel, err)
		}
		if len(doc) == 0 {
			t.Fatalf("Unmarshal(%s) returned empty map", rel)
		}
	}
}
