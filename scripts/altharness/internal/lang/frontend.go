package lang

import (
	"alt/scripts/altharness/internal/fsperm"
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// RunFrontend is scripts/lint/frontend.sh. The Bun pin comes from the project
// Dockerfile. A missing matching binary is installed into
// $XDG_CACHE_HOME/alt-lint/bun/<version>/bun (HOME/.cache when XDG_CACHE_HOME
// is unset), which is .../bun/1.3.12/bun for the current frontend pin.
// The biome canary runs before format, lint, and check.
func RunFrontend(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	if len(args) != 1 || args[0] == "" {
		fmt.Fprintln(stderr, "usage: frontend.sh <project-dir>")
		return 2
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	rel := strings.TrimSuffix(strings.TrimPrefix(args[0], "./"), "/")
	project := filepath.Join(root, rel)
	if st, err := os.Stat(project); err != nil || !st.IsDir() {
		fmt.Fprintf(stderr, "error: %s is not a frontend project directory (missing package.json)\n", rel)
		return 2
	}
	if _, err := os.Stat(filepath.Join(project, "package.json")); err != nil {
		fmt.Fprintf(stderr, "error: %s is not a frontend project directory (missing package.json)\n", rel)
		return 2
	}
	if _, err := os.Stat(filepath.Join(project, "node_modules")); err != nil {
		fmt.Fprintf(stderr, "error: %s/node_modules is missing; run `bun install --frozen-lockfile` in %s\n", rel, rel)
		return 1
	}
	if _, err := exec.LookPath("python3"); err != nil {
		fmt.Fprintln(stderr, "error: required command not found in PATH: python3")
		return 1
	}
	dockerfile := filepath.Join(project, "Dockerfile")
	expected, err := bunPinFromDockerfile(dockerfile)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s/Dockerfile is missing (source of truth for Bun version pin)\n", rel)
		if !os.IsNotExist(err) {
			fmt.Fprintf(stderr, "error: could not parse a single Bun version pin from %s/Dockerfile (expected 'FROM oven/bun:<version>')\n", rel)
		}
		return 1
	}
	if expected == "" {
		fmt.Fprintf(stderr, "error: could not parse a single Bun version pin from %s/Dockerfile (expected 'FROM oven/bun:<version>')\n", rel)
		return 1
	}
	bin, rc := resolveBun(ctx, stdout, stderr, rel, expected)
	if rc != 0 {
		return rc
	}
	actual := bunVersion(ctx, bin)
	if actual != expected {
		shown := actual
		if shown == "" {
			shown = "unknown"
		}
		fmt.Fprintf(stderr, "error: bun version mismatch: expected %s, got '%s'\n", expected, shown)
		return 1
	}
	if _, err := os.Stat(filepath.Join(project, "bun.lock")); err != nil {
		fmt.Fprintf(stderr, "error: %s/bun.lock is missing\n", rel)
		return 1
	}
	if err := checkLockedTools(project, "@biomejs/biome", "svelte-check"); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	canary := filepath.Join(root, "scripts", "lint", "canary_audit.py")
	if _, err := os.Stat(canary); err == nil {
		fmt.Fprintf(stdout, "==> [%s] canary audit (biome)\n", rel)
		if code, err := run(ctx, stdout, stderr, project, nil, "python3", canary, "frontend-canary", "--project-dir", project); err != nil || code != 0 {
			return 1
		}
	} else if _, err := os.Stat(filepath.Join(root, "scripts", "lint", "run.sh")); err == nil {
		fmt.Fprintf(stderr, "error: missing canary audit helper: %s\n", canary)
		return 1
	}
	policy := os.Getenv("LINT_POLICY")
	if policy == "" {
		policy = "fix-existing"
	}
	fmtRC, lintRC, checkRC := 0, 0, 0
	lintStatus := "pass"
	fmtStatus := "pass"
	pathEnv := []string{"PATH=" + filepath.Dir(bin) + string(os.PathListSeparator) + os.Getenv("PATH")}
	if policy == "block-new" {
		biome := filepath.Join(project, "node_modules", ".bin", "biome")
		if code, err := run(ctx, stdout, stderr, project, pathEnv, biome, "format", "."); err != nil || code != 0 {
			fmtStatus = "fail"
			fmtRC = code
			if err != nil {
				fmtRC = 127
			}
		}
		tmp, err := os.CreateTemp("", "alt-biome-lint-*.json")
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return 1
		}
		jsonPath := tmp.Name()
		defer os.Remove(jsonPath)
		code, runErr := run(ctx, tmp, stderr, project, pathEnv, biome, "check", "--error-on-warnings", "--reporter=json", ".")
		tmp.Close()
		if runErr != nil {
			lintRC = 127
			lintStatus = "fail"
		} else {
			lintRC = code
			switch {
			case code == 0:
				lintStatus = "pass"
			case fmtRC != 0:
				lintStatus = "fail"
			default:
				fp, fpErr := run(ctx, stdout, stderr, project, nil, "python3", filepath.Join(root, "scripts", "lint", "check_ratchet.py"), "check-fingerprints",
					"--repo-root", root, "--project-dir", project, "--biome-json", jsonPath, "--require-findings")
				if fpErr == nil && fp == 0 {
					lintRC = 0
					lintStatus = "pass (block-new)"
				} else {
					lintStatus = "fail"
				}
			}
		}
	} else if code, err := run(ctx, stdout, stderr, project, pathEnv, bin, "run", "lint:check"); err != nil || code != 0 {
		lintStatus = "fail"
		lintRC = code
		if err != nil {
			lintRC = 127
		}
	}
	checkStatus := "pass"
	if code, err := run(ctx, stdout, stderr, project, pathEnv, bin, "run", "check"); err != nil || code != 0 {
		checkStatus = "fail"
		checkRC = code
		if err != nil {
			checkRC = 127
		}
	}
	if policy == "block-new" {
		fmt.Fprintf(stdout, "format: %s (exit %d)\n", fmtStatus, fmtRC)
	}
	fmt.Fprintf(stdout, "lint:check: %s (exit %d)\n", lintStatus, lintRC)
	fmt.Fprintf(stdout, "check: %s (exit %d)\n", checkStatus, checkRC)
	if fmtRC != 0 || lintRC != 0 || checkRC != 0 {
		return 1
	}
	return 0
}

