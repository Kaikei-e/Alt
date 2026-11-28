package guards

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"alt/scripts/altharness/internal/ratchet"
)

const (
	lintChangedGolangciVersion = "v2.14.0"
	lintChangedUsage           = "usage: alt-harness lint-changed [--root <dir>] [--hook]"
)

// RunLintChanged executes fast, changed-file static linting (golangci-lint / ruff / biome)
// scoped strictly to dirty working-tree files.
func RunLintChanged(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return RunLintChangedWithStdin(ctx, args, nil, stdout, stderr)
}

// RunLintChangedWithStdin executes `alt-harness lint-changed`.
// In --hook mode (Claude Code / Codex Stop hook), exit 0 allows stopping and
// exit 2 blocks stopping with the lint diagnostics on stderr.
func RunLintChangedWithStdin(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}

	hookMode := false
	rootArg := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			_, _ = fmt.Fprintln(stdout, lintChangedUsage)
			return 0
		case "--hook":
			hookMode = true
		case "--root":
			if i+1 >= len(args) {
				_, _ = fmt.Fprintln(stderr, lintChangedUsage)
				return 2
			}
			i++
			rootArg = args[i]
		default:
			_, _ = fmt.Fprintf(stderr, "unknown flag %q\n%s\n", args[i], lintChangedUsage)
			return 2
		}
	}

	if hookMode && stdin != nil {
		if raw, err := io.ReadAll(stdin); err == nil && len(bytes.TrimSpace(raw)) > 0 {
			var payload map[string]any
			if json.Unmarshal(raw, &payload) == nil {
				if active, _ := payload["stop_hook_active"].(bool); active {
					return 0
				}
				if rootArg == "" {
					if cwd, ok := payload["cwd"].(string); ok && strings.TrimSpace(cwd) != "" {
						rootArg = strings.TrimSpace(cwd)
					}
				}
			}
		}
	}

	repoRoot := resolveLintChangedRoot(rootArg)
	if repoRoot == "" {
		return 0
	}

	code, output := runChangedLint(ctx, repoRoot)
	if code == 0 {
		return 0
	}
	msg := strings.TrimSpace(output)
	if msg == "" {
		msg = "changed-file lint gate failed"
	}
	_, _ = fmt.Fprintln(stderr, msg)
	if hookMode {
		return 2
	}
	return code
}

func resolveLintChangedRoot(explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		if abs, err := filepath.Abs(explicit); err == nil {
			if top := gitTopLevel(abs); top != "" {
				return top
			}
			return abs
		}
		return explicit
	}
	if envRoot := strings.TrimSpace(os.Getenv("ALT_HARNESS_REPO_ROOT")); envRoot != "" {
		return envRoot
	}
	if wd, err := os.Getwd(); err == nil {
		if top := gitTopLevel(wd); top != "" {
			return top
		}
		return wd
	}
	return ""
}

