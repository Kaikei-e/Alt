// Package preflight implements `alt-harness preflight`, a pure-Go port of
// `.claude/skills/adr-preflight/scripts/adr_preflight.py`.
package preflight

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	maxBriefADRs     = 8
	maxTitleChars    = 120
	maxBriefChars    = 6000
	maxFindingLines  = 40
	testReceiptScope = "__tests__"
	workflowRelPath  = ".github/workflows/adr-graph.yml"
	selfServiceCmd   = "./scripts/altharness/alt-harness preflight"
)

var scopeAliases = map[string][]string{
	"alt-backend": {
		"alt-backend",
		"alt-harvester",
		"alt-notifier",
		"alt-data-hub",
	},
	".github": {".github", "ci-cd"},
}

var patchLinePrefixes = []string{
	"*** Add File: ",
	"*** Update File: ",
	"*** Delete File: ",
	"*** Move to: ",
}

var (
	sanitiseRe      = regexp.MustCompile(`[^A-Za-z0-9_.-]`)
	docdagVersionRe = regexp.MustCompile(`(?m)^\s*DOCDAG_VERSION:\s*([^\s#]+)`)
	adrFileRelRe    = regexp.MustCompile(`^docs/ADR/(\d{6})\.md$`)
)

var testPathSegments = map[string]struct{}{
	"tests":     {},
	"test":      {},
	"__tests__": {},
	"e2e":       {},
	"testdata":  {},
	"fixtures":  {},
}

var testFileSuffixes = []string{
	"_test.go",
	"_test.py",
	".test.ts",
	".spec.ts",
	".test.tsx",
	".spec.tsx",
	".test.js",
	".spec.js",
	".test.mjs",
	"_test.ts",
	"_test.tsx",
	"_test.rs",
	"_test.sh",
}

var errMissingDocdag = errors.New("docdag is not on PATH")

// Options configures a single preflight invocation without touching process globals.
type Options struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Cwd       string
	Env       map[string]string
	AnchorDir string
}

type validateResult struct {
	missingBinary bool
	returncode    int
	output        string
	summaryLine   string
}

type bindingADR struct {
	adrID            string
	title            string
	relPath          string
	affectedServices []string
}

type gitIdentity struct {
	root      string
	commonDir string
}

type scopeSection struct {
	scope string
	adrs  []bindingADR
}

type runner struct {
	ctx       context.Context
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	cwd       string
	env       map[string]string
	anchorDir string
}

// Run executes `alt-harness preflight` using process environment and working directory.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return RunWithOptions(ctx, args, Options{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
	})
}

// RunWithOptions executes `alt-harness preflight` with explicit IO, cwd, env, and anchor directory.
func RunWithOptions(ctx context.Context, args []string, opts Options) int {
	if ctx == nil {
		ctx = context.Background()
	}
	stdin := opts.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	cwd := opts.Cwd
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	env := opts.Env
	if env == nil {
		env = currentEnvMap()
	}

	r := &runner{
		ctx:       ctx,
		stdin:     stdin,
		stdout:    stdout,
		stderr:    stderr,
		cwd:       cwd,
		env:       env,
		anchorDir: opts.AnchorDir,
	}
	return r.main(args)
}

func currentEnvMap() map[string]string {
	m := make(map[string]string)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			m[k] = v
		}
	}
	return m
}

func (r *runner) envSlice() []string {
	keys := make([]string, 0, len(r.env))
	for k := range r.env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+r.env[k])
	}
	return out
}

func (r *runner) isDisabled() bool {
	return r.env["ALT_ADR_PREFLIGHT"] == "disabled"
}

func (r *runner) getStateDir() string {
	if override := r.env["ALT_ADR_PREFLIGHT_STATE_DIR"]; override != "" {
		return override
	}
	if xdg := r.env["XDG_STATE_HOME"]; xdg != "" {
		return filepath.Join(xdg, "alt-adr-preflight")
	}
	home := r.env["HOME"]
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".local", "state", "alt-adr-preflight")
}

