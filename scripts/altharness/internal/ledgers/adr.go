package ledgers

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RunAdr fails when an ADR adds a supersedes edge that its body never names.
func RunAdr(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	opt, errText := parseAdrArgs(args)
	if errText != "" {
		fmt.Fprint(stderr, errText)
		return 2
	}
	if opt.help {
		fmt.Fprint(stdout, adrUsage)
		return 0
	}
	if !opt.baseSet {
		fmt.Fprintln(stderr, "error: --base is required")
		return 2
	}
	return runAdrGate(ctx, opt.base, opt.dir, stdout, stderr)
}

const adrUsage = `usage: adr_supersede_gate --base REV [--dir PATH]

Fail when an ADR adds a supersedes edge that its body does not reference.
`

type adrOpts struct {
	base    string
	baseSet bool
	dir     string
	help    bool
}

func parseAdrArgs(args []string) (adrOpts, string) {
	opt := adrOpts{dir: "docs/ADR"}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			opt.help = true
			return opt, ""
		case arg == "--base" || arg == "--dir":
			if i+1 >= len(args) {
				return opt, fmt.Sprintf("error: %s requires a value\n", arg)
			}
			i++
			if arg == "--base" {
				opt.base, opt.baseSet = args[i], true
			} else {
				opt.dir = args[i]
			}
		case strings.HasPrefix(arg, "--base="):
			opt.base, opt.baseSet = strings.TrimPrefix(arg, "--base="), true
		case strings.HasPrefix(arg, "--dir="):
			opt.dir = strings.TrimPrefix(arg, "--dir=")
		default:
			return opt, fmt.Sprintf("error: unrecognized arguments: %s\n", arg)
		}
	}
	return opt, ""
}

var (
	adrFilenameRe    = regexp.MustCompile(`^(\d{6})\.md$`)
	bodyWikilinkRe   = regexp.MustCompile(`\[\[(\d{6})(?:\|[^\]\n]*)?\]\]`)
	supersedesKeyRe  = regexp.MustCompile(`^supersedes\s*:\s*(.*)$`)
	supersedesItemRe = regexp.MustCompile(`(?i)^(?:\[\[)?(?:ADR-)?(\d{6})(?:\|[^\]]*)?(?:\]\])?$`)
	blockItemRe      = regexp.MustCompile(`^\s*-\s+(.*)$`)
)

func splitFrontmatter(text string) ([]string, string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return nil, text
	}
	first := strings.TrimRight(strings.TrimPrefix(lines[0], "\uFEFF"), " \t")
	if first != "---" {
		return nil, text
	}
	for idx := 1; idx < len(lines); idx++ {
		if strings.TrimRight(lines[idx], " \t") == "---" {
			body := strings.Join(lines[idx+1:], "\n")
			return lines[1:idx], body
		}
	}
	return nil, text
}