func gitTopLevel(dir string) string {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	out, err := exec.Command(gitBin, "-C", dir, "--no-optional-locks", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func defaultAgyRunChangedLint(ctx context.Context, repoRoot string) (int, string) {
	if strings.TrimSpace(repoRoot) == "" {
		return 0, ""
	}
	return runChangedLint(ctx, repoRoot)
}

func runChangedLint(ctx context.Context, repoRoot string) (int, string) {
	scan, err := listDirtyFiles(ctx, repoRoot)
	if err != nil || len(scan.allPaths) == 0 {
		return 0, ""
	}

	goPkgsByMod := map[string]map[string]struct{}{}
	pyFilesByMod := map[string][]string{}
	tsFilesByMod := map[string][]string{}

	for _, rel := range scan.existingFiles {
		norm := filepath.ToSlash(rel)
		if isIgnoredForChangedLint(norm) {
			continue
		}
		absPath := filepath.Join(repoRoot, filepath.FromSlash(norm))
		info, statErr := os.Stat(absPath)
		if statErr != nil || info.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(absPath))
		switch ext {
		case ".go":
			if isGeneratedGoPath(norm) {
				continue
			}
			modRoot := findNearestProjectRoot(absPath, repoRoot, "go.mod")
			if modRoot == "" {
				continue
			}
			pkgRel, relErr := filepath.Rel(modRoot, filepath.Dir(absPath))
			if relErr != nil {
				continue
			}
			pkgSpec := "./" + filepath.ToSlash(pkgRel)
			if pkgRel == "." {
				pkgSpec = "."
			}
			if goPkgsByMod[modRoot] == nil {
				goPkgsByMod[modRoot] = map[string]struct{}{}
			}
			goPkgsByMod[modRoot][pkgSpec] = struct{}{}
		case ".py":
			pyRoot := findNearestProjectRoot(absPath, repoRoot, "pyproject.toml")
			if pyRoot == "" {
				continue
			}
			if relPy, relErr := filepath.Rel(pyRoot, absPath); relErr == nil {
				pyFilesByMod[pyRoot] = append(pyFilesByMod[pyRoot], filepath.ToSlash(relPy))
			}
		case ".ts", ".tsx", ".svelte":
			if isGeneratedTSPath(norm) {
				continue
			}
			tsRoot := findNearestProjectRoot(absPath, repoRoot, "package.json")
			if tsRoot == "" {
				continue
			}
			if relTS, relErr := filepath.Rel(tsRoot, absPath); relErr == nil {
				tsFilesByMod[tsRoot] = append(tsFilesByMod[tsRoot], filepath.ToSlash(relTS))
			}
		}
	}

	var failures []string

	// 1. Go changed packages via golangci-lint with repo .golangci.yml.
	for _, modRoot := range sortedKeys(goPkgsByMod) {
		pkgs := make([]string, 0, len(goPkgsByMod[modRoot]))
		for pkg := range goPkgsByMod[modRoot] {
			pkgs = append(pkgs, pkg)
		}
		sort.Strings(pkgs)
		if out, failed := lintChangedGoPackages(ctx, repoRoot, modRoot, pkgs); failed {
			failures = append(failures, out)
		}
	}

	// 2. Python changed files via ruff check + ruff format --check.
	for _, pyRoot := range sortedSliceMapKeys(pyFilesByMod) {
		files := uniqueSortedStrings(pyFilesByMod[pyRoot])
		if out, failed := lintChangedPythonFiles(ctx, repoRoot, pyRoot, files); failed {
			failures = append(failures, out)
		}
	}

	// 3. TypeScript / Svelte changed files via biome check (only in biome-configured projects).
	for _, tsRoot := range sortedSliceMapKeys(tsFilesByMod) {
		files := uniqueSortedStrings(tsFilesByMod[tsRoot])
		if out, failed := lintChangedTSFiles(ctx, repoRoot, tsRoot, files); failed {
			failures = append(failures, out)
		}
	}

	// 4. CHECK-02 inline suppression ratchet (catches increases AND un-updated decreases
	// including suppressions removed by deleting source files).
	if needsRatchetCheck(repoRoot, scan.allPaths) {
		var rOut, rErr bytes.Buffer
		if rc := ratchet.Run(ctx, []string{"--repo-root", repoRoot}, &rOut, &rErr); rc != 0 {
			msg := strings.TrimSpace(rErr.String())
			if msg == "" {
				msg = strings.TrimSpace(rOut.String())
			}
			if msg == "" {
				msg = fmt.Sprintf("suppression ratchet exited with code %d", rc)
			}
			failures = append(failures, "[ratchet] "+msg)
		}
	}

	if len(failures) == 0 {
		return 0, ""
	}
	return 1, "Changed-file lint gate failed (fix violations before stopping):\n\n" + strings.Join(failures, "\n\n")
}

type dirtyScan struct {
	existingFiles []string
	allPaths      []string
}

func listDirtyFiles(ctx context.Context, repoRoot string) (dirtyScan, error) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return dirtyScan{}, err
	}
	cmd := exec.CommandContext(ctx, gitBin, "-C", repoRoot, "--no-optional-locks", "status", "--porcelain", "-z")
	out, err := cmd.Output()
	if err != nil {
		return dirtyScan{}, err
	}
	entries := bytes.Split(out, []byte{0})
	var scan dirtyScan
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if len(entry) < 4 {
			continue
		}
		xy := string(entry[:2])
		path := string(entry[3:])
		// Rename/copy entries consume an extra NUL-separated path (original path).
		origPath := ""
		if strings.ContainsAny(xy, "RC") && i+1 < len(entries) {
			i++
			origPath = string(entries[i])
		}
		if strings.TrimSpace(origPath) != "" {
			scan.allPaths = append(scan.allPaths, origPath)
		}
		if strings.TrimSpace(path) != "" {
			scan.allPaths = append(scan.allPaths, path)
			if xy[0] != 'D' && xy[1] != 'D' {
				scan.existingFiles = append(scan.existingFiles, path)
			}
		}
	}
	return scan, nil
}

