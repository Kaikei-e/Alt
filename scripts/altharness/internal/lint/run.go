// Package lint is the ./scripts/lint/run.sh entrypoint: policy, disk floor,
// target discovery, and dispatch. Named gates repo/harness/both run in process
// on the worker pool. Other targets still exec scripts/lint/<kind>.sh until
// that language runner is ported.
package lint

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"alt/scripts/altharness/internal/lang"
	"alt/scripts/altharness/internal/meta"
)

// Run matches scripts/lint/run.sh.
func Run(ctx context.Context, repo string, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}
	policy := os.Getenv("LINT_POLICY")
	if policy == "" {
		policy = "fix-existing"
	}
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--policy":
			if i+1 >= len(args) || args[i+1] == "" {
				fmt.Fprintln(stderr, "usage: run.sh [--policy <fix-existing|block-new>] <target|--list|all>")
				return 2
			}
			i++
			policy = args[i]
		case strings.HasPrefix(a, "--policy="):
			policy = strings.TrimPrefix(a, "--policy=")
		default:
			rest = append(rest, a)
		}
	}
	switch policy {
	case "fix-existing", "block-new":
	default:
		fmt.Fprintf(stderr, "invalid lint policy '%s': expected 'fix-existing' or 'block-new'\n", policy)
		return 2
	}
	if err := os.Setenv("LINT_POLICY", policy); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if len(rest) != 1 || rest[0] == "" {
		fmt.Fprintln(stderr, "usage: run.sh [--policy <fix-existing|block-new>] <target|--list|all>")
		return 2
	}
	if rest[0] == "--list" {
		targets, err := listTargets(repo)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
		for _, t := range targets {
			fmt.Fprintln(stdout, t)
		}
		return 0
	}
	if code := checkDisk(repo, stderr); code != 0 {
		return code
	}
	if rest[0] == "all" {
		return runAll(ctx, repo, stdout, stderr)
	}
	target := strings.TrimPrefix(rest[0], "./")
	target = strings.TrimSuffix(target, "/")
	switch target {
	case "", "/", "..":
		fmt.Fprintf(stderr, "invalid lint target: %s\n", rest[0])
		return 2
	}
	if strings.HasPrefix(target, "/") || target == ".." || strings.HasPrefix(target, "../") || strings.Contains(target, "/../") || strings.HasSuffix(target, "/..") {
		fmt.Fprintf(stderr, "invalid lint target: %s\n", rest[0])
		return 2
	}
	switch target {
	case "run", "go", "python", "rust", "deno", "frontend", "check-link-vs-url-naming":
		fmt.Fprintf(stderr, "invalid lint target '%s': must be a project directory or named target (adr, apparmor, compose, e2e, harness, observability, proto, registry, repo)\n", target)
		return 2
	}
	if target == "repo" || target == "harness" || target == "both" {
		steps, err := meta.Catalog(ctx, repo, target)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 2
		}
		code, err := meta.Run(ctx, steps, meta.Options{Repo: repo, Stdout: stdout, Stderr: stderr, Timeout: 15 * time.Minute})
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return 1
		}
		return code
	}
	kind := ""
	if !strings.Contains(target, "/") {
		script := filepath.Join(repo, "scripts", "lint", target+".sh")
		if st, err := os.Stat(script); err == nil && !st.IsDir() {
			if st, err := os.Stat(filepath.Join(repo, target)); err == nil && st.IsDir() {
				if mk := markerKind(filepath.Join(repo, target)); mk != "" && mk != target {
					fmt.Fprintf(stderr, "named lint target '%s' shadows directory %s/ (%s)\n", target, target, mk)
					return 2
				}
			}
			kind = target
		}
	}
	if kind == "" {
		dir := filepath.Join(repo, target)
		st, err := os.Stat(dir)
		if err != nil || !st.IsDir() {
			fmt.Fprintf(stderr, "lint target directory not found: %s\n", target)
			return 2
		}
		kind = markerKind(dir)
		if kind == "" {
			fmt.Fprintf(stderr, "no recognized lint marker in %s\n", target)
			return 2
		}
	}
	if code, ok := runKind(ctx, kind, target, stdout, stderr); ok {
		return code
	}
	script := filepath.Join(repo, "scripts", "lint", kind+".sh")
	if _, err := os.Stat(script); err != nil {
		fmt.Fprintf(stderr, "missing lint script: scripts/lint/%s.sh\n", kind)
		return 2
	}
	cmd := exec.CommandContext(ctx, "bash", script, target)
	cmd.Dir = repo
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	return 0
}

