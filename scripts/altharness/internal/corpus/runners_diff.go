package corpus

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runCheck01Diff(ctx context.Context, c Case, beforePath, afterPath, repoRoot string) CheckOutcome {
	script := filepath.Join(repoRoot, "scripts", "lint", "behavior_screener.py")
	if !regularFile(script) {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("missing behavior_screener.py at %s", script)}
	}
	py, errMsg := python3()
	if py == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: "CHECK-01 exec error: " + errMsg}
	}
	cmd := []string{py, script, "--before", beforePath, "--after", afterPath, "--language", c.Language, "--fail-on-candidates"}
	if len(c.Origin.Files) > 0 {
		cmd = append(cmd, "--logical-path", c.Origin.Files[0])
	}
	return runSimple(ctx, subprocessTimeout(), "", "CHECK-01", cmd)
}

func runCheck05Diff(ctx context.Context, _ Case, beforePath, afterPath, repoRoot string) CheckOutcome {
	script := filepath.Join(repoRoot, "scripts", "lint", "check_generated_code.py")
	if !regularFile(script) {
		return CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("missing check_generated_code.py at %s", script)}
	}
	py, errMsg := python3()
	if py == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: "CHECK-05 exec error: " + errMsg}
	}
	return runSimple(ctx, subprocessTimeout(), "", "CHECK-05", []string{py, script, "--before", beforePath, "--after", afterPath})
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

func runCheck06Diff(ctx context.Context, c Case, beforePath, afterPath, repoRoot string) CheckOutcome {
	script := filepath.Join(repoRoot, "scripts", "lint", "check_worktree_hygiene.py")
	git, gitErr := exec.LookPath("git")
	if !regularFile(script) || gitErr != nil {
		return CheckOutcome{ExitCode: -1, RunnerError: "CHECK-06 prerequisites missing (check_worktree_hygiene.py or git)"}
	}
	py, errMsg := python3()
	if py == "" {
		return CheckOutcome{ExitCode: -1, RunnerError: "CHECK-06 exec error: " + errMsg}
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
	return runSimple(ctx, subprocessTimeout(), "", "CHECK-06", []string{py, script, "--repo-root", tmp})
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

func taskPacketSupportsVerify(ctx context.Context, py, script string) bool {
	if !regularFile(script) {
		return false
	}
	out := runSimple(ctx, subprocessTimeout(), "", "VERIFY-01 task_packet --help", []string{py, script, "--help"})
	return out.RunnerError == "" && out.ExitCode == 0 && strings.Contains(out.Output, "verify-sequence")
}

func safeOverlayCopy(tmpRepo, wtDir, rel string) string {
	if !isNormalizedRepoRel(rel) {
		return fmt.Sprintf("VERIFY-01 refused unsafe overlay path: %s", pyRepr(rel))
	}
	repoResolved, err := filepath.EvalSymlinks(tmpRepo)
	if err != nil {
		repoResolved = tmpRepo
	}
	wtResolved, err := filepath.EvalSymlinks(wtDir)
	if err != nil {
		wtResolved = wtDir
	}
	src := filepath.Join(tmpRepo, filepath.FromSlash(rel))
	info, err := os.Lstat(src)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Sprintf("VERIFY-01 refused non-regular or symlink source file: %s", pyRepr(rel))
	}
	if resolved, err := filepath.EvalSymlinks(src); err != nil || !isRelTo(resolved, repoResolved) {
		return fmt.Sprintf("VERIFY-01 refused non-regular or symlink source file: %s", pyRepr(rel))
	}
	cur := wtDir
	parts := strings.Split(rel, "/")
	for _, part := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, part)
		if st, err := os.Lstat(cur); err == nil && st.Mode()&os.ModeSymlink != 0 {
			return fmt.Sprintf("VERIFY-01 refused symlink parent directory in worktree: %s", pyRepr(rel))
		}
	}
	rawDest := filepath.Join(wtDir, filepath.FromSlash(rel))
	if st, err := os.Lstat(rawDest); err == nil && st.Mode()&os.ModeSymlink != 0 {
		_ = os.Remove(rawDest)
	}
	if err := os.MkdirAll(filepath.Dir(rawDest), fsperm.Dir); err != nil {
		return fmt.Sprintf("VERIFY-01 overlay destination escapes worktree: %s", pyRepr(rel))
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(rawDest))
	if err != nil || !isRelTo(parent, wtResolved) {
		return fmt.Sprintf("VERIFY-01 overlay destination escapes worktree: %s", pyRepr(rel))
	}
	if err := copyFile(src, rawDest); err != nil {
		return fmt.Sprintf("VERIFY-01 overlay destination escapes worktree: %s", pyRepr(rel))
	}
	return ""
}

