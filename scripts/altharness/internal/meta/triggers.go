package meta

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var forbiddenTrigger = regexp.MustCompile(`^[[:space:]]*(pull_request_target|workflow_run)[[:space:]]*:`)

// ForbiddenTriggers scans workflow files for elevated-trust triggers.
// Zero workflow files is a failure, not a pass.
func ForbiddenTriggers(ctx context.Context, repo string) (string, int, error) {
	if ctx.Err() != nil {
		return "", 1, ctx.Err()
	}
	dir := filepath.Join(repo, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", 1, fmt.Errorf("ERROR: no workflow files found under .github/workflows/: %w", err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	if len(files) == 0 {
		return "ERROR: no workflow files found under .github/workflows/\n", 1, nil
	}
	var b strings.Builder
	violations := 0
	for _, f := range files {
		rel, _ := filepath.Rel(repo, f)
		fh, err := os.Open(f)
		if err != nil {
			return "", 1, fmt.Errorf("ERROR: grep failed on %s: %w", rel, err)
		}
		sc := bufio.NewScanner(fh)
		lineNo := 0
		fileHit := false
		for sc.Scan() {
			lineNo++
			if forbiddenTrigger.MatchString(sc.Text()) {
				fmt.Fprintf(&b, "::error file=%s,line=%d::Forbidden trigger — 'pull_request_target' and 'workflow_run' run with elevated trust and must not be introduced (ADR-000763 security hardening).\n", rel, lineNo)
				fileHit = true
			}
		}
		fh.Close()
		if err := sc.Err(); err != nil {
			return "", 1, fmt.Errorf("ERROR: grep failed on %s: %w", rel, err)
		}
		if fileHit {
			violations++
		}
	}
	if violations > 0 {
		fmt.Fprintf(&b, "FAIL: %d workflow file(s) use forbidden triggers (grep check).\n", violations)
		return b.String(), 1, nil
	}
	b.WriteString("OK: grep check clean.\n")
	return b.String(), 0, nil
}

func astStep(repo, name string, args ...string) Step {
	return Step{
		Name: name, Parallel: true, Cache: true, Dir: repo,
		Run: func(ctx context.Context) (string, int, error) {
			bin, err := lookAstGrep()
			if err != nil {
				return "ERROR: ast-grep not found in PATH or ~/.local/bin/ast-grep\n", 1, nil
			}
			cmd := exec.CommandContext(ctx, bin, args...)
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if err == nil {
				return string(out), 0, nil
			}
			if ee, ok := err.(*exec.ExitError); ok {
				return string(out), ee.ExitCode(), nil
			}
			return string(out), 1, err
		},
	}
}

func gofmtStep(repo, name string, paths ...string) Step {
	return Step{
		Name: name, Parallel: true, Cache: true, Dir: repo,
		Run: func(ctx context.Context) (string, int, error) {
			gofmtBin, err := exec.LookPath("gofmt")
			if err != nil {
				return "ERROR: gofmt not found in PATH\n", 1, nil
			}
			args := append([]string{"-s", "-l"}, paths...)
			cmd := exec.CommandContext(ctx, gofmtBin, args...)
			cmd.Dir = repo
			out, err := cmd.CombinedOutput()
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					return string(out), ee.ExitCode(), nil
				}
				return string(out), 1, err
			}
			unformatted := strings.TrimSpace(string(out))
			if unformatted != "" {
				return "ERROR: unformatted Go files (run gofmt -s -w):\n" + unformatted + "\n", 1, nil
			}
			return "OK: gofmt -s clean.\n", 0, nil
		},
	}
}