func runKind(ctx context.Context, kind, target string, stdout, stderr io.Writer) (int, bool) {
	args := []string{target}
	switch kind {
	case "go":
		return lang.RunGo(ctx, args, stdout, stderr), true
	case "python":
		return lang.RunPython(ctx, args, stdout, stderr), true
	case "rust":
		return lang.RunRust(ctx, args, stdout, stderr), true
	case "deno":
		return lang.RunDeno(ctx, args, stdout, stderr), true
	case "frontend":
		return lang.RunFrontend(ctx, args, stdout, stderr), true
	case "adr":
		return lang.RunAdr(ctx, args, stdout, stderr), true
	case "proto":
		return lang.RunProto(ctx, args, stdout, stderr), true
	case "compose":
		return lang.RunCompose(ctx, args, stdout, stderr), true
	case "observability":
		return lang.RunObservability(ctx, args, stdout, stderr), true
	case "apparmor":
		return lang.RunApparmor(ctx, args, stdout, stderr), true
	case "e2e":
		return lang.RunE2E(ctx, args, stdout, stderr), true
	case "registry":
		return lang.RunRegistry(ctx, args, stdout, stderr), true
	default:
		return 0, false
	}
}

func runAll(ctx context.Context, repo string, stdout, stderr io.Writer) int {
	targets, err := listTargets(repo)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if len(targets) == 0 {
		fmt.Fprintln(stderr, "no lint targets found")
		return 1
	}
	pass, fail := 0, 0
	statuses := make([]string, len(targets))
	for i, t := range targets {
		fmt.Fprintf(stdout, "==> [all] running: %s\n", t)
		code := Run(ctx, repo, []string{t}, stdout, stderr)
		if code == 0 {
			statuses[i] = "PASS"
			pass++
		} else {
			statuses[i] = "FAIL"
			fail++
		}
	}
	fmt.Fprintf(stdout, "\n==> [all] summary (%d passed, %d failed, %d total)\n", pass, fail, len(targets))
	for i, t := range targets {
		fmt.Fprintf(stdout, "  %-4s  %s\n", statuses[i], t)
	}
	if fail > 0 {
		return 1
	}
	return 0
}

func checkDisk(repo string, stderr io.Writer) int {
	minKB := int64(10485760)
	if raw := os.Getenv("ALT_LINT_MIN_FREE_KB"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to determine free disk space (available='%s', required='%s')\n", os.Getenv("ALT_LINT_FAKE_FREE_KB"), raw)
			return 1
		}
		minKB = n
	}
	var avail int64
	if raw := os.Getenv("ALT_LINT_FAKE_FREE_KB"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to determine free disk space (available='%s', required='%d')\n", raw, minKB)
			return 1
		}
		avail = n
	} else {
		var st syscall.Statfs_t
		if err := syscall.Statfs(repo, &st); err != nil {
			fmt.Fprintf(stderr, "ERROR: failed to determine free disk space (available='', required='%d')\n", minKB)
			return 1
		}
		avail = int64(st.Bavail) * int64(st.Bsize) / 1024
	}
	if avail < minKB {
		fmt.Fprintf(stderr, "ERROR: ディスク空き容量不足 (available: %d MB, required: %d MB)\n", avail/1024, minKB/1024)
		return 1
	}
	return 0
}

