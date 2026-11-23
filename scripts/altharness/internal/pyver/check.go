package pyver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/pelletier/go-toml/v2"
)

const (
	exitClean      = 0
	exitViolations = 1
	exitUsage      = 2
)

var scriptsExpectedFloor = [2]int{3, 13}
var pep758Min = [2]int{3, 14}

var excludedRepoPrefixes = []string{"stopped-using-k8s/", "scripts/harness/corpus/"}

var excludedDirNames = map[string]struct{}{
	".git": {}, ".venv": {}, "__pycache__": {}, "build": {}, "dist": {},
	"node_modules": {}, ".mypy_cache": {}, ".pytest_cache": {}, ".ruff_cache": {},
}

var floorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`>=\s*(\d+)\.(\d+)`),
	regexp.MustCompile(`~=\s*(\d+)\.(\d+)`),
	regexp.MustCompile(`==\s*(\d+)\.(\d+)`),
}

var dockerfileFromRE = regexp.MustCompile(`(?i)^\s*FROM\s+(?:--platform=\S+\s+)?(?:docker\.io/(?:library/)?)?python:(\d+)\.(\d+)`)

// Violation is one ARCH-03 finding.
type Violation struct {
	Path    string
	Line    int
	Rule    string
	Message string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: [%s] %s", v.Path, v.Line, v.Rule, v.Message)
}

type pyRuntime struct {
	bin   string
	major int
	minor int
}

func extractRequiresPythonFloor(spec string) *[2]int {
	for _, pattern := range floorPatterns {
		m := pattern.FindStringSubmatch(spec)
		if m == nil {
			continue
		}
		maj := atoi(m[1])
		min := atoi(m[2])
		out := [2]int{maj, min}
		return &out
	}
	return nil
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		n = n*10 + int(c-'0')
	}
	return n
}

