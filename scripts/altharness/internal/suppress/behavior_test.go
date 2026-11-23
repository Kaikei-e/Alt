package suppress

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const minimalAllowlist = `version = 1

[ruff]
allowed_global_ignores = ["E501", "RUF001", "RUF002", "RUF003"]
allowed_test_per_file_ignores = ["S101"]

[[bandit]]
path = "tag-generator/app/pyproject.toml"
status = "frozen-baseline"
reason = "Pre-existing tag-generator bandit baseline"
expires = "2027-05-22"
exclude_dirs = ["tests", ".venv"]
skips = ["B101", "B104"]

[[golangci_exclusions]]
config = ".golangci.yml"
path = '_test\.go$'
linters = ["gosec"]
text = "G101"
status = "frozen-baseline"
reason = "Dummy tokens in test files"
expires = "2027-05-22"

[[golangci_exclusions]]
config = ".golangci.yml"
path_except = '^knowledge-sovereign/app/usecase/(knowledge_home_projector|knowledge_trail_projector)/[^/]+\.go$'
linters = ["forbidigo"]
status = "frozen-baseline"
reason = "Scope forbidigo to projectors"
expires = "2027-05-22"

[[cargo_lint_allows]]
path = "recap-worker/recap-worker/Cargo.toml"
section = "lints.clippy"
status = "frozen-baseline"
reason = "Pre-existing pedantic allows"
expires = "2027-05-22"
allows = ["doc_markdown", "module_name_repetitions"]
`

func TestExpiryHorizon(t *testing.T) {
	repo := newGitRepo(t)
	al := filepath.Join(repo, "allow.toml")
	cases := []struct {
		name   string
		toml   string
		rust   string
		py     string
		want   int
		substr string
	}{
		{
			name: "exact 366-day horizon passes",
			toml: minimalAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Boundary 366 days from today\"\nexpires = \"2027-11-23\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n",
			want: 0,
		},
		{
			name:   "367 days is expires-too-far",
			toml:   minimalAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"One day past the horizon\"\nexpires = \"2027-11-24\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n",
			want:   1,
			substr: "config.allowlist.expires-too-far",
		},
		{
			name:   "far future fails even with an existing ADR",
			toml:   strings.Replace(minimalAllowlist, "status = \"frozen-baseline\"\nreason = \"Pre-existing tag-generator bandit baseline\"\nexpires = \"2027-05-22\"", "status = \"frozen-baseline\"\nadr = \"ADR-000979\"\nreason = \"Far future bypass attempt\"\nexpires = \"2099-12-31\"", 1),
			want:   1,
			substr: "config.allowlist.expires-too-far",
		},
		{
			name:   "rust broad suppression past the horizon",
			toml:   minimalAllowlist,
			rust:   "// ADR-000979: wit-bindgen generated code requires allow\n// expires: 2099-12-31\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n",
			want:   1,
			substr: "source.broad.expires-too-far",
		},
		{
			name:   "python file noqa past the horizon",
			toml:   minimalAllowlist,
			py:     "# ADR-000979: vendored bridge requires file-level ignore (expires: 2028-01-01)\n# ruff: noqa: F401\n",
			want:   1,
			substr: "source.broad.expires-too-far",
		},
		{
			name:   "expired allowlist entry",
			toml:   minimalAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Approved exception already expired\"\nexpires = \"2026-11-21\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n",
			want:   1,
			substr: "config.allowlist.expired",
		},
		{
			name: "expires equal to today passes",
			toml: minimalAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Approved exception expiring today\"\nexpires = \"2026-11-22\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n",
			want: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, al, tc.toml)
			rustPath := filepath.Join(repo, "plecto", "src", "lib.rs")
			pyPath := filepath.Join(repo, "app", "a.py")
			if tc.rust != "" {
				writeFile(t, rustPath, tc.rust)
			} else {
				_ = os.Remove(rustPath)
			}
			if tc.py != "" {
				writeFile(t, pyPath, tc.py)
			} else {
				_ = os.Remove(pyPath)
			}
			code, _, errText := runCheck(t, repo, al)
			if code != tc.want {
				t.Fatalf("exit %d, want %d\nstderr:\n%s", code, tc.want, errText)
			}
			if tc.substr != "" && !strings.Contains(errText, tc.substr) {
				t.Fatalf("stderr missing %s:\n%s", tc.substr, errText)
			}
		})
	}
}

