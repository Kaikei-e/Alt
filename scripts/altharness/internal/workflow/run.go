package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Run checks .github/workflows in --repo-root (default: current directory).
// Extra positional arguments are ignored for parity with the legacy CLI.
// An empty workflow set fails closed.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		ctx = context.Background()
	}
	repoRoot := "."
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--repo-root":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "::error::--repo-root requires a value")
				return 1
			}
			i++
			repoRoot = args[i]
		case strings.HasPrefix(arg, "--repo-root="):
			repoRoot = strings.TrimPrefix(arg, "--repo-root=")
		}
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "::error::%s\n", err.Error())
		return 1
	}
	files, err := workflowFilesIn(repoRoot)
	bad := 0
	if err != nil {
		fmt.Fprintf(stderr, "::error::cannot read .github/workflows: %s\n", err.Error())
		bad++
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "::error::no workflow files found under .github/workflows; refusing to pass vacuously.")
		bad++
	}
	scripts := loadFrontendScriptsIn(repoRoot)
	var runners []string
	for _, relPath := range files {
		if err := ctx.Err(); err != nil {
			fmt.Fprintf(stderr, "::error::%s\n", err.Error())
			return 1
		}
		fullPath := joinRepo(repoRoot, relPath)
		raw, err := os.ReadFile(fullPath)
		if err != nil {
			fmt.Fprintf(stderr, "::error file=%s::failed to read workflow: %s\n", relPath, err.Error())
			bad++
			continue
		}
		text := string(raw)
		textErrors, textBad := checkWorkflowTextDetails(relPath, text, false)
		for _, msg := range textErrors {
			fmt.Fprintln(stderr, msg)
		}
		bad += textBad
		doc, err := parseYAML(text)
		if err != nil {
			fmt.Fprintf(stderr, "::error file=%s::failed to parse YAML: %s\n", relPath, err.Error())
			bad++
			continue
		}
		m, ok := doc.(ymap)
		if !ok {
			continue
		}
		names := triggerNames(m)
		_, push := names["push"]
		_, pr := names["pull_request"]
		if scripts != nil && (push || pr) {
			for _, step := range iterRunSteps(m) {
				if !step.blocking {
					continue
				}
				resolved := expandRunScripts(step.command, scripts, 0)
				if collectsClientProject(resolved, step.env) {
					runners = append(runners, relPath+"::"+step.job)
				}
			}
		}
		errors, fileBad := checkWorkflowDocDetailsIn(relPath, m, repoRoot)
		for _, msg := range errors {
			fmt.Fprintln(stderr, msg)
		}
		bad += fileBad
	}
	if scripts == nil {
		fmt.Fprintf(stderr, "::error::cannot read %s, so the check that alt-frontend-sv's %s vitest project is actually run cannot be evaluated. If the frontend moved or was removed, update this script — do not let the check pass vacuously.\n", frontendPkgJSON, pyReprString(clientProject))
		bad++
	} else if len(runners) == 0 {
		fmt.Fprintf(stderr, "::error::no push/pull_request-triggered job runs alt-frontend-sv's %s vitest project. Those are the src/**/*.svelte.{test,spec}.ts component specs; without a job that sets %s=true and runs vitest without a --project filter or with --project=%s, they are collected zero times while CI still reports green. Restore the job (see alt-frontend-sv-unit-test.yaml) rather than deleting this check. `continue-on-error` and `if: false` steps deliberately do not count.\n", pyReprString(clientProject), browserEnv, clientProject)
		bad++
	}
	if bad > 0 {
		return 1
	}
	fmt.Fprintln(stdout, "OK: no forbidden triggers, GitHub-hosted labels only, permissions declared, checkouts credential-free, no untrusted context in run bodies, every action SHA-pinned, path filters resolve (rule 8) and match own and called reusable workflows (rule 11), every container/service image digest-pinned, job timeouts declared, and swallowed failures carry reasons (rule 12).")
	sort.Strings(runners)
	fmt.Fprintf(stdout, "OK: client vitest project is run by %s.\n", strings.Join(runners, ", "))
	return 0
}

func joinRepo(repoRoot, rel string) string {
	if repoRoot == "" || repoRoot == "." {
		return filepath.FromSlash(rel)
	}
	return filepath.Join(repoRoot, filepath.FromSlash(rel))
}

func workflowFiles() ([]string, error) {
	return workflowFilesIn(".")
}

func workflowFilesIn(repoRoot string) ([]string, error) {
	dirRel := ".github/workflows"
	dir := joinRepo(repoRoot, dirRel)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []string
	for _, ent := range entries {
		name := ent.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		full := filepath.Join(dir, name)
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, dirRel+"/"+name)
	}
	sort.Strings(files)
	return files, nil
}
