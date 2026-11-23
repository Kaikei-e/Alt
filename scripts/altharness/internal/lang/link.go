package lang

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// RunLinkNaming is scripts/lint/check-link-vs-url-naming.sh.
func RunLinkNaming(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	_, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	root, rc := mustRepo(stderr)
	if rc != 0 {
		return rc
	}
	files := collectKnowledgeFiles(root)
	if len(files) == 0 {
		fmt.Fprintln(stderr, "OK: no Knowledge-layer files matched the scan.")
		return 0
	}
	patterns := []string{
		`^\s*Link\s+string\s+` + "`",
		`json:"link"`,
		`db:"link"`,
		`^\s*link\??:\s*string`,
		`^\s*"link":\s*string`,
	}
	violations := 0
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			fmt.Fprintf(stderr, "error: bad pattern %s: %s\n", pattern, err.Error())
			return 1
		}
		var matches []string
		for _, file := range files {
			if ctx.Err() != nil {
				fmt.Fprintf(stderr, "error: %s\n", ctx.Err().Error())
				return 1
			}
			data, err := os.ReadFile(file)
			if err != nil {
				fmt.Fprintf(stderr, "error: read %s: %s\n", file, err.Error())
				return 1
			}
			rel, err := filepath.Rel(root, file)
			if err != nil {
				rel = file
			}
			rel = filepath.ToSlash(rel)
			for i, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "allow:link-rss-spec") || !re.MatchString(line) {
					continue
				}
				matches = append(matches, fmt.Sprintf("%s:%d:%s", rel, i+1, line))
			}
		}
		if len(matches) > 0 {
			fmt.Fprintf(stderr, "::error::URL-vs-Link drift in Knowledge layer (pattern: %s)\n", pattern)
			fmt.Fprintln(stderr, strings.Join(matches, "\n"))
			violations++
		}
	}
	if violations > 0 {
		fmt.Fprint(stderr, `
URL-vs-Link naming drift detected.

The Knowledge Home / Knowledge Loop layers must use URL (Article URL)
or WebsiteURL (Website URL) per docs/glossary/ubiquitous-language.md.
"Link" / "link" survives only inside the RSS parser boundary
(driver/gateway). If a hit above is intentional RSS-spec preserve,
append "allow:link-rss-spec" to the line as a marker comment.

If a new Knowledge field genuinely needs the "Link" name, update the
glossary first and document the exception there before silencing the
linter.
`)
		return 1
	}
	fmt.Fprintln(stderr, "OK: no URL-vs-Link naming drift in Knowledge layers.")
	return 0
}

func collectKnowledgeFiles(root string) []string {
	type spec struct {
		dir   string
		depth int
		match func(rel, name string) bool
	}
	specs := []spec{
		{filepath.Join(root, "alt-backend", "app", "domain"), 1, func(rel, name string) bool {
			return strings.HasPrefix(name, "knowledge_") && strings.HasSuffix(name, ".go")
		}},
		{filepath.Join(root, "alt-backend", "app", "connect", "v2"), 0, func(rel, name string) bool {
			return strings.HasSuffix(name, ".go") && strings.Contains(filepath.ToSlash(rel), "knowledge_")
		}},
		{filepath.Join(root, "alt-backend", "app", "usecase"), 0, func(rel, name string) bool {
			return strings.HasSuffix(name, ".go") && strings.Contains(filepath.ToSlash(rel), "knowledge_")
		}},
		{filepath.Join(root, "alt-backend", "app", "job"), 1, func(rel, name string) bool {
			return strings.HasPrefix(name, "knowledge_") && strings.HasSuffix(name, ".go")
		}},
		{filepath.Join(root, "alt-frontend-sv", "src", "lib", "components", "knowledge-home"), 0, func(rel, name string) bool {
			return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".svelte")
		}},
		{filepath.Join(root, "alt-frontend-sv", "src", "lib", "components", "knowledge-loop"), 0, func(rel, name string) bool {
			return strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".svelte")
		}},
		{filepath.Join(root, "alt-frontend-sv", "src", "lib", "connect"), 0, func(rel, name string) bool {
			return strings.HasPrefix(name, "knowledge_")
		}},
	}
	var files []string
	for _, spec := range specs {
		_ = filepath.WalkDir(spec.dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, relErr := filepath.Rel(spec.dir, path)
			if relErr != nil {
				return nil
			}
			depth := 0
			if rel != "." {
				depth = strings.Count(rel, string(os.PathSeparator))
			}
			if d.IsDir() {
				if spec.depth > 0 && depth >= spec.depth {
					return filepath.SkipDir
				}
				return nil
			}
			if spec.depth > 0 && depth >= spec.depth {
				return nil
			}
			name := d.Name()
			slash := filepath.ToSlash(rel)
			if strings.Contains(slash, "/gen/") || strings.HasPrefix(slash, "gen/") {
				return nil
			}
			if strings.Contains(name, "_test.") || strings.HasSuffix(name, ".test.ts") {
				return nil
			}
			if spec.match(rel, name) {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}