func TestADRGateAndBadBase(t *testing.T) {
	repo := newGitRepo(t)
	al := filepath.Join(repo, "scripts", "lint", "suppression_allowlist.toml")
	writeFile(t, al, minimalAllowlist)
	rs := filepath.Join(repo, "plecto", "src", "lib.rs")
	writeFile(t, rs, "// ADR-000979: wit-bindgen generated code requires allow\n// expires: 2027-05-22\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "base")
	base := runGit(t, repo, "rev-parse", "HEAD")

	extended := strings.Replace(minimalAllowlist,
		"status = \"frozen-baseline\"\nreason = \"Pre-existing tag-generator bandit baseline\"\nexpires = \"2027-05-22\"",
		"status = \"frozen-baseline\"\nadr = \"ADR-000979\"\nreason = \"Extended without ADR update\"\nexpires = \"2027-09-01\"", 1)
	writeFile(t, al, extended)
	code, _, errText := runCheckBase(t, repo, al, base)
	if code != 1 || !strings.Contains(errText, "config.allowlist.expires-extended-without-adr-update") {
		t.Fatalf("allowlist extension without ADR update: exit %d\n%s", code, errText)
	}

	writeFile(t, filepath.Join(repo, "docs", "ADR", "000979.md"), "# ADR-000979\nExtended bandit baseline to 2027-09-01.\n")
	code, out, errText := runCheckBase(t, repo, al, base)
	if code != 0 {
		t.Fatalf("allowlist extension with ADR update: exit %d\n%s%s", code, out, errText)
	}

	runGit(t, repo, "checkout", "--", ".")
	writeFile(t, rs, "// ADR-000979: wit-bindgen generated code requires allow\n// expires: 2027-09-01\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	code, _, errText = runCheckBase(t, repo, al, base)
	if code != 1 || !strings.Contains(errText, "source.broad.expires-extended-without-adr-update") {
		t.Fatalf("source extension without ADR update: exit %d\n%s", code, errText)
	}

	writeFile(t, filepath.Join(repo, "docs", "ADR", "001050.md"), "# ADR-001050\nRenew plecto wit-bindgen allow.\n")
	writeFile(t, rs, "// ADR-001050: wit-bindgen generated code requires allow\n// expires: 2027-09-01\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	code, out, errText = runCheckBase(t, repo, al, base)
	if code != 0 {
		t.Fatalf("source extension with new ADR: exit %d\n%s%s", code, out, errText)
	}

	clobber := filepath.Join(t.TempDir(), "clobbered.txt")
	code, _, errText = runCheck(t, repo, al, "--base=--output="+clobber)
	if code == 0 || fileExists(clobber) {
		t.Fatalf("bad --base must fail closed, exit %d file=%v\n%s", code, fileExists(clobber), errText)
	}
	if !strings.Contains(errText, "invalid --base git revision") {
		t.Fatalf("bad --base diagnostic:\n%s", errText)
	}
}

func TestDiffReport(t *testing.T) {
	repo := newGitRepo(t)
	al := filepath.Join(repo, "scripts", "lint", "suppression_allowlist.toml")
	writeFile(t, al, allowlistV1)
	pyFile := filepath.Join(repo, "app.py")
	writeFile(t, pyFile, "x = 1  # noqa: F401 -- removed later\ny = 2  # noqa: S603 -- shifted line stays\n")
	writeFile(t, filepath.Join(repo, "main.go"), "package main\nfunc f() { _ = a() // #nosec G304 -- stays unchanged\n}\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "base")
	base := runGit(t, repo, "rev-parse", "HEAD")

	writeFile(t, al, allowlistV2)
	writeFile(t, pyFile, "# comment\n# comment 2\ny = 2  # noqa: S603 -- shifted line stays\nq = f\"DELETE FROM {tbl}\"  # noqa: S608 -- identifier (trusted constant)\n")
	writeFile(t, filepath.Join(repo, "lib.rs"), "// ADR-000902: generated bindings require allow (expires: 2027-05-22)\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	summary := filepath.Join(t.TempDir(), "step_summary.md")
	writeFile(t, summary, "# Existing Header\n")

	var out, errBuf bytes.Buffer
	code := RunDiff(context.Background(), []string{"--repo-root", repo, "--base", base, "--summary-file", summary}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("diff exit %d\n%s%s", code, out.String(), errBuf.String())
	}
	text := out.String()
	for _, want := range []string{"| **Total**", "+2", "`app.py:4`", "S608", "`lib.rs:2`", "2027-05-22", "crate/Cargo.toml", "2027-06-01", "removed later", "F401"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "shifted line stays") {
		t.Fatalf("shifted line was reported\n%s", text)
	}
	summaryText := readText(t, summary)
	if !strings.HasPrefix(summaryText, "# Existing Header\n") || !strings.Contains(summaryText, "Suppression Governance Diff Report") {
		t.Fatalf("summary file:\n%s", summaryText)
	}

	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "head")
	head := runGit(t, repo, "rev-parse", "HEAD")
	out.Reset()
	errBuf.Reset()
	code = RunDiff(context.Background(), []string{"--repo-root", repo, "--base", base, "--head", head}, &out, &errBuf)
	if code != 0 || !strings.Contains(out.String(), "`app.py:4`") || !strings.Contains(out.String(), "`lib.rs:2`") {
		t.Fatalf("head mode exit %d\n%s%s", code, out.String(), errBuf.String())
	}

	clobber := filepath.Join(t.TempDir(), "clobbered.txt")
	out.Reset()
	errBuf.Reset()
	code = RunDiff(context.Background(), []string{"--repo-root", repo, "--base=--output=" + clobber}, &out, &errBuf)
	if code == 0 || fileExists(clobber) {
		t.Fatalf("diff bad --base exit %d exists=%v\n%s", code, fileExists(clobber), errBuf.String())
	}
	out.Reset()
	errBuf.Reset()
	code = RunDiff(context.Background(), []string{"--repo-root", repo, "--base", base, "--head=--output=" + clobber}, &out, &errBuf)
	if code == 0 || fileExists(clobber) {
		t.Fatalf("diff bad --head exit %d exists=%v\n%s", code, fileExists(clobber), errBuf.String())
	}

	outside := filepath.Join(t.TempDir(), "outside_secret.py")
	writeFile(t, outside, "x = 1  # noqa: S603 -- leaked outside secret\n")
	if err := os.Symlink(outside, filepath.Join(repo, "escape_link.py")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	code = RunDiff(context.Background(), []string{"--repo-root", repo, "--base", head}, &out, &errBuf)
	if code != 0 || strings.Contains(out.String(), "leaked outside secret") {
		t.Fatalf("symlink escape exit %d\n%s%s", code, out.String(), errBuf.String())
	}
	_ = os.Remove(filepath.Join(repo, "escape_link.py"))

	writeFile(t, al, "version = 1\n[[bandit\nbroken toml\n")
	out.Reset()
	errBuf.Reset()
	code = RunDiff(context.Background(), []string{"--repo-root", repo, "--base", head}, &out, &errBuf)
	if code == 0 {
		t.Fatalf("corrupt allowlist should fail closed\n%s", out.String())
	}
}

const allowlistV1 = `version = 1

[ruff]
allowed_global_ignores = ["E501"]
allowed_test_per_file_ignores = ["S101"]

[[bandit]]
path = "svc/pyproject.toml"
status = "frozen-baseline"
reason = "Old bandit skip"
expires = "2027-05-22"
exclude_dirs = ["tests"]
skips = ["B101"]
`

const allowlistV2 = `version = 1

[ruff]
allowed_global_ignores = ["E501"]
allowed_test_per_file_ignores = ["S101"]

[[bandit]]
path = "svc/pyproject.toml"
status = "frozen-baseline"
reason = "Old bandit skip"
expires = "2027-05-22"
exclude_dirs = ["tests"]
skips = ["B101"]

[[cargo_lint_allows]]
path = "crate/Cargo.toml"
section = "lints.clippy"
adr = "ADR-000979"
reason = "Approved pedantic allow"
expires = "2027-06-01"
allows = ["doc_markdown"]
`

func newGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(repo, "docs", "ADR", "000979.md"), "# ADR-000979\n")
	return repo
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test Author",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test Committer",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func runCheck(t *testing.T, repo, allow string, extra ...string) (int, string, string) {
	t.Helper()
	args := []string{"--repo-root", repo, "--allowlist", allow, "--today", "2026-11-22", "--no-python-noqa-baseline"}
	args = append(args, extra...)
	var out, errBuf bytes.Buffer
	code := RunCheck(context.Background(), args, &out, &errBuf)
	return code, out.String(), errBuf.String()
}

func runCheckBase(t *testing.T, repo, allow, base string) (int, string, string) {
	t.Helper()
	return runCheck(t, repo, allow, "--base", base)
}
