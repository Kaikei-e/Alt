package suppress

import (
	"alt/scripts/altharness/internal/tomlx"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepoConfigParsers(t *testing.T) {
	t.Parallel()
	root := repoRootFromHere(t)
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == ".venv" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "stopped-using-k8s/") {
			return nil
		}
		if name == "pyproject.toml" || name == "Cargo.toml" || name == "suppression_allowlist.toml" || name == "python_noqa_baseline.toml" || name == ".golangci.yml" || name == ".golangci.yaml" {
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no config files found")
	}
	for _, rel := range files {
		rel := rel
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			b, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Ext(rel) == ".yml" || filepath.Ext(rel) == ".yaml" {
				got, err := parseYAML(string(b))
				if err != nil {
					t.Fatal(err)
				}
				if got.pyValue() == nil {
					t.Fatalf("yaml parsed to nil for %s", rel)
				}
				return
			}
			got, err := parseTOML(string(b))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var want map[string]any
			if err := tomlx.Unmarshal(b, &want); err != nil {
				t.Fatalf("tomlx: %v", err)
			}
			if js(got) != js(want) {
				t.Fatalf("toml mismatch\n got %s\nwant %s", truncate(js(got), 500), truncate(js(want), 500))
			}
		})
	}
}

func repoRootFromHere(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "altharness", "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found from " + wd)
		}
		dir = parent
	}
}

func js(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(normalizeJSON(v)); err != nil {
		return err.Error()
	}
	return strings.TrimSpace(buf.String())
}

func normalizeJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = normalizeJSON(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = normalizeJSON(val)
		}
		return out
	case orderedMap:
		items := make([]any, len(t.pairs))
		for i, p := range t.pairs {
			items[i] = []any{p.k, normalizeJSON(p.v)}
		}
		return map[string]any{"__map": items}
	case int:
		return t
	case int64:
		return t
	case tomlTime:
		return map[string]any{"__t": "datetime", "v": t.raw}
	default:
		return v
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