var bunFromRe = regexp.MustCompile(`(?m)^FROM[ \t\r\f\v]+oven/bun:([0-9][0-9.]*)(?:-[^ \t\r\f\v]+)?`)

func bunPinFromDockerfile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	found := map[string]struct{}{}
	for _, m := range bunFromRe.FindAllSubmatch(data, -1) {
		found[string(m[1])] = struct{}{}
	}
	if len(found) != 1 {
		return "", nil
	}
	for ver := range found {
		return ver, nil
	}
	return "", nil
}

// pinnedBunCacheFile is $XDG_CACHE_HOME/alt-lint/bun/<version>/bun.
func pinnedBunCacheFile(version string) (string, error) {
	base, err := cacheHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "alt-lint", "bun", version, "bun"), nil
}

func resolveBun(ctx context.Context, stdout, stderr io.Writer, rel, version string) (string, int) {
	if override := os.Getenv("ALT_LINT_BUN_BIN"); override != "" {
		st, err := os.Stat(override)
		if err != nil || st.Mode()&0o111 == 0 {
			fmt.Fprintf(stderr, "error: ALT_LINT_BUN_BIN is not executable: %s\n", override)
			return "", 1
		}
		got := bunVersion(ctx, override)
		if got != version {
			if got == "" {
				got = "unknown"
			}
			fmt.Fprintf(stderr, "error: ALT_LINT_BUN_BIN (%s) bun version mismatch: expected %s, got '%s'\n", override, version, got)
			return "", 1
		}
		return override, 0
	}
	if path, err := exec.LookPath("bun"); err == nil && bunVersion(ctx, path) == version {
		return path, 0
	}
	cached, err := pinnedBunCacheFile(version)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	if st, err := os.Stat(cached); err == nil && st.Mode()&0o111 != 0 && bunVersion(ctx, cached) == version {
		return cached, 0
	}
	target, ok := bunZipTarget(runtime.GOOS, runtime.GOARCH)
	if !ok {
		fmt.Fprintf(stderr, "error: unsupported platform for automatic Bun %s install: %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		return "", 1
	}
	if _, err := exec.LookPath("curl"); err != nil {
		fmt.Fprintf(stderr, "error: required command not found in PATH for Bun %s download: curl\n", version)
		return "", 1
	}
	baseURL := strings.TrimRight(os.Getenv("ALT_LINT_BUN_DOWNLOAD_BASE_URL"), "/")
	if baseURL == "" {
		baseURL = "https://github.com/oven-sh/bun/releases/download"
	}
	url := fmt.Sprintf("%s/bun-v%s/%s.zip", baseURL, version, target)
	tmp, err := os.MkdirTemp("", "alt-bun-install-")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	defer os.RemoveAll(tmp)
	zipPath := filepath.Join(tmp, target+".zip")
	fmt.Fprintf(stderr, "==> [%s] installing pinned bun %s into %s\n", rel, version, filepath.Dir(cached))
	if code, err := run(ctx, io.Discard, stderr, tmp, nil, "curl", "-fsSL", "--retry", "3", url, "-o", zipPath); err != nil || code != 0 {
		fmt.Fprintf(stderr, "error: failed to download bun %s from %s\n", version, url)
		return "", 1
	}
	extract := filepath.Join(tmp, "extract")
	if err := unzip(zipPath, extract); err != nil {
		fmt.Fprintf(stderr, "error: failed to extract bun archive downloaded from %s\n", url)
		return "", 1
	}
	extracted := filepath.Join(extract, target, "bun")
	if _, err := os.Stat(extracted); err != nil {
		found := findFile(extract, "bun", 2)
		if found == "" {
			fmt.Fprintf(stderr, "error: bun binary not found in extracted archive %s\n", url)
			return "", 1
		}
		extracted = found
	}
	if err := os.MkdirAll(filepath.Dir(cached), fsperm.Dir); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	staged, err := os.CreateTemp(filepath.Dir(cached), "bun.tmp.*")
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	stagedPath := staged.Name()
	staged.Close()
	if err := copyFile(extracted, stagedPath); err != nil {
		_ = os.Remove(stagedPath)
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	if want := strings.ToLower(strings.Join(strings.Fields(os.Getenv("ALT_LINT_BUN_SHA256")), "")); want != "" {
		if !sha256Re.MatchString(want) {
			_ = os.Remove(stagedPath)
			fmt.Fprintf(stderr, "error: invalid ALT_LINT_BUN_SHA256 (expected 64 hex chars): '%s'\n", os.Getenv("ALT_LINT_BUN_SHA256"))
			return "", 1
		}
		sum, err := fileSHA256(stagedPath)
		if err != nil || sum != want {
			_ = os.Remove(stagedPath)
			fmt.Fprintf(stderr, "error: downloaded bun SHA-256 mismatch: expected %s, got %s\n", want, sum)
			return "", 1
		}
	}
	if err := os.Chmod(stagedPath, fsperm.Dir); err != nil {
		_ = os.Remove(stagedPath)
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	got := bunVersion(ctx, stagedPath)
	if got != version {
		_ = os.Remove(stagedPath)
		if got == "" {
			got = "unknown"
		}
		fmt.Fprintf(stderr, "error: downloaded bun version mismatch: expected %s, got '%s'\n", version, got)
		return "", 1
	}
	if err := os.Rename(stagedPath, cached); err != nil {
		_ = os.Remove(stagedPath)
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return "", 1
	}
	return cached, 0
}

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

func bunZipTarget(goos, goarch string) (string, bool) {
	switch goos + "/" + goarch {
	case "linux/amd64":
		return "bun-linux-x64", true
	case "linux/arm64":
		return "bun-linux-aarch64", true
	case "darwin/arm64":
		return "bun-darwin-aarch64", true
	case "darwin/amd64":
		return "bun-darwin-x64", true
	default:
		return "", false
	}
}

func bunVersion(ctx context.Context, bin string) string {
	out, _, code, err := capture(ctx, "", nil, bin, "--version")
	if err != nil || code != 0 {
		return ""
	}
	return strings.Join(strings.Fields(out), "")
}

func checkLockedTools(project string, pkgs ...string) error {
	lock, err := os.ReadFile(filepath.Join(project, "bun.lock"))
	if err != nil {
		return fmt.Errorf("error: %s", err.Error())
	}
	var errs []string
	for _, pkg := range pkgs {
		re := regexp.MustCompile(`(?m)^\s*"` + regexp.QuoteMeta(pkg) + `":\s*\["` + regexp.QuoteMeta(pkg) + `@([^"]+)"`)
		found := map[string]struct{}{}
		for _, m := range re.FindAllSubmatch(lock, -1) {
			found[string(m[1])] = struct{}{}
		}
		if len(found) != 1 {
			vers := make([]string, 0, len(found))
			for ver := range found {
				vers = append(vers, ver)
			}
			sort.Strings(vers)
			errs = append(errs, fmt.Sprintf("error: expected exactly 1 locked version for %s in %s, found %s", pkg, filepath.Join(project, "bun.lock"), pyList(vers)))
			continue
		}
		var locked string
		for ver := range found {
			locked = ver
		}
		pkgJSON := filepath.Join(project, "node_modules", pkg, "package.json")
		data, err := os.ReadFile(pkgJSON)
		if err != nil {
			errs = append(errs, fmt.Sprintf("error: %s is missing; run `bun install --frozen-lockfile`", pkgJSON))
			continue
		}
		var doc struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(data, &doc); err != nil || doc.Version != locked {
			errs = append(errs, fmt.Sprintf("error: %s version mismatch: bun.lock pins %s, node_modules has %s", pkg, locked, doc.Version))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	return nil
}

func pyList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = "'" + strings.ReplaceAll(item, "'", `\'`) + "'"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func unzip(src, dest string) error {
	reader, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, file := range reader.File {
		target := filepath.Join(dest, file.Name)
		if !strings.HasPrefix(target, dest+string(os.PathSeparator)) && target != dest {
			return fmt.Errorf("zip entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(target, fsperm.Dir); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), fsperm.Dir); err != nil {
			return err
		}
		in, err := file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, file.Mode())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		in.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func findFile(root, name string, maxDepth int) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(os.PathSeparator)) + 1
		}
		if d.IsDir() && depth > maxDepth {
			return filepath.SkipDir
		}
		if !d.IsDir() && d.Name() == name && depth <= maxDepth {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
