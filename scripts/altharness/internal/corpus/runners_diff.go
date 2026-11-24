package corpus

import (
	"alt/scripts/altharness/internal/fsperm"
	"alt/scripts/altharness/internal/generated"
	"alt/scripts/altharness/internal/hygiene"
	"alt/scripts/altharness/internal/packet"
	"alt/scripts/altharness/internal/screener"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runCheck01Diff(ctx context.Context, c Case, beforePath, afterPath, _ string) CheckOutcome {
	args := []string{"--before", beforePath, "--after", afterPath, "--language", c.Language, "--fail-on-candidates"}
	if len(c.Origin.Files) > 0 {
		args = append(args, "--logical-path", c.Origin.Files[0])
	}
	return runInProcess(ctx, subprocessTimeout(), "CHECK-01", func(cctx context.Context, stdout, stderr io.Writer) int {
		return screener.Run(cctx, args, stdout, stderr)
	})
}

func runCheck05Diff(ctx context.Context, _ Case, beforePath, afterPath, _ string) CheckOutcome {
	args := []string{"--before", beforePath, "--after", afterPath}
	return runInProcess(ctx, subprocessTimeout(), "CHECK-05", func(cctx context.Context, stdout, stderr io.Writer) int {
		return generated.Run(cctx, args, stdout, stderr)
	})
}

func gitStep(ctx context.Context, gitBin, repo string, args []string, prefix string) string {
	stdout, stderr, code, errMsg := runCaptured(ctx, subprocessTimeout(), "", prefix+" git "+args[0], append([]string{gitBin, "-C", repo}, args...))
	if errMsg != "" {
		return errMsg
	}
	if code != 0 {
		detail := stderr
		if strings.TrimSpace(detail) == "" {
			detail = stdout
		}
		return fmt.Sprintf("%s git %s failed: %s", prefix, args[0], snippet(detail))
	}
	return ""
}

func gitStdout(ctx context.Context, gitBin, repo string, args []string) (string, string) {
	stdout, stderr, code, errMsg := runCaptured(ctx, subprocessTimeout(), "", "VERIFY-01 git "+args[0], append([]string{gitBin, "-C", repo}, args...))
	if errMsg != "" {
		return "", errMsg
	}
	if code != 0 {
		detail := stderr
		if strings.TrimSpace(detail) == "" {
			detail = stdout
		}
		return "", fmt.Sprintf("VERIFY-01 git %s failed: %s", args[0], snippet(detail))
	}
	return stdout, ""
}

func runCheck06Diff(ctx context.Context, c Case, beforePath, afterPath, _ string) CheckOutcome {
	git, gitErr := exec.LookPath("git")
	if gitErr != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: "CHECK-06 prerequisites missing (git)"}
	}
	tmp, err := os.MkdirTemp("", "alt-corpus-check06-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("CHECK-06 exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	for _, step := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Corpus Runner"},
		{"config", "user.email", "corpus@example.com"},
	} {
		if msg := gitStep(ctx, git, tmp, step, "CHECK-06"); msg != "" {
			return CheckOutcome{ExitCode: -1, RunnerError: msg}
		}
	}
	tracked := "sample" + filepath.Ext(beforePath)
	if c.Language == "binary" {
		tracked = "main.go"
	}
	if err := copyFile(beforePath, filepath.Join(tmp, tracked)); err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("CHECK-06 exec error: %v", err)}
	}
	for _, step := range [][]string{{"add", tracked}, {"commit", "-q", "-m", "baseline"}} {
		if msg := gitStep(ctx, git, tmp, step, "CHECK-06"); msg != "" {
			return CheckOutcome{ExitCode: -1, RunnerError: msg}
		}
	}
	if c.Language == "binary" {
		if afterPath != beforePath {
			if err := copyFile(afterPath, filepath.Join(tmp, c.AfterFile)); err != nil {
				return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("CHECK-06 exec error: %v", err)}
			}
		}
	} else {
		raw, err := os.ReadFile(afterPath)
		if err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("CHECK-06 exec error: %v", err)}
		}
		if err := os.WriteFile(filepath.Join(tmp, tracked), raw, fsperm.File); err != nil {
			return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("CHECK-06 exec error: %v", err)}
		}
	}
	args := []string{"--repo-root", tmp}
	return runInProcess(ctx, subprocessTimeout(), "CHECK-06", func(cctx context.Context, stdout, stderr io.Writer) int {
		return hygiene.Run(cctx, args, stdout, stderr)
	})
}

func isVerify01TestFile(rel string) bool { return verify01TestFileRE.MatchString(rel) }

