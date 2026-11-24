package generated

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

var (
	semverRE       = regexp.MustCompile(`^v\d+\.\d+\.\d+(?:[-+][^\s]+)?$`)
	bufRemoteRE    = regexp.MustCompile(`(?:^\s*(?:-\s*)?|[{,]\s*)remote\s*:\s*(?:"([^"]+)"|'([^']+)'|([^\s,}\]]+))`)
	bufPinnedTagRE = regexp.MustCompile(`:v\d+(?:\.\d+)*$`)
	atLatestRE     = regexp.MustCompile("(?:^|[\\s\"'(=`])[^\\s\"'(=`]+@latest(?:$|[\\s\"'`);,|&\\\\<>])")
	goGenerateRE   = regexp.MustCompile(`^\s*//\s*go:generate\b`)
)

func (c *checker) checkPins(ctx context.Context, repoRoot string) ([]Violation, error) {
	root := prepareRoot(repoRoot)
	var violations []Violation
	violations = append(violations, checkGoToolMod(root)...)
	violations = append(violations, checkBufGenPins(root)...)
	latest, err := c.checkAtLatestReferences(ctx, root)
	if err != nil {
		return nil, err
	}
	violations = append(violations, latest...)
	return violations, nil
}

func checkGoToolMod(repoRoot string) []Violation {
	const rel = "scripts/codegen/go.tool.mod"
	full := filepath.Join(repoRoot, filepath.FromSlash(rel))
	if !isRegularFile(full) {
		return []Violation{{
			Path:    rel,
			Line:    1,
			Rule:    ruleMissingToolMod,
			Message: "missing pinned Go tool manifest scripts/codegen/go.tool.mod",
		}}
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		return []Violation{{
			Path:    rel,
			Line:    1,
			Rule:    ruleInvalidToolMod,
			Message: "go.tool.mod must declare at least one `tool` directive",
		}}
	}
	text := string(raw)
	var violations []Violation
	for i, line := range splitLines(text) {
		if strings.Contains(line, "@latest") {
			violations = append(violations, Violation{
				Path:    rel,
				Line:    i + 1,
				Rule:    ruleUnpinnedLatest,
				Message: "go.tool.mod must not reference @latest",
			})
		}
	}
	tools, requires := parseGoToolMod(text)
	if len(tools) == 0 {
		violations = append(violations, Violation{
			Path:    rel,
			Line:    1,
			Rule:    ruleInvalidToolMod,
			Message: "go.tool.mod must declare at least one `tool` directive",
		})
		return violations
	}
	for _, toolPkg := range tools {
		if !toolHasPinnedRequire(toolPkg, requires) {
			violations = append(violations, Violation{
				Path:    rel,
				Line:    1,
				Rule:    ruleInvalidToolMod,
				Message: "tool `" + toolPkg + "` lacks a pinned semver `require` entry in go.tool.mod",
			})
		}
	}
	return violations
}

func parseGoToolMod(text string) ([]string, map[string]string) {
	var tools []string
	requires := map[string]string{}
	inBlock := ""
	for _, rawLine := range splitLines(text) {
		line := rawLine
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if inBlock != "" {
			if line == ")" {
				inBlock = ""
				continue
			}
			parts := strings.Fields(line)
			if inBlock == "tool" && len(parts) > 0 {
				tools = append(tools, parts[0])
			} else if inBlock == "require" && len(parts) >= 2 {
				requires[parts[0]] = parts[1]
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "tool ("):
			inBlock = "tool"
		case strings.HasPrefix(line, "require ("):
			inBlock = "require"
		case strings.HasPrefix(line, "tool "):
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				tools = append(tools, parts[1])
			}
		case strings.HasPrefix(line, "require "):
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				requires[parts[1]] = parts[2]
			}
		}
	}
	return tools, requires
}

func toolHasPinnedRequire(toolPkg string, requires map[string]string) bool {
	for modPath, version := range requires {
		if (toolPkg == modPath || strings.HasPrefix(toolPkg, modPath+"/")) && semverRE.MatchString(version) {
			return true
		}
	}
	return false
}

