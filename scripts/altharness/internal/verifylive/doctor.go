package verifylive

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"alt/scripts/altharness/internal/execx"
	"alt/scripts/altharness/internal/fsperm"
)

var (
	markdownLinkRE     = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	requiredFeatureSec = []string{
		"## Sub-features",
		"## How to get to it",
		"## Driving it with the harness",
		"## Gotchas",
	}
)

func effectiveEnv(key string, overrides map[string]string) string {
	if overrides != nil {
		if val, ok := overrides[key]; ok {
			return val
		}
	}
	return os.Getenv(key)
}

func isProductionProject(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "alt")
}

// DoctorOptions controls which runtime checks RunDoctorWithOptions executes.
type DoctorOptions struct {
	HygieneOnly      bool
	RequireSSEPython bool
}

// RunDoctor executes fail-closed isolation, toolchain, skill, and feature-map hygiene checks.
func RunDoctor(ctx context.Context, repoRoot, evidenceDir, projectOverride string, envOverrides map[string]string) DoctorReport {
	return RunDoctorWithOptions(ctx, repoRoot, evidenceDir, projectOverride, envOverrides, DoctorOptions{
		RequireSSEPython: true,
	})
}

// RunDoctorWithOptions executes fail-closed isolation, skill, feature-map, and
// optional live-probe toolchain/writability diagnostics.
func RunDoctorWithOptions(ctx context.Context, repoRoot, evidenceDir, projectOverride string, envOverrides map[string]string, opts DoctorOptions) DoctorReport {
	report := DoctorReport{
		OK:            true,
		IsolationMode: "loopback-127.0.0.1",
		RepoRoot:      repoRoot,
		EvidenceDir:   evidenceDir,
	}

	add := func(res CheckResult) {
		if !res.OK {
			report.OK = false
		}
		report.Checks = append(report.Checks, res)
	}

	// 1. Production isolation check (-p alt / COMPOSE_PROJECT_NAME=alt MUST be rejected).
	add(checkIsolation(projectOverride, envOverrides))

	// 2. Go toolchain.
	if goPath, err := exec.LookPath("go"); err != nil {
		add(CheckResult{Name: "toolchain-go", OK: false, Detail: "go binary not found in PATH"})
	} else {
		add(CheckResult{Name: "toolchain-go", OK: true, Detail: goPath})
	}

	if !opts.HygieneOnly {
		// 3. Python runtime for live SSE probe (fail-closed when sse probe is requested).
		if opts.RequireSSEPython {
			pyCmd, pyDetail, pyErr := resolvePythonForSSE(ctx, repoRoot)
			if pyErr != nil {
				add(CheckResult{Name: "toolchain-python", OK: false, Detail: pyErr.Error()})
			} else {
				add(CheckResult{Name: "toolchain-python", OK: true, Detail: fmt.Sprintf("%s (%s)", strings.Join(pyCmd, " "), pyDetail)})
			}
		}

		// 4. Writable evidence directory (with symlink rejection).
		add(checkEvidenceDirWritable(evidenceDir))
	}

	// 5. Skill files and .agents/skills symlinks.
	add(checkSkillFilesAndSymlinks(repoRoot))

	// 6. Feature Map index hygiene (zero orphans, zero broken links, 4 required sections).
	featCheck, features := checkFeatureMapHygiene(repoRoot)
	report.Features = features
	add(featCheck)

	return report
}

func checkIsolation(projectOverride string, envOverrides map[string]string) CheckResult {
	composeEnv := strings.TrimSpace(effectiveEnv("COMPOSE_PROJECT_NAME", envOverrides))
	verifyEnv := strings.TrimSpace(effectiveEnv("ALT_VERIFY_PROJECT", envOverrides))
	override := strings.TrimSpace(projectOverride)

	if isProductionProject(composeEnv) {
		return CheckResult{
			Name:   "isolation",
			OK:     false,
			Detail: "COMPOSE_PROJECT_NAME=alt targets production stack; unset it or use a non-production project (e.g. alt-verify-<id>)",
		}
	}
	if isProductionProject(verifyEnv) {
		return CheckResult{
			Name:   "isolation",
			OK:     false,
			Detail: "ALT_VERIFY_PROJECT=alt targets production stack and is forbidden",
		}
	}
	if isProductionProject(override) {
		return CheckResult{
			Name:   "isolation",
			OK:     false,
			Detail: "--project alt targets production stack and is forbidden",
		}
	}
	return CheckResult{
		Name:   "isolation",
		OK:     true,
		Detail: "production project 'alt' guard active; Mode A binds strictly to 127.0.0.1:0",
	}
}

