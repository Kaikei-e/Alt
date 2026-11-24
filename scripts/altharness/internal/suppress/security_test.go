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

const securityAllowlist = `version = 1

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

[[cargo_lint_allows]]
path = "recap-worker/recap-worker/Cargo.toml"
section = "lints.clippy"
status = "frozen-baseline"
reason = "Pre-existing pedantic allows"
expires = "2027-05-22"
allows = ["doc_markdown", "module_name_repetitions"]
`

func TestExpiryHorizonAndADRReuse(t *testing.T) {
	repo := newSecurityRepo(t)
	cases := []struct {
		name   string
		toml   string
		rust   string
		py     string
		goBody string
		want   int
		substr string
	}{
		{name: "exact 366-day horizon passes", toml: securityAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Boundary 366 days from today\"\nexpires = \"2027-11-23\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n", want: 0},
		{name: "367 days is expires-too-far", toml: securityAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"One day past the horizon\"\nexpires = \"2027-11-24\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n", want: 1, substr: "config.allowlist.expires-too-far"},
		{name: "far future fails even with an existing ADR", toml: strings.Replace(securityAllowlist, "status = \"frozen-baseline\"\nreason = \"Pre-existing tag-generator bandit baseline\"\nexpires = \"2027-05-22\"", "status = \"frozen-baseline\"\nadr = \"ADR-000979\"\nreason = \"Far future bypass attempt\"\nexpires = \"2099-12-31\"", 1), want: 1, substr: "config.allowlist.expires-too-far"},
		{name: "frozen ceiling without ADR", toml: strings.Replace(securityAllowlist, "expires = \"2027-05-22\"", "expires = \"2099-12-31\"", 1), want: 1, substr: "config.allowlist.expires-extended-without-adr"},
		{name: "missing expires", toml: securityAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Approved exception missing expires\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n", want: 1, substr: "config.allowlist.missing-expires"},
		{name: "expired allowlist entry", toml: securityAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Approved exception already expired\"\nexpires = \"2026-11-21\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n", want: 1, substr: "config.allowlist.expired"},
		{name: "expires equal to today passes", toml: securityAllowlist + "\n[[bandit]]\npath = \"svc/pyproject.toml\"\nadr = \"ADR-000979\"\nreason = \"Approved exception expiring today\"\nexpires = \"2026-11-22\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n", want: 0},
		{name: "rust broad suppression past the horizon", toml: securityAllowlist, rust: "// ADR-000979: wit-bindgen generated code requires allow\n// expires: 2099-12-31\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n", want: 1, substr: "source.broad.expires-too-far"},
		{name: "python file noqa past the horizon", toml: securityAllowlist, py: "# ADR-000979: vendored bridge requires file-level ignore (expires: 2028-01-01)\n# ruff: noqa: F401\n", want: 1, substr: "source.broad.expires-too-far"},
		{name: "sql value interpolation", toml: securityAllowlist, py: "q = f\"WHERE id = {val}\"  # noqa: S608 -- value from trusted config\n", want: 1, substr: "source.sql.value-interpolation-forbidden"},
		{name: "sql untrusted boundary", toml: securityAllowlist, py: "q = f\"FROM {tbl}\"  # noqa: S608 -- identifier from untrusted request\n", want: 1, substr: "source.sql.untrusted-boundary-forbidden"},
		{name: "nolint gosec forbidden", toml: securityAllowlist, goBody: "package foo\nfunc f() { _ = a() //nolint:gosec // reason text here\n}\n", want: 1, substr: "source.go.nolint-gosec-forbidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newSecurityRepo(t)
			al := filepath.Join(root, "scripts", "lint", "suppression_allowlist.toml")
			writeSecurity(t, al, tc.toml)
			if tc.rust != "" {
				writeSecurity(t, filepath.Join(root, "plecto", "src", "lib.rs"), tc.rust)
			}
			if tc.py != "" {
				writeSecurity(t, filepath.Join(root, "app", "a.py"), tc.py)
			}
			if tc.goBody != "" {
				writeSecurity(t, filepath.Join(root, "svc", "a.go"), tc.goBody)
			}
			gitSecurity(t, root, "add", ".")
			code, _, errText := runSecurityCheck(t, root, al)
			if code != tc.want {
				t.Fatalf("exit %d, want %d\n%s", code, tc.want, errText)
			}
			if tc.substr != "" && !strings.Contains(errText, tc.substr) {
				t.Fatalf("stderr missing %s:\n%s", tc.substr, errText)
			}
		})
	}
	_ = repo
}