func relDisplay(path, repoRoot string) string {
	abs, err1 := filepath.Abs(path)
	root, err2 := filepath.Abs(repoRoot)
	if err1 != nil || err2 != nil {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func floorLess(a, b [2]int) bool {
	if a[0] != b[0] {
		return a[0] < b[0]
	}
	return a[1] < b[1]
}

type sourceUnit struct {
	display  string
	floor    [2]int
	text     string
	readErr  *Violation
	pep      []Violation
	reported map[int]struct{}
	needAST  bool
}

func checkSourceText(text, display string, floor [2]int, rt pyRuntime) sourceUnit {
	unit := sourceUnit{display: display, floor: floor, text: text, reported: map[int]struct{}{}}
	if floorLess(floor, pep758Min) {
		for _, lineNo := range findUnparenthesizedExceptLines(text) {
			unit.reported[lineNo] = struct{}{}
			unit.pep = append(unit.pep, Violation{
				Path: display,
				Line: lineNo,
				Rule: "source.python.syntax-incompatible",
				Message: fmt.Sprintf(
					"unparenthesized multi-exception `except` (PEP 758) requires Python 3.14+, incompatible with requires-python floor %d.%d",
					floor[0], floor[1],
				),
			})
		}
	}
	runtime := [2]int{rt.major, rt.minor}
	if floorLess(runtime, floor) {
		unit.needAST = false
		return unit
	}
	unit.needAST = true
	return unit
}

func finishSource(unit sourceUnit, ast *astResponse, rt pyRuntime) []Violation {
	out := append([]Violation{}, unit.pep...)
	floor := unit.floor
	runtime := [2]int{rt.major, rt.minor}
	if floorLess(runtime, floor) {
		out = append(out, Violation{
			Path: unit.display,
			Line: 1,
			Rule: "source.python.runtime-too-old",
			Message: fmt.Sprintf(
				"current Python runtime %d.%d is older than requires-python floor %d.%d; cannot verify AST syntax",
				runtime[0], runtime[1], floor[0], floor[1],
			),
		})
		return out
	}
	if ast != nil && !ast.OK {
		lineNo := ast.Line
		if lineNo == 0 {
			lineNo = 1
		}
		if _, seen := unit.reported[lineNo]; !seen {
			out = append(out, Violation{
				Path:    unit.display,
				Line:    lineNo,
				Rule:    "source.python.syntax-incompatible",
				Message: fmt.Sprintf("syntax incompatible with Python %d.%d: %s", floor[0], floor[1], ast.Msg),
			})
		}
	}
	return out
}

func readSource(path, display string) (string, *Violation) {
	raw, err := os.ReadFile(path)
	if err != nil {
		v := Violation{Path: display, Line: 1, Rule: "source.python.read-error", Message: fmt.Sprintf("failed to read Python source: %s", err.Error())}
		return "", &v
	}
	if !utf8.Valid(raw) {
		v := Violation{Path: display, Line: 1, Rule: "source.python.read-error", Message: "failed to read Python source: invalid utf-8"}
		return "", &v
	}
	return string(raw), nil
}

func parsePyproject(path, repoRoot string) (map[string]any, []Violation) {
	rel := relDisplay(path, repoRoot)
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, []Violation{{Path: rel, Line: 1, Rule: "config.python.pyproject-parse-error", Message: fmt.Sprintf("failed to parse pyproject.toml: %s", err.Error())}}
	}
	if !utf8.Valid(raw) {
		return nil, []Violation{{Path: rel, Line: 1, Rule: "config.python.pyproject-parse-error", Message: "failed to parse pyproject.toml: invalid utf-8"}}
	}
	var data map[string]any
	if err := toml.Unmarshal(raw, &data); err != nil {
		return nil, []Violation{{Path: rel, Line: 1, Rule: "config.python.pyproject-parse-error", Message: fmt.Sprintf("failed to parse pyproject.toml: %s", err.Error())}}
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func checkRuffTargetVersion(data map[string]any, rel string, violations *[]Violation) {
	toolRaw, ok := asMap(data["tool"])
	if !ok {
		return
	}
	ruffRaw, ok := asMap(toolRaw["ruff"])
	if !ok {
		return
	}
	sections := []struct {
		name string
		obj  any
	}{
		{"[tool.ruff]", ruffRaw},
		{"[tool.ruff.lint]", ruffRaw["lint"]},
		{"[tool.ruff.format]", ruffRaw["format"]},
	}
	for _, section := range sections {
		obj, ok := asMap(section.obj)
		if !ok {
			continue
		}
		if _, exists := obj["target-version"]; exists {
			*violations = append(*violations, Violation{
				Path: rel,
				Line: 1,
				Rule: "config.python.ruff-target-version-forbidden",
				Message: section.name + ".target-version is forbidden (ARCH-03); " +
					"remove it so Ruff derives target version exclusively from " +
					"[project].requires-python",
			})
		}
	}
}

func extractAndValidateFloor(data map[string]any, rel string, violations *[]Violation) *[2]int {
	projectRaw, _ := asMap(data["project"])
	var reqRaw any
	if projectRaw != nil {
		reqRaw = projectRaw["requires-python"]
	}
	req, ok := reqRaw.(string)
	if !ok || strings.TrimSpace(req) == "" {
		*violations = append(*violations, Violation{
			Path:    rel,
			Line:    1,
			Rule:    "config.python.requires-python-missing",
			Message: "[project].requires-python must be declared as the single source of truth",
		})
		return nil
	}
	floor := extractRequiresPythonFloor(req)
	if floor == nil {
		*violations = append(*violations, Violation{
			Path: rel,
			Line: 1,
			Rule: "config.python.requires-python-invalid",
			Message: fmt.Sprintf(
				"could not extract minimum Python major.minor version from requires-python=%s",
				pyReprString(req),
			),
		})
		return nil
	}
	if rel == "scripts/pyproject.toml" && *floor != scriptsExpectedFloor {
		*violations = append(*violations, Violation{
			Path: rel,
			Line: 1,
			Rule: "config.python.scripts-requires-python-mismatch",
			Message: fmt.Sprintf(
				"scripts/pyproject.toml must declare requires-python floor %d.%d to match host python3, got %s",
				scriptsExpectedFloor[0], scriptsExpectedFloor[1], pyReprString(req),
			),
		})
	}
	return floor
}

func discoverDockerfiles(projectDir, repoRoot string) ([]string, error) {
	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	dirs := []string{absProject}
	parent := filepath.Dir(absProject)
	if filepath.Base(absProject) == "app" && parent != absRoot {
		if st, err := os.Stat(parent); err == nil && st.IsDir() {
			dirs = append(dirs, parent)
		}
	}
	var found []string
	seen := map[string]struct{}{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		names := make([]string, 0, len(entries))
		for _, ent := range entries {
			names = append(names, ent.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			if name != "Dockerfile" && !strings.HasPrefix(name, "Dockerfile.") {
				continue
			}
			path := filepath.Join(dir, name)
			info, err := os.Lstat(path)
			if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			found = append(found, path)
		}
	}
	return found, nil
}

func checkDockerfiles(projectDir string, floor [2]int, repoRoot string) []Violation {
	var violations []Violation
	files, err := discoverDockerfiles(projectDir, repoRoot)
	if err != nil {
		return []Violation{{Path: relDisplay(projectDir, repoRoot), Line: 1, Rule: "config.python.dockerfile-read-error", Message: err.Error()}}
	}
	for _, dockerfile := range files {
		rel := relDisplay(dockerfile, repoRoot)
		raw, err := os.ReadFile(dockerfile)
		if err != nil || !utf8.Valid(raw) {
			msg := "failed to read Dockerfile"
			if err != nil {
				msg = fmt.Sprintf("failed to read Dockerfile: %s", err.Error())
			} else {
				msg = "failed to read Dockerfile: invalid utf-8"
			}
			violations = append(violations, Violation{Path: rel, Line: 1, Rule: "config.python.dockerfile-read-error", Message: msg})
			continue
		}
		lines := splitLines(string(raw))
		for i, rawLine := range lines {
			stripped := strings.TrimSpace(rawLine)
			if stripped == "" || strings.HasPrefix(stripped, "#") {
				continue
			}
			m := dockerfileFromRE.FindStringSubmatch(stripped)
			if m == nil {
				continue
			}
			df := [2]int{atoi(m[1]), atoi(m[2])}
			if df != floor {
				violations = append(violations, Violation{
					Path: rel,
					Line: i + 1,
					Rule: "config.python.dockerfile-version-mismatch",
					Message: fmt.Sprintf(
						"Dockerfile base image python:%d.%d does not match pyproject.toml requires-python floor %d.%d",
						df[0], df[1], floor[0], floor[1],
					),
				})
			}
		}
	}
	return violations
}

func iterProjectPythonFiles(projectDir string) ([]string, error) {
	resolved, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, err
	}
	var files []string
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		relCur, err := filepath.Rel(resolved, dir)
		if err != nil {
			return err
		}
		relCur = filepath.ToSlash(relCur)
		if relCur == "." {
			relCur = ""
		}
		var dirs []string
		var pys []string
		for _, ent := range entries {
			name := ent.Name()
			path := filepath.Join(dir, name)
			info, lerr := os.Lstat(path)
			if lerr != nil {
				continue
			}
			if info.IsDir() {
				if _, skip := excludedDirNames[name]; skip {
					continue
				}
				if relCur == "harness" && name == "corpus" {
					continue
				}
				if info.Mode()&os.ModeSymlink != 0 {
					continue
				}
				dirs = append(dirs, name)
				continue
			}
			if strings.HasSuffix(name, ".py") {
				pys = append(pys, name)
			}
		}
		sort.Strings(dirs)
		sort.Strings(pys)
		for _, name := range pys {
			files = append(files, filepath.Join(dir, name))
		}
		for _, name := range dirs {
			if err := walk(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(resolved); err != nil {
		return nil, err
	}
	return files, nil
}

type projectResult struct {
	violations []Violation
	sources    []sourceUnit
}

func checkProject(projectDir, repoRoot string, rt pyRuntime) (projectResult, error) {
	pyproject := filepath.Join(projectDir, "pyproject.toml")
	data, parseViolations := parsePyproject(pyproject, repoRoot)
	if data == nil {
		return projectResult{violations: parseViolations}, nil
	}
	rel := relDisplay(pyproject, repoRoot)
	var violations []Violation
	checkRuffTargetVersion(data, rel, &violations)
	floor := extractAndValidateFloor(data, rel, &violations)
	res := projectResult{violations: violations}
	if floor == nil {
		return res, nil
	}
	res.violations = append(res.violations, checkDockerfiles(projectDir, *floor, repoRoot)...)
	files, err := iterProjectPythonFiles(projectDir)
	if err != nil {
		return res, err
	}
	for _, pyFile := range files {
		display := relDisplay(pyFile, repoRoot)
		text, readErr := readSource(pyFile, display)
		if readErr != nil {
			res.violations = append(res.violations, *readErr)
			continue
		}
		res.sources = append(res.sources, checkSourceText(text, display, *floor, rt))
	}
	return res, nil
}

func excludedRepoRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	for _, prefix := range excludedRepoPrefixes {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func discoverRepoPyprojects(ctx context.Context, repoRoot string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(repoRoot, ".git")); err == nil {
		if gitBin, err := execLook("git"); err == nil {
			out, err := gitOutput(ctx, gitBin, repoRoot, "ls-files", "*pyproject.toml")
			if err == nil {
				var results []string
				lines := splitLines(string(out))
				sort.Strings(lines)
				for _, raw := range lines {
					rel := strings.TrimSpace(raw)
					if rel == "" || excludedRepoRel(rel) {
						continue
					}
					candidate := filepath.Join(repoRoot, filepath.FromSlash(rel))
					if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
						results = append(results, candidate)
					}
				}
				return results, nil
			}
		}
	}
	var found []string
	resolved, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(resolved, dir)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		var dirs []string
		for _, ent := range entries {
			name := ent.Name()
			path := filepath.Join(dir, name)
			info, lerr := os.Lstat(path)
			if lerr != nil {
				continue
			}
			if info.IsDir() {
				if _, skip := excludedDirNames[name]; skip {
					continue
				}
				next := name
				if rel != "" {
					next = rel + "/" + name
				}
				if excludedRepoRel(next+"/") || excludedRepoRel(next) {
					// skip excluded trees
					if strings.HasPrefix(next+"/", "stopped-using-k8s/") || strings.HasPrefix(next, "stopped-using-k8s") && (next == "stopped-using-k8s" || strings.HasPrefix(next, "stopped-using-k8s/")) {
						continue
					}
					if next == "scripts/harness/corpus" || strings.HasPrefix(next, "scripts/harness/corpus/") {
						continue
					}
				}
				if info.Mode()&os.ModeSymlink != 0 {
					continue
				}
				dirs = append(dirs, name)
				continue
			}
			if name == "pyproject.toml" {
				fileRel := name
				if rel != "" {
					fileRel = rel + "/" + name
				}
				if !excludedRepoRel(fileRel) {
					found = append(found, path)
				}
			}
		}
		sort.Strings(dirs)
		for _, name := range dirs {
			if err := walk(filepath.Join(dir, name)); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(resolved); err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

func pyReprString(s string) string {
	quote := byte('\'')
	if strings.Contains(s, "'") && !strings.Contains(s, "\"") {
		quote = '"'
	}
	var b strings.Builder
	b.WriteByte(quote)
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case rune(quote):
			b.WriteByte('\\')
			b.WriteByte(quote)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\x%02x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte(quote)
	return b.String()
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
