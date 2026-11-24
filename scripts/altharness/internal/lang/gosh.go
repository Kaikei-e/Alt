package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	golangciLintVersion = "v2.14.0"
	govulncheckVersion  = "v1.6.0"
	golangciModule      = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint"
	govulnModule        = "golang.org/x/vuln/cmd/govulncheck"
)

// RunGo is scripts/lint/go.sh. The canary audit runs before gofmt, go vet,
// golangci-lint, and govulncheck, and a canary failure returns before them.
func RunGo(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) != 1 || args[0] == "" || strings.HasPrefix(args[0], "/") {
		fmt.Fprintln(stderr, "Usage: go.sh <project-dir-relative-to-repo-root>")
		return 2
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	rel := strings.TrimSuffix(strings.TrimPrefix(args[0], "./"), "/")
	project := filepath.Join(root, rel)
	st, err := os.Stat(project)
	if err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "ERROR: project directory not found: %s\n", project)
		return 1
	}
	if abs, err := filepath.Abs(project); err == nil {
		project = abs
	}
	if _, err := os.Stat(filepath.Join(project, "go.mod")); err != nil {
		fmt.Fprintf(stderr, "ERROR: go.mod not found in %s\n", project)
		return 1
	}
	config := filepath.Join(root, ".golangci.yml")
	if _, err := os.Stat(config); err != nil {
		fmt.Fprintf(stderr, "ERROR: Missing golangci-lint config: %s\n", config)
		return 1
	}
	if rc := requireCommands(stderr, "go", "jq", "python3"); rc != 0 {
		return rc
	}
	goVerOut, _, verCode, verErr := capture(ctx, project, nil, "go", "env", "GOVERSION")
	if verErr != nil {
		commandMissing(stderr, "go")
		return 1
	}
	goVer := strings.TrimSpace(goVerOut)
	if verCode != 0 || goVer == "" {
		fmt.Fprintln(stderr, "ERROR: Could not determine Go toolchain version via 'go env GOVERSION'")
		return 1
	}
	base, err := cacheHome()
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	cache := filepath.Join(base, "alt-lint", goVer)
	lintBin, rc := ensureGoTool(ctx, stderr, root, cache, "golangci-lint", golangciModule, golangciLintVersion)
	if rc != 0 {
		return rc
	}
	vulnBin, rc := ensureGoTool(ctx, stderr, root, cache, "govulncheck", govulnModule, govulncheckVersion)
	if rc != 0 {
		return rc
	}
	env := []string{"GOFLAGS=-buildvcs=false"}
	canary := filepath.Join(root, "scripts", "lint", "canary_audit.py")
	if _, err := os.Stat(canary); err != nil {
		fmt.Fprintf(stderr, "ERROR: Missing canary audit helper: %s\n", canary)
		return 1
	}
	canaryDir, err := os.MkdirTemp("", "alt-go-canary-")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	defer os.RemoveAll(canaryDir)
	govulnFile, err := os.CreateTemp("", "alt-govulncheck-*.json")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	govulnPath := govulnFile.Name()
	govulnFile.Close()
	defer os.Remove(govulnPath)

	fmt.Fprintf(stdout, "==> [%s] canary audit (golangci-lint incl. gosec SARIF)\n", rel)
	canaryCode, canaryErr := run(ctx, stdout, stderr, project, nil, "python3", canary, "go-canary",
		"--project-dir", project, "--config", config, "--golangci-lint-bin", lintBin, "--work-dir", canaryDir)
	if canaryErr != nil || canaryCode != 0 {
		return 1
	}

	status := map[string]string{
		"canary": "PASS", "gofmt": "PASS", "govet": "PASS",
		"golangci": "PASS", "gosec": "PASS", "govulncheck": "PASS",
	}
	overall := 0
	fmt.Fprintf(stdout, "==> [%s] gofmt\n", rel)
	gofmtOut, gofmtErrText, gofmtCode, gofmtErr := capture(ctx, project, env, "gofmt", "-l", ".")
	if gofmtErr != nil {
		commandMissing(stderr, "gofmt")
		return 1
	}
	if gofmtCode != 0 {
		fmt.Fprint(stderr, gofmtErrText)
		return gofmtCode
	}
	var bad []string
	for _, line := range strings.Split(strings.TrimRight(gofmtOut, "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, "gen/") {
			continue
		}
		bad = append(bad, line)
	}
	if len(bad) > 0 {
		fmt.Fprintln(stderr, "ERROR: Run gofmt -w on these files:")
		fmt.Fprintln(stderr, strings.Join(bad, "\n"))
		status["gofmt"] = "FAIL"
		overall = 1
	}

	fmt.Fprintf(stdout, "==> [%s] go vet\n", rel)
	if vetCode, vetErr := run(ctx, stdout, stderr, project, env, "go", "vet", "./..."); vetErr != nil {
		commandMissing(stderr, "go")
		return 1
	} else if vetCode != 0 {
		status["govet"] = "FAIL"
		overall = 1
	}

	reports := filepath.Join(project, "_reports")
	if err := os.MkdirAll(reports, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	for _, name := range []string{"golangci-lint.sarif", "gosec.sarif", "gosec.sarif.tmp", "govulncheck.sarif", "govulncheck.sarif.tmp", "govulncheck.stderr"} {
		_ = os.Remove(filepath.Join(reports, name))
	}
	policy := os.Getenv("LINT_POLICY")
	if policy == "" {
		policy = "fix-existing"
	}
	fmt.Fprintf(stdout, "==> [%s] golangci-lint (incl. gosec SARIF)\n", rel)
	sarif := filepath.Join(reports, "golangci-lint.sarif")
	lintCode, lintErr := run(ctx, stdout, stderr, project, env, lintBin, "run", "--config", config, "--output.text.path", "stdout", "--output.sarif.path", sarif)
	if lintErr != nil {
		fmt.Fprintf(stderr, "ERROR: golangci-lint failed to start: %s\n", lintErr.Error())
		return 1
	}
	if lintCode != 0 && lintCode != 1 {
		_ = os.Remove(sarif)
		fmt.Fprintf(stderr, "ERROR: golangci-lint failed to run (exit code %d)\n", lintCode)
		return lintCode
	}
	if info, err := os.Stat(sarif); err != nil || info.Size() == 0 {
		_ = os.Remove(sarif)
		fmt.Fprintf(stderr, "ERROR: golangci-lint did not produce ./_reports/golangci-lint.sarif (exit code %d)\n", lintCode)
		return 1
	}
	ratchet := filepath.Join(root, "scripts", "lint", "check_ratchet.py")
	if lintCode == 1 {
		if policy == "block-new" {
			fp, fpErr := run(ctx, stdout, stderr, project, nil, "python3", ratchet, "check-fingerprints",
				"--repo-root", root, "--project-dir", project, "--golangci-sarif", "./_reports/golangci-lint.sarif", "--require-findings")
			if fpErr != nil || fp != 0 {
				status["golangci"] = "FAIL"
				overall = 1
			} else {
				status["golangci"] = "PASS (block-new)"
			}
		} else {
			status["golangci"] = "FAIL"
			overall = 1
		}
	}
	gosec := filepath.Join(reports, "gosec.sarif")
	if rc := jqInto(ctx, project, stderr, gosecJQ, sarif, gosec); rc != 0 {
		return rc
	}
	_ = os.Remove(sarif)
	if code, err := run(ctx, stdout, stderr, project, nil, "python3", canary, "go-verify-scan", "--project-dir", project, "--gosec-sarif", "./_reports/gosec.sarif"); err != nil || code != 0 {
		return 1
	}
	countOut, _, countCode, countErr := capture(ctx, project, nil, "jq", `[.runs[]?.results[]?] | length`, gosec)
	if countErr != nil || countCode != 0 {
		fmt.Fprintln(stderr, "ERROR: failed to count gosec results")
		return 1
	}
	if n, _ := strconv.Atoi(strings.TrimSpace(countOut)); n > 0 {
		if policy == "block-new" {
			fp, fpErr := run(ctx, stdout, stderr, project, nil, "python3", ratchet, "check-fingerprints",
				"--repo-root", root, "--project-dir", project, "--golangci-sarif", "./_reports/gosec.sarif", "--require-findings")
			if fpErr != nil || fp != 0 {
				status["gosec"] = "FAIL"
				overall = 1
			} else {
				status["gosec"] = "PASS (block-new)"
			}
		} else {
			status["gosec"] = "FAIL"
			overall = 1
		}
	}

	fmt.Fprintf(stdout, "==> [%s] govulncheck (SARIF)\n", rel)
	if rc := runGovuln(ctx, project, env, vulnBin, []string{"-format=sarif", "./..."}, filepath.Join(reports, "govulncheck.sarif"), filepath.Join(reports, "govulncheck.stderr"), stderr, "ERROR: govulncheck execution failed"); rc != 0 {
		return rc
	}
	if info, err := os.Stat(filepath.Join(reports, "govulncheck.sarif")); err != nil || info.Size() == 0 {
		fmt.Fprintln(stderr, "ERROR: govulncheck did not produce a non-empty ./_reports/govulncheck.sarif")
		return 1
	}
	if rc := runGovuln(ctx, project, env, vulnBin, []string{"-format=json", "./..."}, govulnPath, filepath.Join(reports, "govulncheck.stderr"), stderr, "ERROR: govulncheck JSON coverage run failed"); rc != 0 {
		return rc
	}
	if code, err := run(ctx, stdout, stderr, project, nil, "python3", canary, "go-verify-govulncheck",
		"--project-dir", project, "--govulncheck-sarif", "./_reports/govulncheck.sarif", "--govulncheck-json", govulnPath); err != nil || code != 0 {
		return 1
	}
	unique := filepath.Join(reports, "govulncheck.sarif.tmp")
	if rc := jqInto(ctx, project, stderr, `(.runs[]?.results[]?.stacks | select(type == "array")) |= unique`, filepath.Join(reports, "govulncheck.sarif"), unique); rc != 0 {
		return rc
	}
	if err := os.Rename(unique, filepath.Join(reports, "govulncheck.sarif")); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	if code, err := run(ctx, stderr, stderr, project, nil, "jq", "-r", govulnMessageJQ, filepath.Join(reports, "govulncheck.sarif")); err != nil || code != 0 {
		fmt.Fprintln(stderr, "ERROR: failed to render govulncheck findings")
		return 1
	}
	calledOut, _, calledCode, calledErr := capture(ctx, project, nil, "jq", `[.runs[]?.results[]? | select(.level == "error")] | length`, filepath.Join(reports, "govulncheck.sarif"))
	if calledErr != nil || calledCode != 0 {
		fmt.Fprintln(stderr, "ERROR: failed to count govulncheck results")
		return 1
	}
	if n, _ := strconv.Atoi(strings.TrimSpace(calledOut)); n > 0 {
		status["govulncheck"] = "FAIL"
		overall = 1
	}
	fmt.Fprintf(stdout, "==> [%s] summary\n", rel)
	for _, item := range []struct{ name, key string }{
		{"canary", "canary"}, {"gofmt", "gofmt"}, {"go vet", "govet"},
		{"golangci-lint", "golangci"}, {"gosec", "gosec"}, {"govulncheck", "govulncheck"},
	} {
		fmt.Fprintf(stdout, "  %-15s %s\n", item.name, status[item.key])
	}
	return overall
}