func needsRatchetCheck(repoRoot string, allPaths []string) bool {
	supPath := filepath.Join(repoRoot, "scripts", "lint", "ratchet", "suppressions.toml")
	st, err := os.Stat(supPath)
	if err != nil || st.IsDir() {
		return false
	}
	for _, rel := range allPaths {
		norm := filepath.ToSlash(rel)
		if norm == "scripts/lint/ratchet/suppressions.toml" {
			return true
		}
		if isIgnoredForChangedLint(norm) {
			continue
		}
		switch strings.ToLower(filepath.Ext(norm)) {
		case ".py", ".go", ".rs", ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs", ".svelte":
			return true
		}
	}
	return false
}

func isIgnoredForChangedLint(norm string) bool {
	return norm == "stopped-using-k8s" ||
		strings.HasPrefix(norm, "stopped-using-k8s/") ||
		strings.HasPrefix(norm, "scripts/harness/corpus/")
}

func isGeneratedGoPath(norm string) bool {
	return strings.HasSuffix(norm, ".pb.go") ||
		strings.HasSuffix(norm, ".connect.go") ||
		strings.Contains(norm, "/gen/") ||
		strings.HasPrefix(norm, "gen/")
}

func isGeneratedTSPath(norm string) bool {
	return strings.HasSuffix(norm, "_pb.ts") ||
		strings.Contains(norm, "/lib/gen/")
}

// findNearestProjectRoot walks up from filePath's directory up to repoRoot
// looking for a directory containing markerFile (e.g. "go.mod", "pyproject.toml", "package.json").
func findNearestProjectRoot(filePath, repoRoot, markerFile string) string {
	dir := filepath.Dir(filePath)
	cleanRoot := ""
	if strings.TrimSpace(repoRoot) != "" {
		if absRoot, err := filepath.Abs(repoRoot); err == nil {
			cleanRoot = filepath.Clean(absRoot)
		}
	}
	if absDir, err := filepath.Abs(dir); err == nil {
		dir = filepath.Clean(absDir)
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, markerFile)); err == nil && !st.IsDir() {
			return dir
		}
		if cleanRoot != "" && dir == cleanRoot {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func resolveGolangciLintBin(ctx context.Context, repoRoot string) string {
	if goBin, err := exec.LookPath("go"); err == nil {
		cmd := exec.CommandContext(ctx, goBin, "env", "GOVERSION")
		cmd.Dir = repoRoot
		if out, gerr := cmd.Output(); gerr == nil {
			goVer := strings.TrimSpace(string(out))
			if goVer != "" {
				if base := lintCacheBase(); base != "" {
					cached := filepath.Join(base, "alt-lint", goVer, "golangci-lint-"+lintChangedGolangciVersion, "golangci-lint")
					if st, serr := os.Stat(cached); serr == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
						return cached
					}
				}
			}
		}
	}
	if pathBin, err := exec.LookPath("golangci-lint"); err == nil {
		return pathBin
	}
	return ""
}

func lintCacheBase() string {
	if xdg := strings.TrimSpace(os.Getenv("XDG_CACHE_HOME")); xdg != "" {
		return xdg
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cache")
	}
	return ""
}