func lookPathInEnv(name string, env map[string]string) (string, error) {
	if strings.ContainsRune(name, os.PathSeparator) {
		st, err := os.Stat(name)
		if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			return "", exec.ErrNotFound
		}
		return name, nil
	}
	for _, dir := range filepath.SplitList(env["PATH"]) {
		if dir == "" {
			dir = "."
		}
		candidate := filepath.Join(dir, name)
		st, err := os.Stat(candidate)
		if err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

func (r *runner) runCmd(dir, name string, args ...string) (stdout, stderr string, code int, notFound bool, err error) {
	bin, lerr := lookPathInEnv(name, r.env)
	if lerr != nil {
		return "", "", 127, true, lerr
	}
	cmd := exec.CommandContext(r.ctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = r.envSlice()
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()
	if runErr == nil {
		return outBuf.String(), errBuf.String(), 0, false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		return outBuf.String(), errBuf.String(), exitErr.ExitCode(), false, nil
	}
	return outBuf.String(), errBuf.String(), 1, false, runErr
}

func sanitiseToken(raw, fallback string) string {
	cleaned := sanitiseRe.ReplaceAllString(raw, "_")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return fallback
	}
	return cleaned
}

func serviceNamesForScope(scope string) []string {
	if aliases, ok := scopeAliases[scope]; ok {
		return aliases
	}
	return []string{scope}
}

func deriveScopeFromRelpath(relPath string, isDir bool) string {
	norm := filepath.ToSlash(filepath.Clean(relPath))
	if norm == "" || norm == "." {
		return ""
	}
	parts := strings.Split(norm, "/")
	if len(parts) == 1 && !isDir {
		return "repo-root"
	}
	return parts[0]
}

func isExemptRelpath(relPath, scope string) bool {
	if scope == "" || scope == "docs" {
		return true
	}
	return filepath.ToSlash(filepath.Clean(relPath)) == "docdag.yaml"
}

func isTestRelpath(relPath string) bool {
	norm := filepath.ToSlash(filepath.Clean(relPath))
	if norm == "" || norm == "." {
		return false
	}
	parts := strings.Split(norm, "/")
	for _, seg := range parts {
		if _, ok := testPathSegments[seg]; ok {
			return true
		}
	}
	base := parts[len(parts)-1]
	if base == "conftest.py" {
		return true
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return true
	}
	if strings.HasPrefix(base, "test-") && strings.HasSuffix(base, ".sh") {
		return true
	}
	for _, suffix := range testFileSuffixes {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

func unquoteYAMLScalar(raw string) string {
	val := strings.TrimSpace(raw)
	if len(val) >= 2 && val[0] == val[len(val)-1] && (val[0] == '"' || val[0] == '\'') {
		return strings.TrimSpace(val[1 : len(val)-1])
	}
	return val
}

func extractPatchPaths(commandText string) []string {
	var extracted []string
	for _, line := range splitLines(commandText) {
		for _, prefix := range patchLinePrefixes {
			if strings.HasPrefix(line, prefix) {
				candidate := strings.TrimSpace(line[len(prefix):])
				if candidate != "" {
					extracted = append(extracted, candidate)
				}
				break
			}
		}
	}
	return extracted
}

func extractHookPaths(payload map[string]any) []string {
	var paths []string

	if tc, ok := payload["toolCall"].(map[string]any); ok {
		tcName, _ := tc["name"].(string)
		if tcArgs, ok := tc["args"].(map[string]any); ok {
			for _, field := range []string{"Path", "TargetFile", "file_path", "notebook_path"} {
				if v, ok := tcArgs[field].(string); ok && strings.TrimSpace(v) != "" {
					paths = append(paths, strings.TrimSpace(v))
				}
			}
			if tcName == "apply_patch" {
				paths = append(paths, extractApplyPatchCmd(tcArgs["command"])...)
			}
		}
	}

	if ti, ok := payload["tool_input"].(map[string]any); ok {
		for _, field := range []string{"file_path", "notebook_path", "Path", "TargetFile"} {
			if v, ok := ti[field].(string); ok && strings.TrimSpace(v) != "" {
				paths = append(paths, strings.TrimSpace(v))
			}
		}
		if toolName, _ := payload["tool_name"].(string); toolName == "apply_patch" {
			paths = append(paths, extractApplyPatchCmd(ti["command"])...)
		}
	}

	return paths
}

func extractApplyPatchCmd(raw any) []string {
	switch v := raw.(type) {
	case string:
		return extractPatchPaths(v)
	case []any:
		var parts []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return extractPatchPaths(strings.Join(parts, "\n"))
	default:
		return nil
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func capLines(text string, limit int) string {
	trimmed := strings.Trim(text, "\n")
	if trimmed == "" {
		return ""
	}
	lines := splitLines(trimmed)
	if len(lines) <= limit {
		return strings.Join(lines, "\n")
	}
	kept := append([]string(nil), lines[:limit-1]...)
	omitted := len(lines) - (limit - 1)
	kept = append(kept, fmt.Sprintf("... (%d more lines omitted)", omitted))
	return strings.Join(kept, "\n")
}

func resolveRealPath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return abs
}

func (r *runner) resolveGitDir(directory string) (*gitIdentity, bool) {
	resolved := resolveRealPath(directory)
	out, _, code, notFound, err := r.runCmd("", "git", "-C", resolved, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	if notFound || err != nil || code != 0 {
		return nil, false
	}
	var lines []string
	for _, ln := range splitLines(out) {
		if t := strings.TrimSpace(ln); t != "" {
			lines = append(lines, t)
		}
	}
	if len(lines) < 2 {
		return nil, false
	}
	return &gitIdentity{
		root:      resolveRealPath(lines[0]),
		commonDir: resolveRealPath(lines[1]),
	}, true
}

func (r *runner) getScriptGitIdentity() (gitIdentity, error) {
	if r.anchorDir != "" {
		if info, ok := r.resolveGitDir(r.anchorDir); ok {
			return *info, nil
		}
		return gitIdentity{}, errors.New("alt-harness preflight is not inside a git repository")
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(resolveRealPath(exe))
		if info, ok := r.resolveGitDir(exeDir); ok {
			return *info, nil
		}
	}
	if projectDir := r.env["CLAUDE_PROJECT_DIR"]; projectDir != "" {
		if info, ok := r.resolveGitDir(projectDir); ok {
			return *info, nil
		}
	}
	if r.cwd != "" {
		if info, ok := r.resolveGitDir(r.cwd); ok {
			return *info, nil
		}
	}
	return gitIdentity{}, errors.New("alt-harness preflight is not inside a git repository")
}

func isExistingDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func (r *runner) resolveFileInRepo(
	rawPath string,
	baseCwd string,
	scriptCommonDir string,
	gitDirCache map[string]*gitIdentity,
) (root, relPath string, isDir bool, ok bool) {
	var joined string
	if filepath.IsAbs(rawPath) {
		joined = filepath.Clean(rawPath)
	} else {
		joined = filepath.Clean(filepath.Join(baseCwd, rawPath))
	}
	if abs, err := filepath.Abs(joined); err == nil {
		joined = abs
	}

	targetIsDir := isExistingDir(joined)
	probe := joined
	if !targetIsDir {
		probe = filepath.Dir(joined)
	}
	for !isExistingDir(probe) {
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", "", false, false
		}
		probe = parent
	}

	probeReal := resolveRealPath(probe)
	tail, err := filepath.Rel(probe, joined)
	if err != nil {
		return "", "", false, false
	}
	absTarget := filepath.Clean(filepath.Join(probeReal, tail))

	gitInfo, cached := gitDirCache[probeReal]
	if !cached {
		resolved, found := r.resolveGitDir(probeReal)
		if found {
			gitInfo = resolved
		}
		gitDirCache[probeReal] = gitInfo
	}
	if gitInfo == nil || gitInfo.commonDir != scriptCommonDir {
		return "", "", false, false
	}

	rel, err := filepath.Rel(gitInfo.root, absTarget)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", "", false, false
	}
	return gitInfo.root, filepath.ToSlash(rel), targetIsDir, true
}

func (r *runner) runGitStdout(root string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", root}, args...)
	out, stderr, code, notFound, err := r.runCmd("", "git", fullArgs...)
	if notFound || err != nil {
		return "", fmt.Errorf("git %s failed: %v", strings.Join(args, " "), err)
	}
	if code != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(out)
		}
		if detail == "" {
			detail = fmt.Sprintf("exit %d", code)
		}
		return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), detail)
	}
	return out, nil
}