func ensureGoTool(ctx context.Context, stderr io.Writer, root, cache, name, module, version string) (string, int) {
	bin := filepath.Join(cache, name+"-"+version, name)
	if st, err := os.Stat(bin); err == nil && st.Mode()&0o111 != 0 {
		return bin, 0
	}
	binDir := filepath.Dir(bin)
	if err := os.MkdirAll(binDir, fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return "", 1
	}
	fmt.Fprintf(stderr, "Installing %s@%s into %s (%s)...\n", name, version, binDir, filepath.Base(cache))
	code, err := run(ctx, stderr, stderr, root, []string{"GOBIN=" + binDir}, "go", "install", module+"@"+version)
	if err != nil {
		if _, lookErr := exec.LookPath("go"); lookErr != nil {
			commandMissing(stderr, "go")
		} else {
			fmt.Fprintf(stderr, "ERROR: go install %s@%s: %s\n", name, version, err.Error())
		}
		return "", 1
	}
	if code != 0 {
		return "", code
	}
	return bin, 0
}

func runGovuln(ctx context.Context, dir string, env []string, bin string, args []string, stdoutPath, stderrPath string, userErr io.Writer, failText string) int {
	out, err := os.Create(stdoutPath)
	if err != nil {
		fmt.Fprintf(userErr, "ERROR: %s\n", err.Error())
		return 1
	}
	errFile, err := os.Create(stderrPath)
	if err != nil {
		out.Close()
		fmt.Fprintf(userErr, "ERROR: %s\n", err.Error())
		return 1
	}
	code, runErr := run(ctx, out, errFile, dir, env, append([]string{bin}, args...)...)
	out.Close()
	errFile.Close()
	if data, readErr := os.ReadFile(stderrPath); readErr == nil && len(data) > 0 {
		_, _ = userErr.Write(data)
	}
	_ = os.Remove(stderrPath)
	if runErr != nil || code != 0 {
		fmt.Fprintln(userErr, failText)
		return 1
	}
	return 0
}