func checkEvidenceDirWritable(evidenceDir string) CheckResult {
	if err := ensureNoSymlinkPath(evidenceDir); err != nil {
		return CheckResult{
			Name:   "evidence-dir",
			OK:     false,
			Detail: fmt.Sprintf("unsafe evidence directory %s: %v", evidenceDir, err),
		}
	}
	if err := os.MkdirAll(evidenceDir, fsperm.Dir); err != nil {
		return CheckResult{
			Name:   "evidence-dir",
			OK:     false,
			Detail: fmt.Sprintf("cannot create evidence directory %s: %v", evidenceDir, err),
		}
	}
	if err := ensureNoSymlinkPath(evidenceDir); err != nil {
		return CheckResult{
			Name:   "evidence-dir",
			OK:     false,
			Detail: fmt.Sprintf("unsafe evidence directory %s: %v", evidenceDir, err),
		}
	}
	f, err := os.CreateTemp(evidenceDir, ".doctor-write-probe-*")
	if err != nil {
		return CheckResult{
			Name:   "evidence-dir",
			OK:     false,
			Detail: fmt.Sprintf("evidence directory %s is not writable: %v", evidenceDir, err),
		}
	}
	probePath := f.Name()
	_ = f.Close()
	_ = os.Remove(probePath)
	return CheckResult{
		Name:   "evidence-dir",
		OK:     true,
		Detail: fmt.Sprintf("writable (%s)", evidenceDir),
	}
}

func checkSkillFilesAndSymlinks(repoRoot string) CheckResult {
	skills := []string{"verify-alt", "maintain-verification-skill"}
	var problems []string
	for _, name := range skills {
		claudeSkill := filepath.Join(repoRoot, ".claude", "skills", name, "SKILL.md")
		st, err := os.Stat(claudeSkill)
		if err != nil || !st.Mode().IsRegular() || st.Size() == 0 {
			problems = append(problems, fmt.Sprintf("missing or empty %s", filepath.ToSlash(filepath.Join(".claude", "skills", name, "SKILL.md"))))
		}
		agentEntry := filepath.Join(repoRoot, ".agents", "skills", name)
		info, lerr := os.Lstat(agentEntry)
		if lerr != nil {
			problems = append(problems, fmt.Sprintf("missing symlink %s", filepath.ToSlash(filepath.Join(".agents", "skills", name))))
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			problems = append(problems, fmt.Sprintf("%s must be a symlink to ../../.claude/skills/%s", filepath.ToSlash(filepath.Join(".agents", "skills", name)), name))
			continue
		}
		resolvedSkill := filepath.Join(agentEntry, "SKILL.md")
		rst, rerr := os.Stat(resolvedSkill)
		if rerr != nil || !rst.Mode().IsRegular() {
			problems = append(problems, fmt.Sprintf("broken symlink %s (SKILL.md unreachable)", filepath.ToSlash(filepath.Join(".agents", "skills", name))))
		}
	}
	if len(problems) > 0 {
		return CheckResult{
			Name:   "skills-and-symlinks",
			OK:     false,
			Detail: strings.Join(problems, "; "),
		}
	}
	return CheckResult{
		Name:   "skills-and-symlinks",
		OK:     true,
		Detail: "verify-alt and maintain-verification-skill SKILL.md + .agents/skills symlinks valid",
	}
}

