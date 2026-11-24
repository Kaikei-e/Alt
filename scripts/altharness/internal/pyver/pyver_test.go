package pyver

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRuffAndRequiresPython(t *testing.T) {
	tmp := t.TempDir()
	clean := filepath.Join(tmp, "clean.pyproject.toml")
	writeFile(t, clean, "[project]\nname = \"demo\"\nrequires-python = \">=3.14\"\n\n[tool.ruff]\nline-length = 100\n")
	if v := checkPyproject(clean, tmp); len(v) != 0 {
		t.Fatalf("clean: %#v", v)
	}
	for _, section := range []string{"[tool.ruff]", "[tool.ruff.lint]", "[tool.ruff.format]"} {
		path := filepath.Join(tmp, strings.ReplaceAll(strings.Trim(section, "[]"), ".", "_")+".toml")
		writeFile(t, path, "[project]\nname = \"demo\"\nrequires-python = \">=3.14\"\n\n"+section+"\ntarget-version = \"py314\"\n")
		v := checkPyproject(path, tmp)
		if !hasRule(v, "config.python.ruff-target-version-forbidden") || len(v) != 1 {
			t.Fatalf("%s: %#v", section, v)
		}
		if !strings.Contains(v[0].Message, section+".target-version is forbidden") {
			t.Fatalf("message: %s", v[0].Message)
		}
	}
	missing := filepath.Join(tmp, "missing.toml")
	writeFile(t, missing, "[project]\nname = \"demo\"\n")
	if !hasRule(checkPyproject(missing, tmp), "config.python.requires-python-missing") {
		t.Fatal("missing requires-python")
	}
	bad := filepath.Join(tmp, "bad.toml")
	writeFile(t, bad, "[project]\nname = \"demo\"\nrequires-python = \"not-a-version\"\n")
	v := checkPyproject(bad, tmp)
	if !hasRule(v, "config.python.requires-python-invalid") || !strings.Contains(v[0].String(), "requires-python='not-a-version'") {
		t.Fatalf("bad: %#v", v)
	}
	scriptsDir := filepath.Join(tmp, "scripts")
	if err := os.MkdirAll(scriptsDir, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	scriptsToml := filepath.Join(scriptsDir, "pyproject.toml")
	writeFile(t, scriptsToml, "[project]\nname = \"alt-scripts\"\nrequires-python = \">=3.14\"\n")
	if !hasRule(checkPyproject(scriptsToml, tmp), "config.python.scripts-requires-python-mismatch") {
		t.Fatal("scripts floor")
	}
	writeFile(t, scriptsToml, "[project]\nname = \"alt-scripts\"\nrequires-python = \">=3.13\"\n")
	if v := checkPyproject(scriptsToml, tmp); len(v) != 0 {
		t.Fatalf("scripts 3.13: %#v", v)
	}
}

func TestPEP758AndRuntime(t *testing.T) {
	lines := findUnparenthesizedExceptLines("try:\n    pass\nexcept ValueError, KeyError:\n    pass\n")
	if len(lines) != 1 || lines[0] != 3 {
		t.Fatalf("lines: %#v", lines)
	}
	star := findUnparenthesizedExceptLines("try:\n    pass\nexcept* ValueError, KeyError as exc:\n    pass\n")
	if len(star) != 1 || star[0] != 3 {
		t.Fatalf("star: %#v", star)
	}
	okSrc := "MSG = \"except ValueError, KeyError:\"\n# except ValueError, KeyError:\ndef f():\n    try:\n        return 1\n    except (ValueError, KeyError) as exc:\n        return exc\n"
	if lines := findUnparenthesizedExceptLines(okSrc); len(lines) != 0 {
		t.Fatalf("parenthesized/string/comment: %#v", lines)
	}
	rt := pyRuntime{major: 3, minor: 14, bin: ""}
	unit := checkSourceText("def f():\n    try:\n        return 1\n    except ValueError, KeyError:\n        return 0\n", "pep.py", [2]int{3, 13}, rt)
	finished := finishSource(unit, &astResponse{OK: true}, rt)
	if !hasRule(finished, "source.python.syntax-incompatible") {
		t.Fatalf("pep758: %#v", finished)
	}
	okUnit := checkSourceText(okSrc, "ok.py", [2]int{3, 13}, rt)
	if v := finishSource(okUnit, &astResponse{OK: true}, rt); len(v) != 0 {
		t.Fatalf("ok under 3.13 on 3.14 runtime: %#v", v)
	}
	old := pyRuntime{major: 3, minor: 12}
	future := checkSourceText(okSrc, "ok.py", [2]int{3, 99}, old)
	v := finishSource(future, nil, old)
	if !hasRule(v, "source.python.runtime-too-old") {
		t.Fatalf("runtime: %#v", v)
	}
}

func TestDockerfileFloor(t *testing.T) {
	tmp := t.TempDir()
	svc := filepath.Join(tmp, "svc", "app")
	if err := os.MkdirAll(svc, fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(svc, "pyproject.toml"), "[project]\nname = \"svc\"\nrequires-python = \">=3.14,<3.15\"\n")
	writeFile(t, filepath.Join(tmp, "svc", "Dockerfile"), "FROM python:3.14-slim@sha256:deadbeef AS builder\nFROM python:3.14-slim@sha256:deadbeef AS runtime\n")
	writeFile(t, filepath.Join(tmp, "svc", "Dockerfile.backend"), "FROM ollama/ollama:0.40.12\n")
	res, err := checkProject(svc, tmp, pyRuntime{major: 3, minor: 14})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.violations) != 0 || len(res.sources) != 0 {
		t.Fatalf("match: %#v", res.violations)
	}
	writeFile(t, filepath.Join(tmp, "svc", "Dockerfile"), "FROM python:3.14-slim AS builder\nFROM python:3.13-slim AS runtime\n")
	res, err = checkProject(svc, tmp, pyRuntime{major: 3, minor: 14})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(res.violations, "config.python.dockerfile-version-mismatch") {
		t.Fatalf("mismatch: %#v", res.violations)
	}
}