func stripYAMLComment(raw string) string {
	quote := byte(0)
	bracket := 0
	for idx := 0; idx < len(raw); idx++ {
		ch := raw[idx]
		if quote != 0 {
			if ch == quote && (idx == 0 || raw[idx-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '[' {
			bracket++
			continue
		}
		if ch == ']' {
			if bracket > 0 {
				bracket--
			}
			continue
		}
		if ch == '#' && bracket == 0 && (idx == 0 || raw[idx-1] == ' ' || raw[idx-1] == '\t') {
			return raw[:idx]
		}
	}
	return raw
}

func findMatchingBracket(text string, openIdx int) int {
	quote := byte(0)
	depth := 0
	for idx := openIdx; idx < len(text); idx++ {
		ch := text[idx]
		if quote != 0 {
			if ch == quote && (idx == 0 || text[idx-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '[' {
			depth++
			continue
		}
		if ch == ']' {
			depth--
			if depth == 0 {
				return idx
			}
		}
	}
	return -1
}

func splitInlineList(inner string) []string {
	var items []string
	start := 0
	quote := byte(0)
	bracket := 0
	for idx := 0; idx < len(inner); idx++ {
		ch := inner[idx]
		if quote != 0 {
			if ch == quote && (idx == 0 || inner[idx-1] != '\\') {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '[' {
			bracket++
			continue
		}
		if ch == ']' {
			if bracket > 0 {
				bracket--
			}
			continue
		}
		if ch == ',' && bracket == 0 {
			token := strings.TrimSpace(inner[start:idx])
			if token != "" {
				items = append(items, token)
			}
			start = idx + 1
		}
	}
	tail := strings.TrimSpace(inner[start:])
	if tail != "" {
		items = append(items, tail)
	}
	return items
}

func normalizeADRID(token string) string {
	val := strings.TrimSpace(token)
	if len(val) >= 2 {
		if (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
			val = strings.TrimSpace(val[1 : len(val)-1])
		}
	}
	m := supersedesItemRe.FindStringSubmatch(val)
	if m == nil {
		return ""
	}
	return m[1]
}

func parseSupersedes(lines []string) []string {
	var targets []string
	seen := map[string]struct{}{}
	add := func(token string) {
		id := normalizeADRID(token)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		targets = append(targets, id)
	}
	for idx := 0; idx < len(lines); {
		line := lines[idx]
		m := supersedesKeyRe.FindStringSubmatch(line)
		if m == nil {
			idx++
			continue
		}
		rest := strings.TrimSpace(stripYAMLComment(m[1]))
		switch {
		case strings.HasPrefix(rest, "["):
			combined := rest
			closeIdx := findMatchingBracket(combined, 0)
			for closeIdx < 0 && idx+1 < len(lines) {
				idx++
				next := strings.TrimSpace(stripYAMLComment(lines[idx]))
				combined = combined + " " + next
				closeIdx = findMatchingBracket(combined, 0)
			}
			inner := combined[1:]
			if closeIdx >= 0 && closeIdx < len(combined) {
				inner = combined[1:closeIdx]
			}
			for _, item := range splitInlineList(inner) {
				add(item)
			}
			idx++
		case rest == "":
			idx++
			for idx < len(lines) {
				sub := lines[idx]
				stripped := strings.TrimSpace(sub)
				if stripped == "" || strings.HasPrefix(stripped, "#") {
					idx++
					continue
				}
				item := blockItemRe.FindStringSubmatch(sub)
				if item == nil {
					break
				}
				raw := strings.TrimSpace(stripYAMLComment(item[1]))
				if raw != "" {
					add(raw)
				}
				idx++
			}
		default:
			add(rest)
			idx++
		}
	}
	return targets
}

func bodyWikilinkIDs(body string) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, m := range bodyWikilinkRe.FindAllStringSubmatch(body, -1) {
		ids[m[1]] = struct{}{}
	}
	return ids
}

func formatAdrFinding(src, dst string) string {
	return fmt.Sprintf(
		"ADR %s adds supersedes: %s but its body never references [[%s]]. A supersedes edge removes %s from the binding set (docdag query --binding). Name %s in the body with the reason it is replaced, or use depends-on, or drop the edge. See ADR-001011.",
		src, dst, dst, dst, dst,
	)
}

func runAdrGate(ctx context.Context, baseRev, adrDir string, stdout, stderr io.Writer) int {
	info, err := os.Stat(adrDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: ADR directory does not exist: %s\n", adrDir)
		return 2
	}
	if invalidBase(baseRev) {
		fmt.Fprintf(stderr, "error: invalid base revision: %s\n", pyQuote(baseRev))
		return 2
	}
	top := runGit(ctx, adrDir, "rev-parse", "--show-toplevel")
	if top.Missing {
		gitMissing(stderr)
		return 2
	}
	if top.Code != 0 || strings.TrimSpace(top.Stdout) == "" {
		fmt.Fprintf(stderr, "error: %s is not inside a git repository: %s\n", adrDir, top.Stderr)
		return 2
	}
	repoRoot := resolvePath(strings.TrimSpace(top.Stdout))
	rev := runGit(ctx, repoRoot, "rev-parse", "--verify", "--quiet", baseRev+"^{commit}")
	if rev.Missing {
		gitMissing(stderr)
		return 2
	}
	if rev.Code != 0 || strings.TrimSpace(rev.Stdout) == "" {
		fmt.Fprintf(stderr, "error: unknown base revision: %s\n", baseRev)
		return 2
	}
	dirAbs := resolvePath(adrDir)
	relDir, err := filepath.Rel(repoRoot, dirAbs)
	if err != nil || strings.HasPrefix(relDir, "..") {
		fmt.Fprintf(stderr, "error: %s is outside git repository %s\n", adrDir, repoRoot)
		return 2
	}
	relDir = filepath.ToSlash(relDir)
	entries, err := os.ReadDir(adrDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: read ADR directory: %s\n", err.Error())
		return 2
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var findings []string
	superseded := map[string]struct{}{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := adrFilenameRe.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		srcID := m[1]
		path := filepath.Join(adrDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(stderr, "error: read %s: %s\n", path, err.Error())
			return 2
		}
		fm, body := splitFrontmatter(string(data))
		current := parseSupersedes(fm)
		if len(current) == 0 {
			continue
		}
		relFile := relDir + "/" + entry.Name()
		if relDir == "." {
			relFile = entry.Name()
		}
		show := runGit(ctx, repoRoot, "show", baseRev+":"+relFile)
		if show.Missing {
			gitMissing(stderr)
			return 2
		}
		baseSet := map[string]struct{}{}
		if show.Code == 0 {
			baseFM, _ := splitFrontmatter(show.Stdout)
			for _, id := range parseSupersedes(baseFM) {
				baseSet[id] = struct{}{}
			}
		}
		var fresh []string
		for _, dst := range current {
			if _, ok := baseSet[dst]; !ok {
				fresh = append(fresh, dst)
			}
		}
		if len(fresh) == 0 {
			continue
		}
		refs := bodyWikilinkIDs(body)
		for _, dst := range fresh {
			superseded[dst] = struct{}{}
			if _, ok := refs[dst]; !ok {
				findings = append(findings, formatAdrFinding(srcID, dst))
			}
		}
	}
	for _, finding := range findings {
		fmt.Fprintln(stdout, finding)
	}
	ids := make([]string, 0, len(superseded))
	for id := range superseded {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	noun := "ADRs"
	if len(ids) == 1 {
		noun = "ADR"
	}
	idsText := "none"
	if len(ids) > 0 {
		idsText = strings.Join(ids, ", ")
	}
	fmt.Fprintf(stdout, "Summary: %d %s superseded by new edges: %s\n", len(ids), noun, idsText)
	if len(findings) > 0 {
		return 1
	}
	return 0
}