func lintChangedGoPackages(ctx context.Context, repoRoot, modRoot string, pkgs []string) (string, bool) {
	relMod, err := filepath.Rel(repoRoot, modRoot)
	if err != nil {
		relMod = modRoot
	}
	relSlash := filepath.ToSlash(relMod)

	// scripts/altharness is excluded from root .golangci.yml (see internal/lint/run.go:331);
	// verify it with go vet plus unused/ineffassign checks so CLI exec helpers stay clean.
	if relSlash == "scripts/altharness" || strings.HasPrefix(relSlash, "scripts/altharness/") {
		if goBin, gerr := exec.LookPath("go"); gerr == nil {
			vetArgs := append([]string{"vet"}, pkgs...)
			vetCmd := exec.CommandContext(ctx, goBin, vetArgs...)
			vetCmd.Dir = modRoot
			vetCmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
			if out, vetErr := vetCmd.CombinedOutput(); vetErr != nil {
				return fmt.Sprintf("[%s] go vet failed on %s:\n%s", relSlash, strings.Join(pkgs, " "), strings.TrimSpace(string(out))), true
			}
		}
		lintBin := resolveGolangciLintBin(ctx, repoRoot)
		if lintBin == "" {
			return "", false
		}
		args := append([]string{"run", "--no-config", "--default=none", "--enable=unused,ineffassign", "--new-from-rev=HEAD"}, pkgs...)
		cmd := exec.CommandContext(ctx, lintBin, args...)
		cmd.Dir = modRoot
		cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
		if out, runErr := cmd.CombinedOutput(); runErr != nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed == "" {
				trimmed = runErr.Error()
			}
			return fmt.Sprintf("[%s] golangci-lint (unused/ineffassign) failed on %s:\n%s", relSlash, strings.Join(pkgs, " "), trimmed), true
		}
		return "", false
	}

	lintBin := resolveGolangciLintBin(ctx, repoRoot)
	if lintBin == "" {
		return fmt.Sprintf("[%s] golangci-lint is not available on PATH or cache; install golangci-lint %s to verify changed Go packages", relSlash, lintChangedGolangciVersion), true
	}
	args := []string{"run"}
	cfg := filepath.Join(repoRoot, ".golangci.yml")
	if st, serr := os.Stat(cfg); serr == nil && !st.IsDir() {
		args = append(args, "--config", cfg)
	}
	args = append(args, pkgs...)
	cmd := exec.CommandContext(ctx, lintBin, args...)
	cmd.Dir = modRoot
	cmd.Env = append(os.Environ(), "GOFLAGS=-buildvcs=false")
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return "", false
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		trimmed = runErr.Error()
	}
	return fmt.Sprintf("[%s] golangci-lint failed on %s:\n%s", relSlash, strings.Join(pkgs, " "), trimmed), true
}

func lintChangedPythonFiles(ctx context.Context, repoRoot, pyRoot string, files []string) (string, bool) {
	relMod, err := filepath.Rel(repoRoot, pyRoot)
	if err != nil {
		relMod = pyRoot
	}
	var baseArgs []string
	var bin string
	if uvBin, uerr := exec.LookPath("uv"); uerr == nil {
		if _, venvErr := os.Stat(filepath.Join(pyRoot, ".venv")); venvErr == nil {
			bin = uvBin
			baseArgs = []string{"run", "--no-sync", "ruff"}
		}
	}
	if bin == "" {
		if ruffBin, rerr := exec.LookPath("ruff"); rerr == nil {
			bin = ruffBin
		}
	}
	if bin == "" {
		return "", false
	}

	checkArgs := append(append([]string{}, baseArgs...), "check")
	checkArgs = append(checkArgs, files...)
	cmd := exec.CommandContext(ctx, bin, checkArgs...)
	cmd.Dir = pyRoot
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Sprintf("[%s] ruff check failed:\n%s", filepath.ToSlash(relMod), strings.TrimSpace(string(out))), true
	}

	fmtArgs := append(append([]string{}, baseArgs...), "format", "--check")
	fmtArgs = append(fmtArgs, files...)
	cmdFmt := exec.CommandContext(ctx, bin, fmtArgs...)
	cmdFmt.Dir = pyRoot
	if out, runErr := cmdFmt.CombinedOutput(); runErr != nil {
		return fmt.Sprintf("[%s] ruff format --check failed:\n%s", filepath.ToSlash(relMod), strings.TrimSpace(string(out))), true
	}
	return "", false
}

func lintChangedTSFiles(ctx context.Context, repoRoot, tsRoot string, files []string) (string, bool) {
	hasBiomeCfg := false
	for _, name := range []string{"biome.json", "biome.jsonc"} {
		if st, err := os.Stat(filepath.Join(tsRoot, name)); err == nil && !st.IsDir() {
			hasBiomeCfg = true
			break
		}
	}
	if !hasBiomeCfg {
		return "", false
	}
	localBiome := filepath.Join(tsRoot, "node_modules", ".bin", "biome")
	if st, err := os.Stat(localBiome); err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
		return "", false
	}
	relMod, err := filepath.Rel(repoRoot, tsRoot)
	if err != nil {
		relMod = tsRoot
	}
	args := append([]string{"lint"}, files...)
	cmd := exec.CommandContext(ctx, localBiome, args...)
	cmd.Dir = tsRoot
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Sprintf("[%s] biome lint failed:\n%s", filepath.ToSlash(relMod), strings.TrimSpace(string(out))), true
	}
	return "", false
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedSliceMapKeys(m map[string][]string) []string {
	return sortedKeys(m)
}

func uniqueSortedStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
