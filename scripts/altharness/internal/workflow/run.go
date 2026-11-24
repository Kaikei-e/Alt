package workflow

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Run checks .github/workflows in the working directory.
// The Python checker takes no flags; extra arguments are ignored.
// An empty workflow set fails closed.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = args
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "::error::%s\n", err.Error())
		return 1
	}
	files, err := workflowFiles()
	bad := 0
	if err != nil {
		fmt.Fprintf(stderr, "::error::cannot read .github/workflows: %s\n", err.Error())
		bad++
	}
	if len(files) == 0 {
		fmt.Fprintln(stderr, "::error::no workflow files found under .github/workflows; refusing to pass vacuously.")
		bad++
	}
	scripts := loadFrontendScripts()
	var runners []string
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			fmt.Fprintf(stderr, "::error::%s\n", err.Error())
			return 1
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "::error file=%s::failed to read workflow: %s\n", path, err.Error())
			bad++
			continue
		}
		text := string(raw)
		textErrors, textBad := checkWorkflowTextDetails(path, text, false)
		for _, msg := range textErrors {
			fmt.Fprintln(stderr, msg)
		}
		bad += textBad
		doc, err := parseYAML(text)
		if err != nil {
			fmt.Fprintf(stderr, "::error file=%s::failed to parse YAML: %s\n", path, err.Error())
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
					runners = append(runners, path+"::"+step.job)
				}
			}
		}
		errors, fileBad := checkWorkflowDocDetails(path, m)
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

func workflowFiles() ([]string, error) {
	dir := ".github/workflows"
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
		path := dir + "/" + name
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}
