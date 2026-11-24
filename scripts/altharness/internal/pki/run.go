// Package pki generates and drift-checks the six Python infra/pki packages.
// It follows scripts/codegen/pki_py/sync_python_pki.py, including --check.
package pki

import (
	"alt/scripts/altharness/internal/fsperm"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	exitOK    = 0
	exitDrift = 1
	exitUsage = 2
)

const usageText = `usage: sync_python_pki.py [-h] [--check] [--repo-root REPO_ROOT]

Sync or drift-check canonical Python PKI modules across 6 services.
`

// Run is the CLI for sync_python_pki.py.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = ctx
	check := false
	repoFlag := ""
	sawRepo := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			fmt.Fprint(stdout, usageText)
			return exitOK
		case arg == "--check":
			check = true
		case arg == "--repo-root":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "usage: sync_python_pki.py [-h] [--check] [--repo-root REPO_ROOT]\nsync_python_pki.py: error: argument --repo-root: expected one argument\n")
				return exitUsage
			}
			i++
			repoFlag = args[i]
			sawRepo = true
		case strings.HasPrefix(arg, "--repo-root="):
			repoFlag = strings.TrimPrefix(arg, "--repo-root=")
			sawRepo = true
		default:
			fmt.Fprintf(stderr, "usage: sync_python_pki.py [-h] [--check] [--repo-root REPO_ROOT]\nsync_python_pki.py: error: unrecognized arguments: %s\n", arg)
			return exitUsage
		}
	}

	templates, err := findTemplates(repoFlag)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitDrift
	}
	repo := ""
	if sawRepo {
		repo, err = filepath.Abs(repoFlag)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return exitDrift
		}
	} else {
		repo = defaultRepo(templates)
	}

	if check {
		issues, err := checkRepo(repo, templates)
		if err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return exitDrift
		}
		if len(issues) > 0 {
			fmt.Fprint(stderr, "Python PKI drift detected:\n")
			for _, issue := range issues {
				fmt.Fprintf(stderr, "  - %s\n", issue)
			}
			fmt.Fprint(stderr, "Run `python3 scripts/codegen/pki_py/sync_python_pki.py` to regenerate.\n")
			return exitDrift
		}
		fmt.Fprintf(stdout, "OK: all %d Python services match canonical PKI templates.\n", len(services))
		return exitOK
	}

	written, err := syncRepo(repo, templates)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitDrift
	}
	fmt.Fprintf(stdout, "Synced Python PKI modules across %d services (%d file(s) updated).\n", len(services), written)
	return exitOK
}

func defaultRepo(templates string) string {
	return filepath.Clean(filepath.Join(templates, "..", "..", "..", ".."))
}

func findTemplates(repoFlag string) (string, error) {
	var starts []string
	if cwd, err := os.Getwd(); err == nil {
		starts = append(starts, cwd)
	}
	if repoFlag != "" {
		if abs, err := filepath.Abs(repoFlag); err == nil {
			starts = append(starts, abs)
		}
	}
	for _, start := range starts {
		dir := start
		for {
			cand := filepath.Join(dir, "scripts", "codegen", "pki_py", "templates")
			if st, err := os.Stat(filepath.Join(cand, "shared", "ops.py.tmpl")); err == nil && !st.IsDir() {
				return cand, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("cannot find scripts/codegen/pki_py/templates (PKI codegen templates)")
}

func moduleNames(spec serviceSpec) []string {
	names := append([]string{}, sharedModules...)
	names = append(names, flavorModules[spec.flavor]...)
	sort.Strings(names)
	return names
}

func templatePath(templates, flavor, filename string) string {
	for _, shared := range sharedModules {
		if shared == filename {
			return filepath.Join(templates, "shared", filename+".tmpl")
		}
	}
	return filepath.Join(templates, flavor, filename+".tmpl")
}

func renderService(templates string, spec serviceSpec) (map[string]string, error) {
	rendered := make(map[string]string)
	for _, filename := range moduleNames(spec) {
		raw, err := os.ReadFile(templatePath(templates, spec.flavor, filename))
		if err != nil {
			return nil, fmt.Errorf("read template %s: %w", filename, err)
		}
		content := string(raw)
		content = strings.ReplaceAll(content, "{{PKG_PREFIX}}", spec.pkgPrefix)
		content = strings.ReplaceAll(content, "{{TLS_GATE_CALL}}", tlsGateCalls[spec.tlsGate])
		content = strings.ReplaceAll(content, "{{TLS_GATE_HELPERS}}", tlsGateHelpers[spec.tlsGate])
		if spec.inboundTLSModule != "" {
			content = strings.ReplaceAll(content, "{{INBOUND_TLS_MODULE}}", spec.inboundTLSModule)
		}
		rendered[filename] = generatedHeader + content
	}
	return rendered, nil
}

func checkRepo(repo, templates string) ([]string, error) {
	var issues []string
	for _, spec := range services {
		target := filepath.Join(repo, filepath.FromSlash(spec.pkiDir))
		expected, err := renderService(templates, spec)
		if err != nil {
			return nil, err
		}
		st, err := os.Stat(target)
		if err != nil || !st.IsDir() {
			issues = append(issues, "MISSING DIR: "+spec.pkiDir)
			continue
		}
		entries, err := os.ReadDir(target)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", spec.pkiDir, err)
		}
		actual := map[string]struct{}{}
		for _, ent := range entries {
			if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".py") {
				continue
			}
			actual[ent.Name()] = struct{}{}
		}
		var extras []string
		for name := range actual {
			if _, ok := expected[name]; !ok {
				extras = append(extras, name)
			}
		}
		sort.Strings(extras)
		for _, extra := range extras {
			issues = append(issues, fmt.Sprintf("UNEXPECTED FILE: %s/%s", spec.pkiDir, extra))
		}
		for _, filename := range moduleNames(spec) {
			want := expected[filename]
			rel := spec.pkiDir + "/" + filename
			dest := filepath.Join(target, filename)
			info, err := os.Lstat(dest)
			if err != nil || info.IsDir() {
				issues = append(issues, "MISSING FILE: "+rel)
				continue
			}
			got, err := os.ReadFile(dest)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", rel, err)
			}
			if string(got) != want {
				issues = append(issues, "DRIFT: "+rel)
			}
		}
	}
	return issues, nil
}

func syncRepo(repo, templates string) (int, error) {
	written := 0
	for _, spec := range services {
		target := filepath.Join(repo, filepath.FromSlash(spec.pkiDir))
		if err := os.MkdirAll(target, fsperm.Dir); err != nil {
			return written, fmt.Errorf("mkdir %s: %w", spec.pkiDir, err)
		}
		expected, err := renderService(templates, spec)
		if err != nil {
			return written, err
		}
		for _, filename := range moduleNames(spec) {
			want := expected[filename]
			dest := filepath.Join(target, filename)
			write := true
			if got, err := os.ReadFile(dest); err == nil && string(got) == want {
				write = false
			}
			if write {
				if err := os.WriteFile(dest, []byte(want), fsperm.File); err != nil {
					return written, fmt.Errorf("write %s/%s: %w", spec.pkiDir, filename, err)
				}
				written++
			}
		}
	}
	return written, nil
}