func verify01TargetFilename(c Case, beforePath string) string {
	if len(c.Origin.Files) > 0 {
		candidate := filepath.Base(c.Origin.Files[0])
		if candidate != "" && candidate != "." && candidate != ".." {
			return candidate
		}
	}
	return "sample" + filepath.Ext(beforePath)
}

func stageVerify01Repo(ctx context.Context, c Case, beforePath, afterPath, gitBin, tmpRepo string) (string, string) {
	targetName := verify01TargetFilename(c, beforePath)
	targetPath := filepath.Join(tmpRepo, targetName)
	if err := copyFile(beforePath, targetPath); err != nil {
		return "", fmt.Sprintf("VERIFY-01 exec error: %v", err)
	}
	markExecutableIfShell(targetPath)
	for _, step := range [][]string{
		{"init", "-q"},
		{"config", "user.name", "Corpus Runner"},
		{"config", "user.email", "corpus@example.com"},
		{"add", targetName},
		{"commit", "-q", "-m", "base: before state"},
	} {
		if msg := gitStep(ctx, gitBin, tmpRepo, step, "VERIFY-01"); msg != "" {
			return "", msg
		}
	}
	baseOut, errMsg := gitStdout(ctx, gitBin, tmpRepo, []string{"rev-parse", "HEAD"})
	if errMsg != "" {
		return "", errMsg
	}
	baseSHA := strings.TrimSpace(baseOut)
	for _, sf := range c.SupportFiles {
		dest := filepath.Join(tmpRepo, sf)
		if err := copyFile(filepath.Join(c.CaseDir, sf), dest); err != nil {
			return "", fmt.Sprintf("VERIFY-01 exec error: %v", err)
		}
		markExecutableIfShell(dest)
	}
	add := append([]string{"add"}, c.SupportFiles...)
	for _, step := range [][]string{add, {"commit", "-q", "-m", "test: add characterization test"}} {
		if msg := gitStep(ctx, gitBin, tmpRepo, step, "VERIFY-01"); msg != "" {
			return "", msg
		}
	}
	if afterPath == beforePath {
		if msg := gitStep(ctx, gitBin, tmpRepo, []string{"commit", "-q", "--allow-empty", "-m", "impl: preserve behavior"}, "VERIFY-01"); msg != "" {
			return "", msg
		}
	} else {
		raw, err := os.ReadFile(afterPath)
		if err != nil {
			return "", fmt.Sprintf("VERIFY-01 exec error: %v", err)
		}
		if err := os.WriteFile(targetPath, raw, fsperm.File); err != nil {
			return "", fmt.Sprintf("VERIFY-01 exec error: %v", err)
		}
		markExecutableIfShell(targetPath)
		for _, step := range [][]string{{"add", targetName}, {"commit", "-q", "-m", "impl: apply change"}} {
			if msg := gitStep(ctx, gitBin, tmpRepo, step, "VERIFY-01"); msg != "" {
				return "", msg
			}
		}
	}
	return baseSHA, ""
}

func runVerify01Diff(ctx context.Context, c Case, beforePath, afterPath, _ string) CheckOutcome {
	testScript := ""
	for _, sf := range c.SupportFiles {
		if isVerify01TestFile(sf) {
			testScript = sf
			break
		}
	}
	if testScript == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("VERIFY-01 case %s missing test script in support_files", c.CaseID)}
	}
	git, gitErr := exec.LookPath("git")
	_, bashErr := exec.LookPath("bash")
	if gitErr != nil || (strings.HasSuffix(testScript, ".sh") && bashErr != nil) {
		return CheckOutcome{ExitCode: -1, RunnerError: "VERIFY-01 prerequisites missing (git or bash)"}
	}
	testCmd := "./" + testScript
	if strings.HasSuffix(testScript, ".sh") {
		testCmd = "bash ./" + testScript
	}
	tmp, err := os.MkdirTemp("", "alt-corpus-verify01-")
	if err != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("VERIFY-01 exec error: %v", err)}
	}
	defer os.RemoveAll(tmp)
	baseSHA, msg := stageVerify01Repo(ctx, c, beforePath, afterPath, git, tmp)
	if msg != "" {
		return CheckOutcome{ExitCode: -1, RunnerError: msg}
	}
	args := []string{
		"verify-sequence",
		"--repo-root", tmp,
		"--base-commit", baseSHA,
		"--test-command", testCmd,
	}
	return runInProcess(ctx, subprocessTimeout(), "VERIFY-01 task_packet verify-sequence", func(cctx context.Context, stdout, stderr io.Writer) int {
		return packet.Run(cctx, args, stdout, stderr)
	})
}
