package suppress

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsersMatchPython(t *testing.T) {
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
		if stringsHasPrefixPath(rel, "stopped-using-k8s/") {
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
			b, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Ext(rel) == ".yml" || filepath.Ext(rel) == ".yaml" {
				got, err := parseYAML(string(b))
				if err != nil {
					t.Fatal(err)
				}
				want := pythonYAML(t, b)
				if js(got.pyValue()) != want {
					t.Fatalf("yaml mismatch\n got %s\nwant %s", js(got.pyValue()), want)
				}
				return
			}
			got, err := parseTOML(string(b))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			want := pythonTOML(t, b)
			if js(got) != want {
				t.Fatalf("toml mismatch\n got %s\nwant %s", truncate(js(got), 500), truncate(want, 500))
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
		if _, err := os.Stat(filepath.Join(dir, "scripts/lint/check_suppressions.py")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found from " + wd)
		}
		dir = parent
	}
}

func pythonTOML(t *testing.T, src []byte) string {
	t.Helper()
	return pythonNorm(t, src, `
import json, sys, tomllib
def norm(o):
    if isinstance(o, dict):
        return {k: norm(v) for k, v in o.items()}
    if isinstance(o, list):
        return [norm(v) for v in o]
    if isinstance(o, bool):
        return o
    if isinstance(o, int) and not isinstance(o, bool):
        return o
    if isinstance(o, float):
        return o
    if isinstance(o, str):
        return o
    return {"__t": type(o).__name__, "v": str(o)}
print(json.dumps(norm(tomllib.loads(sys.stdin.read())), sort_keys=True, ensure_ascii=False, separators=(",", ":")))
`)
}

func pythonYAML(t *testing.T, src []byte) string {
	t.Helper()
	return pythonNorm(t, src, `
import json, sys, yaml
def norm(o):
    if isinstance(o, dict):
        return {"__map": [[k, norm(v)] for k, v in o.items()]}
    if isinstance(o, list):
        return [norm(v) for v in o]
    if isinstance(o, bool):
        return o
    if isinstance(o, int) and not isinstance(o, bool):
        return o
    if isinstance(o, float):
        return o
    if o is None:
        return None
    if isinstance(o, str):
        return o
    return {"__t": type(o).__name__, "v": str(o)}
print(json.dumps(norm(yaml.safe_load(sys.stdin.read())), ensure_ascii=False, separators=(",", ":")))
`)
}

func pythonNorm(t *testing.T, src []byte, code string) string {
	t.Helper()
	cmd := exec.Command("python3", "-c", code)
	cmd.Stdin = bytes.NewReader(src)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("python: %v\n%s", err, out)
	}
	return string(bytes.TrimSpace(out))
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

func stringsHasPrefixPath(rel, prefix string) bool {
	return strings.HasPrefix(rel, prefix)
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