func listTargets(repo string) ([]string, error) {
	seen := map[string]struct{}{}
	var out []string
	add := func(rel string) {
		if rel == "" || rel == "." {
			return
		}
		if _, ok := seen[rel]; ok {
			return
		}
		seen[rel] = struct{}{}
		out = append(out, rel)
	}
	markers, err := markerFiles(repo)
	if err != nil {
		return nil, err
	}
	for _, rel := range markers {
		dir := filepath.Dir(rel)
		if dir == "." {
			continue
		}
		switch {
		case dir == "stopped-using-k8s" || strings.HasPrefix(dir, "stopped-using-k8s/"):
			continue
		case dir == "scripts/harness/corpus" || strings.HasPrefix(dir, "scripts/harness/corpus/"):
			continue
		case dir == "scripts/altharness" || strings.HasPrefix(dir, "scripts/altharness/"):
			continue
		}
		kind := markerKind(filepath.Join(repo, dir))
		if kind == "" {
			continue
		}
		if kind == "go" && generatedGoStub(repo, dir) {
			continue
		}
		if !strings.Contains(dir, "/") {
			if st, err := os.Stat(filepath.Join(repo, "scripts", "lint", dir+".sh")); err == nil && !st.IsDir() && kind != dir {
				continue
			}
		}
		add(dir)
	}
	entries, _ := os.ReadDir(filepath.Join(repo, "scripts", "lint"))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sh") {
			continue
		}
		name = strings.TrimSuffix(name, ".sh")
		switch name {
		case "run", "all", "go", "python", "rust", "deno", "frontend", "check-link-vs-url-naming":
			continue
		}
		if st, err := os.Stat(filepath.Join(repo, name)); err == nil && st.IsDir() {
			if kind := markerKind(filepath.Join(repo, name)); kind != "" && kind != name {
				continue
			}
		}
		add(name)
	}
	sort.Strings(out)
	return out, nil
}

func markerFiles(repo string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(repo, ".git")); err == nil {
		cmd := exec.Command("git", "-C", repo, "ls-files", "--cached", "--others", "--exclude-standard")
		out, err := cmd.Output()
		if err == nil {
			var files []string
			sc := bufio.NewScanner(strings.NewReader(string(out)))
			for sc.Scan() {
				line := sc.Text()
				base := filepath.Base(line)
				switch base {
				case "go.mod", "Cargo.toml", "pyproject.toml", "deno.json", "bun.lock", "buf.yaml":
					files = append(files, line)
				}
			}
			return files, nil
		}
	}
	var files []string
	skip := map[string]struct{}{".git": {}, "node_modules": {}, ".venv": {}, "target": {}, "__pycache__": {}, ".cache": {}, "vendor": {}, "_reports": {}}
	err := filepath.WalkDir(repo, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == repo || !os.IsNotExist(walkErr) {
				return walkErr
			}
			return nil
		}
		if d.IsDir() {
			if _, ok := skip[d.Name()]; ok && path != repo {
				return filepath.SkipDir
			}
			return nil
		}
		switch d.Name() {
		case "go.mod", "Cargo.toml", "pyproject.toml", "deno.json", "bun.lock", "buf.yaml":
			rel, _ := filepath.Rel(repo, path)
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

func markerKind(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "go"
	}
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "rust"
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return "python"
	}
	if _, err := os.Stat(filepath.Join(dir, "deno.json")); err == nil {
		return "deno"
	}
	if _, err := os.Stat(filepath.Join(dir, "bun.lock")); err == nil {
		return "frontend"
	}
	if _, err := os.Stat(filepath.Join(dir, "buf.yaml")); err == nil {
		return "proto"
	}
	return ""
}

func generatedGoStub(repo, rel string) bool {
	switch rel {
	case "alt-butterfly-facade/internal/gen/proto", "rag-orchestrator/internal/gen/proto":
		return true
	}
	dir := filepath.Join(repo, rel)
	anyGo, nonGen := false, false
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".go") {
			anyGo = true
			if !strings.HasSuffix(d.Name(), ".pb.go") && !strings.HasSuffix(d.Name(), ".connect.go") {
				nonGen = true
			}
		}
		return nil
	})
	if err != nil {
		return false
	}
	return anyGo && !nonGen
}