func validateVerify01Commits(ctx context.Context, gitBin, tmpRepo, baseSHA string) ([]string, *CheckOutcome) {
	commitsOut, errMsg := gitStdout(ctx, gitBin, tmpRepo, []string{"rev-list", "--reverse", "--no-merges", baseSHA + "..HEAD"})
	if errMsg != "" {
		o := CheckOutcome{ExitCode: -1, RunnerError: errMsg}
		return nil, &o
	}
	var commits []string
	for _, line := range strings.Split(commitsOut, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			commits = append(commits, line)
		}
	}
	if len(commits) < Verify01MinCommits {
		o := CheckOutcome{ExitCode: 1, Output: fmt.Sprintf("ERROR [SEQUENCE_TOO_FEW_COMMITS]: found %d commit(s)", len(commits))}
		return nil, &o
	}
	c1Out, c1Err := gitStdout(ctx, gitBin, tmpRepo, []string{"diff-tree", "--no-commit-id", "--name-only", "-r", commits[0]})
	laterOut, laterErr := gitStdout(ctx, gitBin, tmpRepo, []string{"diff", "--name-only", commits[0] + "..HEAD"})
	if c1Err != "" || laterErr != "" {
		msg := c1Err
		if msg == "" {
			msg = laterErr
		}
		o := CheckOutcome{ExitCode: -1, RunnerError: msg}
		return nil, &o
	}
	var c1 []string
	later := map[string]struct{}{}
	for _, line := range strings.Split(c1Out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			c1 = append(c1, line)
		}
	}
	for _, line := range strings.Split(laterOut, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			later[line] = struct{}{}
		}
	}
	fail := ""
	badFirst := len(c1) == 0
	for _, path := range c1 {
		if !isVerify01TestFile(path) {
			badFirst = true
		}
	}
	if badFirst {
		fail = fmt.Sprintf("ERROR [SEQUENCE_FIRST_COMMIT_NOT_TEST_ONLY]: %s", pyList(c1))
	} else {
		for _, path := range c1 {
			if _, ok := later[path]; ok {
				fail = "ERROR [SEQUENCE_TEST_MUTATED_IN_IMPL]: test file modified after first commit"
				break
			}
		}
	}
	if fail != "" {
		o := CheckOutcome{ExitCode: 1, Output: fail}
		return nil, &o
	}
	return c1, nil
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

func verify01Baseline(ctx context.Context, gitBin, tmpRepo, baseSHA string, c1, argv []string) *CheckOutcome {
	parent, err := os.MkdirTemp("", "alt-corpus-verify01-base-")
	if err != nil {
		o := CheckOutcome{ExitCode: -1, RunnerError: fmt.Sprintf("VERIFY-01 exec error: %v", err)}
		return &o
	}
	defer os.RemoveAll(parent)
	wt := filepath.Join(parent, "base")
	if msg := gitStep(ctx, gitBin, tmpRepo, []string{"worktree", "add", "--detach", wt, baseSHA}, "VERIFY-01"); msg != "" {
		o := CheckOutcome{ExitCode: -1, RunnerError: msg}
		return &o
	}
	defer gitStep(ctx, gitBin, tmpRepo, []string{"worktree", "remove", "--force", wt}, "VERIFY-01")
	for _, rel := range c1 {
		if msg := safeOverlayCopy(tmpRepo, wt, rel); msg != "" {
			o := CheckOutcome{ExitCode: -1, RunnerError: msg}
			return &o
		}
	}
	base := runSimple(ctx, subprocessTimeout(), wt, "VERIFY-01 baseline test", argv)
	if base.RunnerError != "" {
		o := CheckOutcome{ExitCode: -1, RunnerError: base.RunnerError}
		return &o
	}
	if base.ExitCode != 0 {
		o := CheckOutcome{ExitCode: 1, Output: "ERROR [SEQUENCE_BASELINE_TEST_FAILED]: " + base.Output}
		return &o
	}
	return nil
}

func verify01Fallback(ctx context.Context, gitBin, tmpRepo, baseSHA, testScript, py, bash string) CheckOutcome {
	c1, fail := validateVerify01Commits(ctx, gitBin, tmpRepo, baseSHA)
	if fail != nil {
		return *fail
	}
	argv := []string{py, "./" + testScript}
	if strings.HasSuffix(testScript, ".sh") {
		argv = []string{bash, "./" + testScript}
	}
	if baseFail := verify01Baseline(ctx, gitBin, tmpRepo, baseSHA, c1, argv); baseFail != nil {
		return *baseFail
	}
	final := runSimple(ctx, subprocessTimeout(), tmpRepo, "VERIFY-01 final test", argv)
	if final.RunnerError != "" {
		return CheckOutcome{ExitCode: -1, RunnerError: final.RunnerError}
	}
	if final.ExitCode != 0 {
		return CheckOutcome{ExitCode: 1, Output: "ERROR [SEQUENCE_FINAL_TEST_FAILED]: " + final.Output}
	}
	return CheckOutcome{ExitCode: 0, Output: "VERIFY-01 sequence passed"}
}

func runVerify01Diff(ctx context.Context, c Case, beforePath, afterPath, repoRoot string) CheckOutcome {
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
	bash, bashErr := exec.LookPath("bash")
	if gitErr != nil || (strings.HasSuffix(testScript, ".sh") && bashErr != nil) {
		return CheckOutcome{ExitCode: -1, RunnerError: "VERIFY-01 prerequisites missing (git or bash)"}
	}
	py, errMsg := python3()
	if py == "" && !strings.HasSuffix(testScript, ".sh") {
		return CheckOutcome{ExitCode: -1, RunnerError: "VERIFY-01 exec error: " + errMsg}
	}
	testCmd := py + " ./" + testScript
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
	taskPacket := filepath.Join(repoRoot, "scripts", "agent-harness", "task_packet.py")
	if py != "" && taskPacketSupportsVerify(ctx, py, taskPacket) {
		return runSimple(ctx, subprocessTimeout(), "", "VERIFY-01 task_packet verify-sequence", []string{
			py, taskPacket, "verify-sequence", "--repo-root", tmp, "--base-commit", baseSHA, "--test-command", testCmd,
		})
	}
	return verify01Fallback(ctx, git, tmp, baseSHA, testScript, py, bash)
}