func TestCLICorpusAndFailClosed(t *testing.T) {
	root := repoRoot(t)
	ctx := context.Background()
	beforePP := filepath.Join(root, "scripts/harness/corpus/C-05/before.pyproject.toml")
	beforePy := filepath.Join(root, "scripts/harness/corpus/C-05/before.py")
	afterPP := filepath.Join(root, "scripts/harness/corpus/C-05/after.pyproject.toml")
	afterPy := filepath.Join(root, "scripts/harness/corpus/C-05/after.py")

	code, out, errb := runCLI(ctx, "--repo-root", root, "--pyproject", beforePP, "--source", beforePy)
	if code != 0 {
		t.Fatalf("before rc=%d out=%s err=%s", code, out, errb)
	}
	if !strings.Contains(out, "OK: Python version configuration check passed.") {
		t.Fatalf("stdout: %s", out)
	}

	code, out, errb = runCLI(ctx, "--repo-root", root, "--pyproject", afterPP, "--source", afterPy)
	combined := out + "\n" + errb
	if code != 1 || !strings.Contains(combined, "config.python.ruff-target-version-forbidden") || !strings.Contains(combined, "source.python.syntax-incompatible") {
		t.Fatalf("after rc=%d %s", code, combined)
	}

	code, out, errb = runCLI(ctx, "--repo-root", root, "--pyproject", beforePP, "--source", afterPy)
	if code != 1 || !strings.Contains(out+"\n"+errb, "source.python.syntax-incompatible") {
		t.Fatalf("after source rc=%d %s %s", code, out, errb)
	}

	tmp := t.TempDir()
	future := filepath.Join(tmp, "future.pyproject.toml")
	src := filepath.Join(tmp, "ok.py")
	writeFile(t, future, "[project]\nname = \"future\"\nrequires-python = \">=3.99\"\n")
	writeFile(t, src, "x = 1\n")
	code, _, errb = runCLI(ctx, "--pyproject", future, "--source", src)
	if code != 1 || !strings.Contains(errb, "python3.99 was not found") || !strings.Contains(errb, "refusing to skip AST syntax verification") {
		t.Fatalf("fail closed rc=%d err=%s", code, errb)
	}

	code, out, errb = runCLI(ctx, "--repo-root", root)
	if code != 0 {
		t.Fatalf("repo rc=%d out=%s err=%s", code, out, errb)
	}
}

func TestCLIUsage(t *testing.T) {
	code, _, errb := runCLI(context.Background(), "--pyproject", "a", "--project-dir", "b")
	if code != 2 || !strings.Contains(errb, "--pyproject and --project-dir are mutually exclusive") {
		t.Fatalf("exclusive rc=%d %s", code, errb)
	}
	code, _, errb = runCLI(context.Background(), "--source", "a.py")
	if code != 2 || !strings.Contains(errb, "--source requires --pyproject") {
		t.Fatalf("source rc=%d %s", code, errb)
	}
	code, _, errb = runCLI(context.Background(), "--bogus")
	if code != 2 || !strings.Contains(errb, "unrecognized arguments") {
		t.Fatalf("bogus rc=%d %s", code, errb)
	}
}

func checkPyproject(path, root string) []Violation {
	data, viols := parsePyproject(path, root)
	if data == nil {
		return viols
	}
	rel := relDisplay(path, root)
	checkRuffTargetVersion(data, rel, &viols)
	extractAndValidateFloor(data, rel, &viols)
	return viols
}

func hasRule(vs []Violation, rule string) bool {
	for _, v := range vs {
		if v.Rule == rule {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func runCLI(ctx context.Context, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