func extractPorcelainPaths(porcelainOutput string) []string {
	var paths []string
	for _, line := range splitLines(porcelainOutput) {
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		rawPath := line[3:]
		if strings.ContainsAny(xy, "RC") {
			if _, after, ok := strings.Cut(rawPath, " -> "); ok {
				rawPath = after
			}
		}
		if len(rawPath) >= 2 && rawPath[0] == '"' && rawPath[len(rawPath)-1] == '"' {
			rawPath = rawPath[1 : len(rawPath)-1]
		}
		paths = append(paths, rawPath)
	}
	return paths
}

func isNonEmptyRegularFile(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode().IsRegular() && st.Size() > 0
}

func (r *runner) resolveBaseRef(root string) string {
	for _, ref := range []string{"origin/main", "main", "origin/master", "master"} {
		_, _, code, notFound, err := r.runCmd("", "git", "-C", root, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
		if !notFound && err == nil && code == 0 {
			return ref
		}
	}
	return ""
}

func (r *runner) detectActiveADRs(root string) ([]string, error) {
	porcelain, err := r.runGitStdout(root,
		"--no-optional-locks",
		"-c", "core.quotePath=false",
		"status", "--porcelain", "-uall",
		"--", "docs/ADR",
	)
	if err != nil {
		return nil, err
	}

	found := make(map[string]struct{})
	deleted := make(map[string]struct{})

	addIfValid := func(rawPath string) {
		norm := filepath.ToSlash(strings.TrimSpace(rawPath))
		if _, isDel := deleted[norm]; isDel {
			return
		}
		m := adrFileRelRe.FindStringSubmatch(norm)
		if len(m) != 2 {
			return
		}
		if isNonEmptyRegularFile(filepath.Join(root, filepath.FromSlash(norm))) {
			found[m[1]] = struct{}{}
		}
	}

	// Source A (always): uncommitted worktree/staged ADRs under docs/ADR.
	for _, line := range splitLines(porcelain) {
		if len(line) < 4 {
			continue
		}
		xy := line[:2]
		rawPath := line[3:]
		if strings.ContainsAny(xy, "RC") {
			if _, after, ok := strings.Cut(rawPath, " -> "); ok {
				rawPath = after
			}
		}
		if len(rawPath) >= 2 && rawPath[0] == '"' && rawPath[len(rawPath)-1] == '"' {
			rawPath = rawPath[1 : len(rawPath)-1]
		}
		norm := filepath.ToSlash(rawPath)
		if xy == " D" || xy == "D " || xy == "DD" {
			deleted[norm] = struct{}{}
			if m := adrFileRelRe.FindStringSubmatch(norm); len(m) == 2 {
				delete(found, m[1])
			}
			continue
		}
		addIfValid(norm)
	}

	// Source B (feature branch only): ADRs committed on non-main/master branches.
	branchOut, _, branchCode, notFound, branchErr := r.runCmd("", "git", "-C", root, "symbolic-ref", "--quiet", "--short", "HEAD")
	if !notFound && branchErr == nil && branchCode == 0 {
		branch := strings.TrimSpace(branchOut)
		if branch != "" && branch != "main" && branch != "master" {
			if baseRef := r.resolveBaseRef(root); baseRef != "" {
				diffOut, _, diffCode, diffNotFound, diffErr := r.runCmd(
					"", "git", "-C", root,
					"--no-optional-locks",
					"-c", "core.quotePath=false",
					"diff", "--name-only", "--diff-filter=AMCR",
					baseRef+"...HEAD", "--", "docs/ADR",
				)
				if !diffNotFound && diffErr == nil && diffCode == 0 {
					for _, line := range splitLines(diffOut) {
						if strings.TrimSpace(line) != "" {
							addIfValid(line)
						}
					}
				}
			}
		}
	}

	ids := make([]string, 0, len(found))
	for id := range found {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (r *runner) computeFingerprint(root string, activeADRs []string) (string, error) {
	canonicalRoot := resolveRealPath(root)
	adrTree, err := r.runGitStdout(root, "rev-parse", "HEAD:docs/ADR")
	if err != nil {
		return "", err
	}
	cfgBlob, err := r.runGitStdout(root, "rev-parse", "HEAD:docdag.yaml")
	if err != nil {
		return "", err
	}
	porcelain, err := r.runGitStdout(root,
		"--no-optional-locks",
		"-c", "core.quotePath=false",
		"status", "--porcelain", "-uall",
		"--", "docs/ADR", "docdag.yaml",
	)
	if err != nil {
		return "", err
	}

	sortedActive := append([]string(nil), activeADRs...)
	sort.Strings(sortedActive)
	parts := []string{canonicalRoot, adrTree, cfgBlob, porcelain, "active:" + strings.Join(sortedActive, ",")}
	for _, relStr := range extractPorcelainPaths(porcelain) {
		target := filepath.Join(root, filepath.FromSlash(relStr))
		st, serr := os.Stat(target)
		if errors.Is(serr, os.ErrNotExist) {
			parts = append(parts, fmt.Sprintf("%s\tabsent", relStr))
			continue
		}
		if serr != nil {
			return "", serr
		}
		parts = append(parts, fmt.Sprintf("%s\t%d:%d", relStr, st.Size(), st.ModTime().UnixNano()))
	}

	h := sha256.New()
	for _, part := range parts {
		b := []byte(part)
		fmt.Fprintf(h, "%d:", len(b))
		_, _ = h.Write(b)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (r *runner) isGitIgnored(root, relPath string) bool {
	_, _, code, notFound, err := r.runCmd("", "git", "-C", root, "check-ignore", "-q", "--", relPath)
	return !notFound && err == nil && code == 0
}

func readPinnedDocdagVersion(root string) string {
	if root == "" {
		return "<DOCDAG_VERSION>"
	}
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(workflowRelPath)))
	if err != nil {
		return "<DOCDAG_VERSION>"
	}
	m := docdagVersionRe.FindSubmatch(raw)
	if len(m) < 2 {
		return "<DOCDAG_VERSION>"
	}
	return unquoteYAMLScalar(string(m[1]))
}

func formatMissingDocdagFix(root string) string {
	version := readPinnedDocdagVersion(root)
	return fmt.Sprintf(
		"FIX: install docdag with `go install github.com/Kaikei-e/DocDag/cmd/docdag@%s` (pinned by DOCDAG_VERSION in %s), then retry the edit.",
		version, workflowRelPath,
	)
}

func (r *runner) runDocdagValidate(root string) validateResult {
	out, stderr, code, notFound, err := r.runCmd(root, "docdag", "validate")
	if notFound {
		return validateResult{missingBinary: true, returncode: 127}
	}
	if err != nil && code == 0 {
		code = 1
		stderr = err.Error()
	}

	summary := "OK"
	for _, ln := range splitLines(out) {
		if t := strings.TrimSpace(ln); t != "" {
			summary = t
		}
	}

	var chunks []string
	if strings.TrimSpace(out) != "" {
		chunks = append(chunks, strings.TrimRight(out, "\n"))
	}
	if strings.TrimSpace(stderr) != "" {
		chunks = append(chunks, strings.TrimRight(stderr, "\n"))
	}
	return validateResult{
		missingBinary: false,
		returncode:    code,
		output:        strings.Join(chunks, "\n"),
		summaryLine:   summary,
	}
}

type docdagJSONRow struct {
	ID     string         `json:"id"`
	Title  string         `json:"title"`
	Path   string         `json:"path"`
	Fields map[string]any `json:"fields"`
}

func (r *runner) loadBindingADRs(root string) ([]bindingADR, error) {
	out, stderr, code, notFound, err := r.runCmd(root, "docdag", "query", "--binding", "--format", "json")
	if notFound {
		return nil, errMissingDocdag
	}
	if err != nil {
		return nil, fmt.Errorf("docdag query failed: %w", err)
	}
	if code != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = strings.TrimSpace(out)
		}
		return nil, fmt.Errorf("docdag query failed (exit %d): %s", code, detail)
	}

	var rows []docdagJSONRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, fmt.Errorf("docdag query returned malformed JSON: %w", err)
	}

	records := make([]bindingADR, 0, len(rows))
	for _, row := range rows {
		rawServices, hasKey := row.Fields["affected_services"]
		if row.Fields == nil || !hasKey {
			return nil, fmt.Errorf(
				"docdag query reported no affected_services for %s: docdag is older than v0.6.0, or docdag.yaml does not declare affected_services under fields:. %s",
				row.ID, formatMissingDocdagFix(root),
			)
		}
		var services []string
		switch sv := rawServices.(type) {
		case nil:
			services = nil
		case string:
			services = []string{sv}
		case []any:
			for _, item := range sv {
				if s, ok := item.(string); ok {
					services = append(services, s)
				}
			}
		}
		records = append(records, bindingADR{
			adrID:            row.ID,
			title:            row.Title,
			relPath:          row.Path,
			affectedServices: services,
		})
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].adrID > records[j].adrID
	})
	return records, nil
}