func jqInto(ctx context.Context, dir string, stderr io.Writer, program, input, output string) int {
	tmp := output + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	code, runErr := run(ctx, out, stderr, dir, nil, "jq", program, input)
	out.Close()
	if runErr != nil {
		_ = os.Remove(tmp)
		commandMissing(stderr, "jq")
		return 1
	}
	if code != 0 {
		_ = os.Remove(tmp)
		return code
	}
	if err := os.Rename(tmp, output); err != nil {
		fmt.Fprintf(stderr, "ERROR: %s\n", err.Error())
		return 1
	}
	return 0
}

const gosecJQ = `
  .runs = ((.runs // []) | map(
    (.artifacts == null) as $no_artifacts
    | .tool.driver.name = "gosec"
    | .results = ((.results // []) | map(
        select(.ruleId == "gosec")
        | .ruleId = (try (.message.text // "" | capture("^(?<id>G[0-9]{3}):").id) catch "gosec")
        | del(.fixes)
        | if $no_artifacts and (.locations | type == "array") then
            .locations |= map(
              if (.physicalLocation.artifactLocation | type == "object") then
                del(.physicalLocation.artifactLocation.index)
              else .
              end
            )
          else .
          end
      ))
    | .tool.driver.rules = (
        .results
        | map({
            id: .ruleId,
            shortDescription: {
              text: ((.message.text // .ruleId) | split("\n")[0])
            }
          })
        | unique_by(.id)
      )
  ))
`

const govulnMessageJQ = `
  .runs[]? as $run |
  ($run.tool.driver.rules // [] | map({(.id): .}) | add // {}) as $rules |
  $run.results[]? |
  ($rules[.ruleId] // {}) as $rule |
  (
    [
      .stacks[]?.frames[-1]?.module,
      .codeFlows[]?.threadFlows[]?.locations[-1]?.module
    ]
    | map(select(. != null and . != ""))
    | unique
    | join(", ")
  ) as $mods |
  (
    .properties.fixedVersion // .properties.fixed_version //
    $rule.properties.fixedVersion // $rule.properties.fixed_version // ""
  ) as $fixed |
  (if .level == "error" then "ERROR" else "INFO" end) +
  " govulncheck " + .ruleId +
  (if $mods != "" then " module=" + $mods else "" end) +
  (if $fixed != "" then " fixed=" + $fixed else "" end) +
  ": " + (.message.text // "")
`
