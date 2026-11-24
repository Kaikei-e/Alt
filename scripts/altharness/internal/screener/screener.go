// Package screener is the CHECK-01 behavior-change screener.
// It compares before/after sources for Go, Python, Rust, and TypeScript
// (including Svelte script blocks) and prints a JSON candidate array.
//
// Exit codes match scripts/lint/behavior_screener.py: 0 when no candidates
// are emitted (or candidates are emitted without --fail-on-candidates),
// 1 when --fail-on-candidates is set and at least one candidate is found,
// 2 on usage, I/O, git, or parse errors.
package screener

import (
	"alt/scripts/altharness/internal/fsperm"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	exitOK         = 0
	exitCandidates = 1
	exitUsage      = 2
)

const usageText = `usage: screener (--before file --after file | --repo-root dir --base rev) [options]

Pair mode:
  --before file --after file [--logical-path path] [--language go|python|rust|typescript]

Git diff mode:
  --repo-root dir --base rev [--head rev]

Common:
  --output file            write the JSON array to a file instead of stdout
  --fail-on-candidates     exit 1 when any behavior-change candidate is found
`

// Candidate is one structural behavior-change finding.
type Candidate struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Function string `json:"function"`
	Kind     string `json:"kind"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

var extToLang = map[string]string{
	".go":     "go",
	".py":     "python",
	".rs":     "rust",
	".ts":     "typescript",
	".tsx":    "typescript",
	".mts":    "typescript",
	".cts":    "typescript",
	".js":     "typescript",
	".jsx":    "typescript",
	".mjs":    "typescript",
	".cjs":    "typescript",
	".svelte": "typescript",
}

var supportedLangs = map[string]struct{}{
	"go": {}, "python": {}, "rust": {}, "typescript": {},
}

// Run is the CLI entry. args excludes the program name.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprintf(stderr, "error: nil context\n")
		return exitUsage
	}
	if err := ctx.Err(); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitUsage
	}
	opt, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitUsage
	}
	if opt.help {
		fmt.Fprint(stdout, usageText)
		return exitOK
	}
	cands, err := execute(ctx, opt)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitUsage
	}
	if cands == nil {
		cands = []Candidate{}
	}
	payload, err := marshalCandidates(cands)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitUsage
	}
	if opt.output != "" {
		dir := filepath.Dir(opt.output)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, fsperm.Dir); err != nil {
				fmt.Fprintf(stderr, "error: %s\n", err.Error())
				return exitUsage
			}
		}
		if err := os.WriteFile(opt.output, payload, fsperm.File); err != nil {
			fmt.Fprintf(stderr, "error: %s\n", err.Error())
			return exitUsage
		}
		if opt.fail && len(cands) > 0 {
			fmt.Fprintf(stderr, "CHECK-01 detected %d behavior-change candidate(s): %s\n", len(cands), joinSortedKinds(cands))
		}
	} else if _, err := stdout.Write(payload); err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return exitUsage
	}
	if opt.fail && len(cands) > 0 {
		return exitCandidates
	}
	return exitOK
}

type options struct {
	before, after string
	logical       string
	language      string
	languageSet   bool
	repo, base    string
	head          string
	headSet       bool
	output        string
	fail          bool
	help          bool
}

func parseArgs(args []string) (options, error) {
	var o options
	take := func(i *int, name, raw string) (string, error) {
		if raw == name {
			if *i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", name)
			}
			*i++
			return args[*i], nil
		}
		prefix := name + "="
		if strings.HasPrefix(raw, prefix) {
			return strings.TrimPrefix(raw, prefix), nil
		}
		return "", fmt.Errorf("unrecognized arguments: %s", raw)
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			o.help = true
		case a == "--fail-on-candidates":
			o.fail = true
		case a == "--before" || strings.HasPrefix(a, "--before="):
			v, err := take(&i, "--before", a)
			if err != nil {
				return o, err
			}
			o.before = v
		case a == "--after" || strings.HasPrefix(a, "--after="):
			v, err := take(&i, "--after", a)
			if err != nil {
				return o, err
			}
			o.after = v
		case a == "--logical-path" || strings.HasPrefix(a, "--logical-path="):
			v, err := take(&i, "--logical-path", a)
			if err != nil {
				return o, err
			}
			o.logical = v
		case a == "--language" || strings.HasPrefix(a, "--language="):
			v, err := take(&i, "--language", a)
			if err != nil {
				return o, err
			}
			o.language = strings.ToLower(v)
			o.languageSet = true
		case a == "--repo-root" || strings.HasPrefix(a, "--repo-root="):
			v, err := take(&i, "--repo-root", a)
			if err != nil {
				return o, err
			}
			o.repo = v
		case a == "--base" || strings.HasPrefix(a, "--base="):
			v, err := take(&i, "--base", a)
			if err != nil {
				return o, err
			}
			o.base = v
		case a == "--head" || strings.HasPrefix(a, "--head="):
			v, err := take(&i, "--head", a)
			if err != nil {
				return o, err
			}
			o.head = v
			o.headSet = true
		case a == "--output" || strings.HasPrefix(a, "--output="):
			v, err := take(&i, "--output", a)
			if err != nil {
				return o, err
			}
			o.output = v
		default:
			return o, fmt.Errorf("unrecognized arguments: %s", a)
		}
	}
	if o.languageSet {
		if _, ok := supportedLangs[o.language]; !ok {
			return o, fmt.Errorf("invalid --language %q", o.language)
		}
	}
	return o, nil
}

func execute(ctx context.Context, o options) ([]Candidate, error) {
	pair := o.before != "" || o.after != ""
	gitMode := o.repo != "" || o.base != ""
	if pair == gitMode {
		return nil, fmt.Errorf("specify either pair mode (--before/--after) or git diff mode (--repo-root/--base)")
	}
	if pair {
		if o.before == "" || o.after == "" {
			return nil, fmt.Errorf("pair mode requires both --before and --after")
		}
		if err := mustRegularFile(o.before, "--before"); err != nil {
			return nil, err
		}
		if err := mustRegularFile(o.after, "--after"); err != nil {
			return nil, err
		}
		before, err := readText(o.before)
		if err != nil {
			return nil, err
		}
		after, err := readText(o.after)
		if err != nil {
			return nil, err
		}
		logical := o.logical
		if logical == "" {
			logical = o.after
		}
		return screenSources(before, after, logical, o.before, o.language, o.languageSet)
	}
	if o.repo == "" || o.base == "" {
		return nil, fmt.Errorf("git diff mode requires both --repo-root and --base")
	}
	info, err := os.Stat(o.repo)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("--repo-root directory not found: %s", o.repo)
	}
	var head *string
	if o.headSet {
		head = &o.head
	}
	return screenGitDiff(ctx, o.repo, o.base, head)
}

func screenSources(beforeSrc, afterSrc, logical, beforeHint, explicit string, explicitSet bool) ([]Candidate, error) {
	lang := inferLang(logical, explicit, explicitSet)
	if lang == "" {
		lang = inferLang(beforeHint, "", false)
	}
	if _, ok := supportedLangs[lang]; !ok {
		return []Candidate{}, nil
	}
	if beforeSrc == afterSrc {
		return []Candidate{}, nil
	}
	var cands []Candidate
	switch lang {
	case "go":
		beforeSum, err := analyzeGo(logical, []byte(beforeSrc))
		if err != nil {
			return nil, fmt.Errorf("failed to parse Go file %s: %s", logical, err.Error())
		}
		afterSum, err := analyzeGo(logical, []byte(afterSrc))
		if err != nil {
			return nil, fmt.Errorf("failed to parse Go file %s: %s", logical, err.Error())
		}
		cands = append(cands, compareGo(logical, beforeSum, afterSum)...)
	case "python":
		part, err := screenPython(beforeSrc, afterSrc, logical)
		if err != nil {
			return nil, err
		}
		cands = append(cands, part...)
	case "rust":
		cands = append(cands, screenRust(beforeSrc, afterSrc, logical)...)
	case "typescript":
		cands = append(cands, screenTypeScript(beforeSrc, afterSrc, logical)...)
	}
	part, err := screenTestExpectations(beforeSrc, afterSrc, logical, lang)
	if err != nil {
		return nil, err
	}
	cands = append(cands, part...)
	if cands == nil {
		cands = []Candidate{}
	}
	return cands, nil
}

func inferLang(pathHint, explicit string, explicitSet bool) string {
	if explicitSet {
		return strings.ToLower(explicit)
	}
	return extToLang[strings.ToLower(filepath.Ext(pathHint))]
}

func mustRegularFile(path, flag string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%s file not found: %s", flag, path)
	}
	return nil
}

func readText(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes.ToValidUTF8(b, []byte("\uFFFD"))), nil
}

func makeCandidate(file, function, kind, before, after string) Candidate {
	sum := sha256.Sum256([]byte(file + "|" + function + "|" + kind))
	return Candidate{
		ID:       kind + ":" + hex.EncodeToString(sum[:])[:12],
		File:     file,
		Function: function,
		Kind:     kind,
		Before:   before,
		After:    after,
	}
}

func marshalCandidates(cands []Candidate) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cands); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

func joinSortedKinds(cands []Candidate) string {
	seen := map[string]struct{}{}
	var kinds []string
	for _, c := range cands {
		if _, ok := seen[c.Kind]; ok {
			continue
		}
		seen[c.Kind] = struct{}{}
		kinds = append(kinds, c.Kind)
	}
	sort.Strings(kinds)
	return strings.Join(kinds, ", ")
}

func fmtCounter(m map[string]int) string {
	if len(m) == 0 {
		return "{}"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, ", ")
}

func sameCounts(a, b map[string]int) bool {
	seen := map[string]struct{}{}
	for k := range a {
		seen[k] = struct{}{}
	}
	for k := range b {
		seen[k] = struct{}{}
	}
	for k := range seen {
		if a[k] != b[k] {
			return false
		}
	}
	return true
}

func joinOrNone(items []string, sep string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, sep)
}

func sortedStrings(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}