func governingADRsForScope(scope string, bindingADRs []bindingADR) []bindingADR {
	names := make(map[string]struct{})
	for _, n := range serviceNamesForScope(scope) {
		names[n] = struct{}{}
	}
	var out []bindingADR
	for _, adr := range bindingADRs {
		for _, svc := range adr.affectedServices {
			if _, ok := names[svc]; ok {
				out = append(out, adr)
				break
			}
		}
	}
	return out
}

func truncateTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxTitleChars {
		return title
	}
	return string(runes[:maxTitleChars-1]) + "…"
}

func serviceArgForSections(sections []scopeSection) string {
	if len(sections) == 1 {
		switch sections[0].scope {
		case ".github":
			return "ci-cd"
		case "repo-root":
			return "<name>"
		default:
			return sections[0].scope
		}
	}
	return "<name>"
}

func buildBriefListings(summaryLines []string, sections []scopeSection) string {
	listingLines := append([]string(nil), summaryLines...)
	listingLines = append(listingLines, "")

	for _, sec := range sections {
		total := len(sec.adrs)
		if total == 0 {
			listingLines = append(listingLines,
				fmt.Sprintf("Scope %s: 0 governing binding ADRs (inspect all binding ADRs with: docdag query --binding --fields id,title)", sec.scope),
			)
			continue
		}
		shown := total
		if shown > maxBriefADRs {
			shown = maxBriefADRs
		}
		if total > shown {
			listingLines = append(listingLines,
				fmt.Sprintf("Scope %s: %d governing binding ADRs (%d newest shown):", sec.scope, total, shown),
			)
		} else {
			noun := "ADRs"
			if total == 1 {
				noun = "ADR"
			}
			listingLines = append(listingLines,
				fmt.Sprintf("Scope %s: %d governing binding %s:", sec.scope, total, noun),
			)
		}
		for _, adr := range sec.adrs[:shown] {
			listingLines = append(listingLines, fmt.Sprintf("  %s  %s", adr.adrID, truncateTitle(adr.title)))
		}
	}
	return strings.Join(listingLines, "\n")
}