func stripYAMLComment(raw string) string {
	inQuote := byte(0)
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if escaped {
			escaped = false
			continue
		}
		if inQuote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if inQuote != 0 {
			if ch == inQuote {
				inQuote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = ch
			continue
		}
		if ch == '#' && (i == 0 || unicode.IsSpace(rune(raw[i-1]))) {
			return trimRightSpace(raw[:i])
		}
	}
	return trimRightSpace(raw)
}

func checkBufGenPins(repoRoot string) []Violation {
	protoDir := filepath.Join(repoRoot, "proto")
	info, err := os.Stat(protoDir)
	if err != nil || !info.IsDir() {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(protoDir, "buf.gen*.yaml"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	var violations []Violation
	for _, cfg := range matches {
		rel, relErr := filepath.Rel(repoRoot, cfg)
		if relErr != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		raw, err := os.ReadFile(cfg)
		if err != nil {
			continue
		}
		for i, rawLine := range splitLines(string(raw)) {
			line := stripYAMLComment(rawLine)
			if strings.TrimSpace(line) == "" {
				continue
			}
			for _, match := range bufRemoteRE.FindAllStringSubmatch(line, -1) {
				plugin := ""
				for _, group := range match[1:] {
					if strings.TrimSpace(group) != "" {
						plugin = strings.TrimSpace(group)
						break
					}
				}
				if plugin == "" || bufPinnedTagRE.MatchString(plugin) {
					continue
				}
				violations = append(violations, Violation{
					Path: rel,
					Line: i + 1,
					Rule: ruleUnpinnedBuf,
					Message: "unpinned remote buf plugin `" + plugin + "`; " +
						"specify an explicit `:v<version>` tag",
				})
			}
		}
	}
	return violations
}

func isLatestScanTarget(rel string) bool {
	rel = filepath.ToSlash(rel)
	if isExcludedRel(rel) || strings.HasPrefix(rel, "docs/ADR/") || strings.HasPrefix(rel, "docs/review/") {
		return false
	}
	name := path.Base(rel)
	if strings.HasPrefix(rel, ".github/workflows/") {
		if strings.HasPrefix(rel, ".github/workflows/scripts/") && strings.HasPrefix(name, "test_") {
			return false
		}
		return true
	}
	if strings.HasPrefix(rel, "scripts/") {
		if strings.HasPrefix(rel, "scripts/tests/") || strings.HasPrefix(rel, "scripts/harness/corpus/") {
			return false
		}
		return true
	}
	if strings.HasPrefix(rel, "docs/runbooks/") || strings.HasPrefix(rel, "docs/best_practices/") ||
		strings.Contains(name, "Dockerfile") || strings.HasPrefix(name, "Makefile") {
		return true
	}
	return strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !strings.Contains("/"+rel, "/testdata/")
}

func (c *checker) checkAtLatestReferences(ctx context.Context, repoRoot string) ([]Violation, error) {
	root := prepareRoot(repoRoot)
	files, err := c.listRepoFiles(ctx, root)
	if err != nil {
		return nil, err
	}
	var violations []Violation
	for _, rel := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !isLatestScanTarget(rel) {
			continue
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		if !isSafeWorktreeFile(root, full) {
			continue
		}
		raw, err := os.ReadFile(full)
		if err != nil {
			continue
		}
		isGo := strings.HasSuffix(rel, ".go")
		for i, line := range splitLines(string(raw)) {
			if isGo && !goGenerateRE.MatchString(line) {
				continue
			}
			if atLatestRE.MatchString(line) {
				violations = append(violations, Violation{
					Path:    rel,
					Line:    i + 1,
					Rule:    ruleUnpinnedLatest,
					Message: "unpinned `@latest` reference found: " + strings.TrimSpace(line),
				})
			}
		}
	}
	return violations, nil
}