func checkFeatureMapHygiene(repoRoot string) (CheckResult, []string) {
	featuresDir := filepath.Join(repoRoot, ".claude", "skills", "verify-alt", "features")
	featRoot, err := os.OpenRoot(featuresDir)
	if err != nil {
		return CheckResult{
			Name:   "feature-map-hygiene",
			OK:     false,
			Detail: fmt.Sprintf("cannot open features directory: %v", err),
		}, nil
	}
	defer func() { _ = featRoot.Close() }()

	repoFS, err := os.OpenRoot(repoRoot)
	if err != nil {
		return CheckResult{
			Name:   "feature-map-hygiene",
			OK:     false,
			Detail: fmt.Sprintf("cannot open repo root: %v", err),
		}, nil
	}
	defer func() { _ = repoFS.Close() }()

	readmeRaw, err := readRootFile(featRoot, "README.md")
	if err != nil {
		return CheckResult{
			Name:   "feature-map-hygiene",
			OK:     false,
			Detail: fmt.Sprintf("cannot read features/README.md: %v", err),
		}, nil
	}

	entries, err := os.ReadDir(featuresDir)
	if err != nil {
		return CheckResult{
			Name:   "feature-map-hygiene",
			OK:     false,
			Detail: fmt.Sprintf("cannot read features directory: %v", err),
		}, nil
	}

	diskFeatures := map[string]struct{}{}
	var featureNames []string
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if name == "README.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		diskFeatures[name] = struct{}{}
		featureNames = append(featureNames, name)
	}
	sort.Strings(featureNames)

	if len(featureNames) == 0 {
		return CheckResult{
			Name:   "feature-map-hygiene",
			OK:     false,
			Detail: "no feature documents found under .claude/skills/verify-alt/features/",
		}, nil
	}

	linkedFeatures := map[string]struct{}{}
	var problems []string
	for _, m := range markdownLinkRE.FindAllStringSubmatch(string(readmeRaw), -1) {
		if len(m) < 2 {
			continue
		}
		target := strings.TrimSpace(m[1])
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "#") {
			continue
		}
		if idx := strings.IndexByte(target, '#'); idx >= 0 {
			target = target[:idx]
		}
		if !strings.HasSuffix(target, ".md") {
			continue
		}
		resolved := filepath.Clean(filepath.Join(featuresDir, target))
		relToRepo, relErr := filepath.Rel(repoRoot, resolved)
		if relErr != nil || strings.HasPrefix(relToRepo, "..") || filepath.IsAbs(relToRepo) {
			problems = append(problems, fmt.Sprintf("link in features/README.md escapes repo root: %s", target))
			continue
		}
		if st, serr := repoFS.Stat(relToRepo); serr != nil || !st.Mode().IsRegular() {
			problems = append(problems, fmt.Sprintf("broken link in features/README.md: %s", target))
			continue
		}
		rel, rerr := filepath.Rel(featuresDir, resolved)
		if rerr == nil && !strings.Contains(rel, string(filepath.Separator)) && rel != "README.md" {
			linkedFeatures[rel] = struct{}{}
		}
	}

	for _, name := range featureNames {
		if _, ok := linkedFeatures[name]; !ok {
			problems = append(problems, fmt.Sprintf("orphan feature file not linked in features/README.md: %s", name))
		}
		raw, rerr := readRootFile(featRoot, name)
		if rerr != nil {
			problems = append(problems, fmt.Sprintf("cannot read feature file %s: %v", name, rerr))
			continue
		}
		content := string(raw)
		for _, heading := range requiredFeatureSec {
			if !hasMarkdownHeading(content, heading) {
				problems = append(problems, fmt.Sprintf("feature %s missing required section %q", name, heading))
			}
		}
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return CheckResult{
			Name:   "feature-map-hygiene",
			OK:     false,
			Detail: strings.Join(problems, "; "),
		}, featureNames
	}
	return CheckResult{
		Name:   "feature-map-hygiene",
		OK:     true,
		Detail: fmt.Sprintf("%d feature docs indexed with 0 orphans, 0 broken links, and all 4 required sections", len(featureNames)),
	}, featureNames
}

func readRootFile(r *os.Root, name string) ([]byte, error) {
	f, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func hasMarkdownHeading(content, heading string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == heading {
			return true
		}
	}
	return false
}

// sseReadinessImports mirrors the imports of sseLiveServerPy. Importing the
// handler runs the news_creator.handler package init, which pulls in most of
// news-creator's runtime dependencies — fastapi and uvicorn alone do not
// satisfy it.
const sseReadinessImports = `import sys
sys.path.insert(0, sys.argv[1])
import uvicorn
from fastapi import FastAPI, Request
from news_creator.handler.sse_stream import SSEHeartbeatConfig, create_sse_response
`

// resolvePythonForSSE returns a command prefix that can import the news-creator
// SSE handler for live verification, preferring news-creator's local .venv.
// Fails closed when no candidate can import the SSE handler (ADR-001036).
func resolvePythonForSSE(ctx context.Context, repoRoot string) ([]string, string, error) {
	ncDir := filepath.Join(repoRoot, "news-creator", "app")
	type candidate struct {
		label string
		cmd   []string
	}
	var candidates []candidate
	venvPy := filepath.Join(ncDir, ".venv", "bin", "python")
	if st, err := os.Stat(venvPy); err == nil && !st.IsDir() {
		candidates = append(candidates, candidate{label: "news-creator .venv", cmd: []string{venvPy}})
	}
	if py3, err := exec.LookPath("python3"); err == nil {
		candidates = append(candidates, candidate{label: "python3", cmd: []string{py3}})
	}
	if uvPath, err := exec.LookPath("uv"); err == nil {
		if st, serr := os.Stat(ncDir); serr == nil && st.IsDir() {
			candidates = append(candidates, candidate{
				label: "uv offline news-creator",
				cmd:   []string{uvPath, "run", "--offline", "--directory", ncDir, "python"},
			})
		}
	}
	var tried []string
	for _, c := range candidates {
		args := append(append([]string(nil), c.cmd...), "-c", sseReadinessImports, ncDir)
		_, _, code, err := execx.Combined(ctx, execx.Cmd{Args: args})
		if err == nil && code == 0 {
			return c.cmd, c.label + " (SSE handler importable)", nil
		}
		tried = append(tried, c.label)
	}
	if len(tried) == 0 {
		tried = []string{"none found"}
	}
	return nil, "", fmt.Errorf(
		"no Python can import news_creator.handler.sse_stream (tried: %s); install news-creator's dependencies with `uv sync` in news-creator/app",
		strings.Join(tried, ", "),
	)
}

func resolveStdlibPython() (string, error) {
	if py3, err := exec.LookPath("python3"); err == nil {
		return py3, nil
	}
	return "", fmt.Errorf("python3 not found in PATH")
}