func buildBriefBody(summaryLines []string, sections []scopeSection) string {
	listings := buildBriefListings(summaryLines, sections)
	serviceArg := serviceArgForSections(sections)
	guidanceLines := []string{
		"Read one ADR: docdag context <id> --depth 2 --budget 1500",
		fmt.Sprintf("List all for a service: %s --service %s", selfServiceCmd, serviceArg),
		"If a planned change contradicts a binding ADR, stop and supersede it with a new ADR rather than contradicting it silently.",
	}
	return listings + "\n\n" + strings.Join(guidanceLines, "\n")
}

func buildHookADRRequiredBrief(summaries []string, sections []scopeSection) string {
	scopeNames := make([]string, 0, len(sections))
	for _, s := range sections {
		scopeNames = append(scopeNames, s.scope)
	}
	header := fmt.Sprintf("ADR PREFLIGHT BLOCKED (ADR required before code edits) — scope(s): %s", strings.Join(scopeNames, ", "))
	listings := buildBriefListings(summaries, sections)
	serviceArg := serviceArgForSections(sections)
	guidance := strings.Join([]string{
		"Required before editing production code (no receipt was recorded; blind retries stay blocked):",
		fmt.Sprintf("1. Inspect governing binding ADRs: docdag context <id> --depth 2 --budget 1500 (list all: %s --service %s)", selfServiceCmd, serviceArg),
		`2. Allocate next ADR ID: docdag new "<title>" --dry-run --format json`,
		"3. Draft docs/ADR/NNNNNN.md from docs/ADR/template.md with status: proposed (stop and ask user before superseding a binding ADR)",
		"4. Validate the ADR: docdag validate --touching docs/ADR/NNNNNN.md",
		"5. Retry this edit (auto-unblocks once valid ADR exists; only docs/, docdag.yaml, and test files are exempt)",
		"Guide: .agents/skills/alt-adr-writer/SKILL.md",
	}, "\n")

	candidate := fmt.Sprintf("%s\n%s\n\n%s\n", header, listings, guidance)
	if len(candidate) < maxBriefChars {
		return candidate
	}

	truncLine := "... (brief truncated; run the --service command per scope)"
	suffix := fmt.Sprintf("%s\n%s\n", truncLine, guidance)
	budget := maxBriefChars - len(header) - len(suffix) - 3
	if budget < 0 {
		budget = 0
	}
	cutListings := listings
	if len(cutListings) > budget {
		cutListings = cutListings[:budget]
		for len(cutListings) > 0 && !utf8.ValidString(cutListings) {
			cutListings = cutListings[:len(cutListings)-1]
		}
	}
	if idx := strings.LastIndex(cutListings, "\n"); idx >= 0 {
		cutListings = cutListings[:idx]
	}
	if cutListings == "" {
		return fmt.Sprintf("%s\n%s", header, suffix)
	}
	return fmt.Sprintf("%s\n%s\n%s", header, cutListings, suffix)
}