func TestSameDiffADRAndFailClosed(t *testing.T) {
	repo := newSecurityRepo(t)
	al := filepath.Join(repo, "scripts", "lint", "suppression_allowlist.toml")
	writeSecurity(t, al, securityAllowlist)
	rs := filepath.Join(repo, "plecto", "src", "lib.rs")
	writeSecurity(t, rs, "// ADR-000979: wit-bindgen generated code requires allow\n// expires: 2027-05-22\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	gitSecurity(t, repo, "add", ".")
	gitSecurity(t, repo, "commit", "-q", "-m", "base")
	base := gitSecurity(t, repo, "rev-parse", "HEAD")

	extended := strings.Replace(securityAllowlist,
		"status = \"frozen-baseline\"\nreason = \"Pre-existing tag-generator bandit baseline\"\nexpires = \"2027-05-22\"",
		"status = \"frozen-baseline\"\nadr = \"ADR-000979\"\nreason = \"Extended without ADR update\"\nexpires = \"2027-09-01\"", 1)
	writeSecurity(t, al, extended)
	code, _, errText := runSecurityCheckBase(t, repo, al, base)
	if code != 1 || !strings.Contains(errText, "config.allowlist.expires-extended-without-adr-update") {
		t.Fatalf("allowlist extension without ADR update: exit %d\n%s", code, errText)
	}
	writeSecurity(t, filepath.Join(repo, "docs", "ADR", "000979.md"), "# ADR-000979\nExtended bandit baseline to 2027-09-01.\n")
	code, out, errText := runSecurityCheckBase(t, repo, al, base)
	if code != 0 {
		t.Fatalf("allowlist extension with ADR update: exit %d\n%s%s", code, out, errText)
	}

	gitSecurity(t, repo, "checkout", "--", ".")
	writeSecurity(t, rs, "// ADR-000979: wit-bindgen generated code requires allow\n// expires: 2027-09-01\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	code, _, errText = runSecurityCheckBase(t, repo, al, base)
	if code != 1 || !strings.Contains(errText, "source.broad.expires-extended-without-adr-update") {
		t.Fatalf("source extension without ADR update: exit %d\n%s", code, errText)
	}
	writeSecurity(t, filepath.Join(repo, "docs", "ADR", "001050.md"), "# ADR-001050\nRenew plecto wit-bindgen allow.\n")
	writeSecurity(t, rs, "// ADR-001050: wit-bindgen generated code requires allow\n// expires: 2027-09-01\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	code, out, errText = runSecurityCheckBase(t, repo, al, base)
	if code != 0 {
		t.Fatalf("source extension with new ADR: exit %d\n%s%s", code, out, errText)
	}

	code, _, errText = runSecurityCheck(t, repo, al, "--base", "does-not-exist")
	if code != 2 || !strings.Contains(errText, "invalid --base git revision 'does-not-exist'") {
		t.Fatalf("unresolved base: exit %d\n%s", code, errText)
	}
	code, _, errText = runSecurityCheck(t, repo, al, "--base=--output=/tmp/not-created")
	if code != 2 || !strings.Contains(errText, "invalid --base git revision") {
		t.Fatalf("dash base: exit %d\n%s", code, errText)
	}
}

func TestDiffSummaryAndFailClosed(t *testing.T) {
	repo := newSecurityRepo(t)
	al := filepath.Join(repo, "scripts", "lint", "suppression_allowlist.toml")
	v1 := "version = 1\n\n[ruff]\nallowed_global_ignores = [\"E501\"]\nallowed_test_per_file_ignores = [\"S101\"]\n\n[[bandit]]\npath = \"svc/pyproject.toml\"\nstatus = \"frozen-baseline\"\nreason = \"Old bandit skip\"\nexpires = \"2027-05-22\"\nexclude_dirs = [\"tests\"]\nskips = [\"B101\"]\n"
	v2 := v1 + "\n[[cargo_lint_allows]]\npath = \"crate/Cargo.toml\"\nsection = \"lints.clippy\"\nadr = \"ADR-000979\"\nreason = \"Approved pedantic allow\"\nexpires = \"2027-06-01\"\nallows = [\"doc_markdown\"]\n"
	writeSecurity(t, al, v1)
	writeSecurity(t, filepath.Join(repo, "app.py"), "x = 1  # noqa: F401 -- removed later\ny = 2  # noqa: S603 -- shifted line stays\n")
	writeSecurity(t, filepath.Join(repo, "main.go"), "package main\nfunc f() { _ = a() // #nosec G304 -- stays unchanged\n}\n")
	gitSecurity(t, repo, "add", ".")
	gitSecurity(t, repo, "commit", "-q", "-m", "base")
	base := gitSecurity(t, repo, "rev-parse", "HEAD")
	writeSecurity(t, al, v2)
	writeSecurity(t, filepath.Join(repo, "app.py"), "# comment\n# comment 2\ny = 2  # noqa: S603 -- shifted line stays\nq = f\"DELETE FROM {t}\"  # noqa: S608 -- identifier (trusted constant)\n")
	writeSecurity(t, filepath.Join(repo, "lib.rs"), "// ADR-000902: generated bindings require allow (expires: 2027-05-22)\n#[allow(clippy::too_many_arguments)]\nmod bindings {}\n")
	summary := filepath.Join(t.TempDir(), "step_summary.md")
	writeSecurity(t, summary, "# Existing Header\n")
	var out, errBuf bytes.Buffer
	code := RunDiff(context.Background(), []string{"--repo-root", repo, "--base", base, "--summary-file", summary}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("diff exit %d\n%s%s", code, out.String(), errBuf.String())
	}
	text := out.String()
	for _, want := range []string{"| **Total**", "`app.py:4`", "S608", "`lib.rs:2`", "2027-05-22", "crate/Cargo.toml", "2027-06-01", "removed later", "F401"} {
		if !strings.Contains(text, want) {
			t.Fatalf("report missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "shifted line stays") {
		t.Fatalf("shifted line was reported\n%s", text)
	}
	summaryText, err := os.ReadFile(summary)
	if err != nil || !strings.HasPrefix(string(summaryText), "# Existing Header\n") || !strings.Contains(string(summaryText), "Suppression Governance Diff Report") {
		t.Fatalf("summary file: %v\n%s", err, summaryText)
	}
	out.Reset()
	errBuf.Reset()
	if rc := RunDiff(context.Background(), []string{"--repo-root", repo, "--base=--output=/tmp/clobber"}, &out, &errBuf); rc == 0 || !strings.Contains(errBuf.String(), "must not be empty or start with '-'") {
		t.Fatalf("dash base rc=%d\n%s", rc, errBuf.String())
	}
	outside := filepath.Join(t.TempDir(), "outside.py")
	writeSecurity(t, outside, "x = 1  # noqa: S603 -- leaked outside secret\n")
	if err := os.Symlink(outside, filepath.Join(repo, "escape_link.py")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	errBuf.Reset()
	if rc := RunDiff(context.Background(), []string{"--repo-root", repo, "--base", base}, &out, &errBuf); rc != 0 || strings.Contains(out.String(), "leaked outside secret") {
		t.Fatalf("symlink rc=%d\n%s%s", rc, out.String(), errBuf.String())
	}
	os.Remove(filepath.Join(repo, "escape_link.py"))
	writeSecurity(t, al, "version = 1\n[[bandit\nbroken toml\n")
	out.Reset()
	errBuf.Reset()
	if rc := RunDiff(context.Background(), []string{"--repo-root", repo, "--base", base}, &out, &errBuf); rc == 0 || !strings.Contains(errBuf.String(), "ERROR: failed to build suppression diff report:") {
		t.Fatalf("corrupt toml rc=%d\n%s%s", rc, out.String(), errBuf.String())
	}
}

func TestCanceledContextFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errb bytes.Buffer
	if code := RunCheck(ctx, []string{"--repo-root", t.TempDir()}, &out, &errb); code == 0 {
		t.Fatalf("canceled check returned 0\n%s%s", out.String(), errb.String())
	}
	out.Reset()
	errb.Reset()
	if code := RunDiff(ctx, []string{"--repo-root", t.TempDir(), "--base", "HEAD"}, &out, &errb); code == 0 {
		t.Fatalf("canceled diff returned 0\n%s%s", out.String(), errb.String())
	}
}

func newSecurityRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	gitSecurity(t, repo, "init", "-q")
	gitSecurity(t, repo, "config", "user.email", "test@example.com")
	gitSecurity(t, repo, "config", "user.name", "Test")
	writeSecurity(t, filepath.Join(repo, "docs", "ADR", "000979.md"), "# ADR-000979\n")
	gitSecurity(t, repo, "add", ".")
	gitSecurity(t, repo, "commit", "-q", "-m", "adr")
	return repo
}

func writeSecurity(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), fsperm.File); err != nil {
		t.Fatal(err)
	}
}

func gitSecurity(t *testing.T, repo string, args ...string) string {
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
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func runSecurityCheck(t *testing.T, repo, allow string, extra ...string) (int, string, string) {
	t.Helper()
	args := []string{"--repo-root", repo, "--allowlist", allow, "--today", "2026-11-22", "--no-python-noqa-baseline"}
	args = append(args, extra...)
	var out, errb bytes.Buffer
	code := RunCheck(context.Background(), args, &out, &errb)
	return code, out.String(), errb.String()
}

func runSecurityCheckBase(t *testing.T, repo, allow, base string) (int, string, string) {
	t.Helper()
	return runSecurityCheck(t, repo, allow, "--base", base)
}