func rootKeyFor(root string) string {
	sum := sha256.Sum256([]byte(resolveRealPath(root)))
	return hex.EncodeToString(sum[:])[:16]
}

func (r *runner) receiptPathFor(sessionID, agentID, root, scope string) string {
	sessTok := sanitiseToken(sessionID, "default")
	agentRaw := agentID
	if agentRaw == "" {
		agentRaw = "main"
	}
	agentTok := sanitiseToken(agentRaw, "main")
	scopeTok := sanitiseToken(scope, "repo-root")
	return filepath.Join(r.getStateDir(), sessTok, agentTok, rootKeyFor(root), scopeTok)
}

func (r *runner) hasValidReceipt(sessionID, agentID, root, scope, expectedFP string) bool {
	path := r.receiptPathFor(sessionID, agentID, root, scope)
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(raw)) == expectedFP
}

func (r *runner) writeReceipt(sessionID, agentID, root, scope, fp string) error {
	path := r.receiptPathFor(sessionID, agentID, root, scope)
	if err := os.MkdirAll(filepath.Dir(path), fsperm.Dir); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(fp+"\n"), fsperm.File)
}

func (r *runner) cleanupSessionState(sessionID string) {
	target := filepath.Join(r.getStateDir(), sanitiseToken(sessionID, "default"))
	_ = os.RemoveAll(target)
}

func formatValidateBlocked(root string, vres validateResult) string {
	if vres.missingBinary {
		return fmt.Sprintf("ADR PREFLIGHT BLOCKED: docdag is not on PATH.\n%s\n", formatMissingDocdagFix(root))
	}
	lines := []string{
		fmt.Sprintf("ADR PREFLIGHT BLOCKED: docdag validate exited with code %d.", vres.returncode),
	}
	if vres.output != "" {
		lines = append(lines, capLines(vres.output, maxFindingLines))
	}
	if vres.returncode == 3 {
		lines = append(lines, fmt.Sprintf(
			"FIX: the configuration (docdag.yaml) or the docdag binary is stale; update docdag to DOCDAG_VERSION in %s or repair the configuration, re-run `docdag validate`, then retry the edit.",
			workflowRelPath,
		))
	} else {
		lines = append(lines, "FIX: repair the findings above, re-run `docdag validate`, then retry the edit.")
	}
	return strings.Join(lines, "\n") + "\n"
}

func stringField(payload map[string]any, keys ...string) string {
	for _, k := range keys {
		if s, ok := payload[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func (r *runner) writePreToolInternalError(err error) int {
	fmt.Fprintf(r.stderr,
		"ADR PREFLIGHT BLOCKED: internal error (%v).\nFIX: inspect ALT_ADR_PREFLIGHT_STATE_DIR or the preflight script; set ALT_ADR_PREFLIGHT=disabled as the documented bypass if needed.\n",
		err,
	)
	return 2
}

func containsString(list []string, target string) bool {
	for _, s := range list {
		if s == target {
			return true
		}
	}
	return false
}

func (r *runner) handlePreToolUse(payload map[string]any) int {
	if r.isDisabled() {
		return 0
	}

	rawPaths := extractHookPaths(payload)
	if len(rawPaths) == 0 {
		return 0
	}

	baseCwd := stringField(payload, "cwd")
	if baseCwd == "" {
		baseCwd = r.cwd
	}
	sessionID := stringField(payload, "session_id", "conversationId")
	if sessionID == "" {
		sessionID = "default"
	}
	agentID := stringField(payload, "agent_id")

	id, err := r.getScriptGitIdentity()
	if err != nil {
		return r.writePreToolInternalError(err)
	}

	gitDirCache := make(map[string]*gitIdentity)
	activeCache := make(map[string][]string)
	fpCache := make(map[string]string)

	prodBlockedByRoot := make(map[string][]string)
	var prodBlockedRoots []string

	prodAllowedByRoot := make(map[string][]string)
	var prodAllowedRoots []string

	testPendingRoots := make(map[string]bool)
	var testRootOrder []string

	for _, rawPath := range rawPaths {
		root, relPath, isDir, ok := r.resolveFileInRepo(rawPath, baseCwd, id.commonDir, gitDirCache)
		if !ok {
			continue
		}
		scope := deriveScopeFromRelpath(relPath, isDir)
		if scope == "" || isExemptRelpath(relPath, scope) {
			continue
		}
		if r.isGitIgnored(root, relPath) {
			continue
		}

		activeADRs, ok := activeCache[root]
		if !ok {
			activeADRs, err = r.detectActiveADRs(root)
			if err != nil {
				return r.writePreToolInternalError(err)
			}
			activeCache[root] = activeADRs
		}
		fp, ok := fpCache[root]
		if !ok {
			fp, err = r.computeFingerprint(root, activeADRs)
			if err != nil {
				return r.writePreToolInternalError(err)
			}
			fpCache[root] = fp
		}

		if isTestRelpath(relPath) {
			if r.hasValidReceipt(sessionID, agentID, root, testReceiptScope, fp) {
				continue
			}
			if !testPendingRoots[root] {
				testPendingRoots[root] = true
				testRootOrder = append(testRootOrder, root)
			}
			continue
		}

		if len(activeADRs) == 0 {
			if _, exists := prodBlockedByRoot[root]; !exists {
				prodBlockedRoots = append(prodBlockedRoots, root)
			}
			if !containsString(prodBlockedByRoot[root], scope) {
				prodBlockedByRoot[root] = append(prodBlockedByRoot[root], scope)
			}
			continue
		}

		if r.hasValidReceipt(sessionID, agentID, root, scope, fp) {
			continue
		}
		if _, exists := prodAllowedByRoot[root]; !exists {
			prodAllowedRoots = append(prodAllowedRoots, root)
		}
		if !containsString(prodAllowedByRoot[root], scope) {
			prodAllowedByRoot[root] = append(prodAllowedByRoot[root], scope)
		}
	}

	// If any production root lacks an active ADR, fail-closed without writing any receipt.
	if len(prodBlockedRoots) > 0 {
		var summaries []string
		var sections []scopeSection
		for _, root := range prodBlockedRoots {
			vres := r.runDocdagValidate(root)
			if vres.missingBinary || vres.returncode != 0 {
				fmt.Fprint(r.stderr, formatValidateBlocked(root, vres))
				return 2
			}
			summaries = append(summaries, vres.summaryLine)
			bindingADRs, err := r.loadBindingADRs(root)
			if err != nil {
				return r.writePreToolInternalError(err)
			}
			for _, scope := range prodBlockedByRoot[root] {
				sections = append(sections, scopeSection{
					scope: scope,
					adrs:  governingADRsForScope(scope, bindingADRs),
				})
			}
		}
		fmt.Fprint(r.stderr, buildHookADRRequiredBrief(summaries, sections))
		return 2
	}

	if len(prodAllowedRoots) == 0 && len(testRootOrder) == 0 {
		return 0
	}

	validated := make(map[string]bool)
	var rootsToValidate []string
	for _, root := range prodAllowedRoots {
		if !validated[root] {
			validated[root] = true
			rootsToValidate = append(rootsToValidate, root)
		}
	}
	for _, root := range testRootOrder {
		if !validated[root] {
			validated[root] = true
			rootsToValidate = append(rootsToValidate, root)
		}
	}

	for _, root := range rootsToValidate {
		vres := r.runDocdagValidate(root)
		if vres.missingBinary || vres.returncode != 0 {
			fmt.Fprint(r.stderr, formatValidateBlocked(root, vres))
			return 2
		}
	}

	for _, root := range prodAllowedRoots {
		fp := fpCache[root]
		for _, scope := range prodAllowedByRoot[root] {
			if err := r.writeReceipt(sessionID, agentID, root, scope, fp); err != nil {
				return r.writePreToolInternalError(err)
			}
		}
	}
	for _, root := range testRootOrder {
		fp := fpCache[root]
		if err := r.writeReceipt(sessionID, agentID, root, testReceiptScope, fp); err != nil {
			return r.writePreToolInternalError(err)
		}
	}

	return 0
}

func (r *runner) handleSessionStart(payload map[string]any) int {
	if r.isDisabled() {
		fmt.Fprintln(r.stdout, "adr_preflight_disabled: ADR preflight gate is off (ALT_ADR_PREFLIGHT=disabled).")
		return 0
	}

	sessionID := stringField(payload, "session_id", "conversationId")
	if sessionID == "" {
		sessionID = "default"
	}
	r.cleanupSessionState(sessionID)

	id, err := r.getScriptGitIdentity()
	if err != nil {
		fmt.Fprintf(r.stdout, "adr_preflight_enabled: internal error (%v); code edits will be blocked until it is fixed.\n", err)
		return 0
	}

	vres := r.runDocdagValidate(id.root)
	if vres.missingBinary {
		fmt.Fprintf(r.stdout,
			"adr_preflight_enabled: docdag is not on PATH; code edits will be blocked until it is installed and fixed.\n%s\n",
			formatMissingDocdagFix(id.root),
		)
		return 0
	}

	if vres.returncode != 0 {
		findings := capLines(vres.output, maxFindingLines)
		msg := "adr_preflight_enabled: docdag validate failed; code edits will be blocked until it is fixed."
		if findings != "" {
			fmt.Fprintf(r.stdout, "%s\n%s\n", msg, findings)
		} else {
			fmt.Fprintln(r.stdout, msg)
		}
		return 0
	}

	fmt.Fprintf(r.stdout,
		"adr_preflight_enabled: %s. ADR-first gate active: production edits require a valid new/updated docs/ADR/NNNNNN.md (status: proposed before code -> accepted after tests pass; tests and docs/ exempt). Check binding ADRs via docdag context <id> (or --section Consequences), draft via docdag new \"<title>\" --dry-run --format json + docs/ADR/template.md, then implement.\n",
		vres.summaryLine,
	)
	return 0
}

func (r *runner) runHookMode() int {
	raw, err := io.ReadAll(r.stdin)
	if err != nil {
		return 0
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return 0
	}
	switch stringField(payload, "hook_event_name", "hookEventName") {
	case "PreToolUse":
		return r.handlePreToolUse(payload)
	case "SessionStart":
		return r.handleSessionStart(payload)
	default:
		return 0
	}
}

func (r *runner) collectChangedPaths(root string) ([]string, error) {
	out, stderr, code, notFound, err := r.runCmd("", "git", "-c", "core.quotePath=false", "-C", root, "status", "--porcelain", "-uall")
	if notFound || err != nil {
		return nil, fmt.Errorf("git status failed: %v", err)
	}
	if code != 0 {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = "git status failed"
		}
		return nil, errors.New(detail)
	}
	var paths []string
	for _, rel := range extractPorcelainPaths(out) {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
	}
	return paths, nil
}

func (r *runner) resolveActiveCheckoutRoot() (string, error) {
	id, err := r.getScriptGitIdentity()
	if err != nil {
		return "", err
	}
	if r.cwd != "" {
		if cwdInfo, ok := r.resolveGitDir(r.cwd); ok && cwdInfo.commonDir == id.commonDir {
			return cwdInfo.root, nil
		}
	}
	return id.root, nil
}

func (r *runner) runCLIPathsMode(rawPaths []string, fallbackRoot string) int {
	id, err := r.getScriptGitIdentity()
	if err != nil {
		fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
		return 1
	}

	gitDirCache := make(map[string]*gitIdentity)
	scopesByRoot := make(map[string][]string)
	var rootOrder []string

	for _, rawPath := range rawPaths {
		root, relPath, isDir, ok := r.resolveFileInRepo(rawPath, r.cwd, id.commonDir, gitDirCache)
		if !ok {
			continue
		}
		scope := deriveScopeFromRelpath(relPath, isDir)
		if scope == "" || isExemptRelpath(relPath, scope) {
			continue
		}
		if r.isGitIgnored(root, relPath) {
			continue
		}
		if _, exists := scopesByRoot[root]; !exists {
			rootOrder = append(rootOrder, root)
		}
		if !containsString(scopesByRoot[root], scope) {
			scopesByRoot[root] = append(scopesByRoot[root], scope)
		}
	}

	rootsToValidate := rootOrder
	if len(rootsToValidate) == 0 {
		rootsToValidate = []string{fallbackRoot}
	}

	var summaries []string
	var sections []scopeSection

	for _, root := range rootsToValidate {
		vres := r.runDocdagValidate(root)
		if vres.missingBinary {
			fmt.Fprintf(r.stdout, "docdag is not on PATH.\n%s\n", formatMissingDocdagFix(root))
			return 1
		}
		if vres.returncode != 0 {
			if vres.output != "" {
				fmt.Fprintln(r.stdout, vres.output)
			}
			return 1
		}
		summaries = append(summaries, vres.summaryLine)
		if scopes, exists := scopesByRoot[root]; exists {
			bindingADRs, err := r.loadBindingADRs(root)
			if err != nil {
				fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
				return 1
			}
			for _, scope := range scopes {
				sections = append(sections, scopeSection{
					scope: scope,
					adrs:  governingADRsForScope(scope, bindingADRs),
				})
			}
		}
	}

	if len(sections) == 0 {
		fmt.Fprintln(r.stdout, "No non-exempt scopes in the requested paths.")
		return 0
	}

	fmt.Fprintln(r.stdout, buildBriefBody(summaries, sections))
	return 0
}

func (r *runner) runCLIServiceMode(serviceName string) int {
	root, err := r.resolveActiveCheckoutRoot()
	if err != nil {
		fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
		return 2
	}
	bindingADRs, err := r.loadBindingADRs(root)
	if err != nil {
		if errors.Is(err, errMissingDocdag) {
			fmt.Fprintf(r.stderr, "alt-harness preflight: docdag is not on PATH\n%s\n", formatMissingDocdagFix(root))
			return 2
		}
		fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
		return 2
	}
	for _, adr := range bindingADRs {
		if containsString(adr.affectedServices, serviceName) {
			fmt.Fprintf(r.stdout, "%s\t%s\t%s\n", adr.adrID, adr.title, adr.relPath)
		}
	}
	return 0
}

func (r *runner) printUsage() int {
	fmt.Fprintln(r.stderr, "Usage: alt-harness preflight <path>... | --changed | --service <name> | --hook")
	return 2
}

func (r *runner) main(args []string) int {
	if len(args) == 0 {
		return r.printUsage()
	}

	first := args[0]
	switch first {
	case "--hook":
		if len(args) != 1 {
			return r.printUsage()
		}
		return r.runHookMode()
	case "--changed":
		if len(args) != 1 {
			return r.printUsage()
		}
		root, err := r.resolveActiveCheckoutRoot()
		if err != nil {
			fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
			return 1
		}
		changed, err := r.collectChangedPaths(root)
		if err != nil {
			fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
			return 1
		}
		return r.runCLIPathsMode(changed, root)
	case "--service":
		if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
			return r.printUsage()
		}
		return r.runCLIServiceMode(strings.TrimSpace(args[1]))
	}

	var pathArgs []string
	if first == "--" {
		pathArgs = args[1:]
		if len(pathArgs) == 0 {
			return r.printUsage()
		}
	} else {
		for _, a := range args {
			if strings.HasPrefix(a, "-") {
				return r.printUsage()
			}
		}
		pathArgs = args
	}

	fallbackRoot, err := r.resolveActiveCheckoutRoot()
	if err != nil {
		fmt.Fprintf(r.stderr, "alt-harness preflight: %v\n", err)
		return 1
	}
	return r.runCLIPathsMode(pathArgs, fallbackRoot)
}
