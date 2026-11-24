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
	"unicode"
	"unicode/utf8"
)

const minDestructiveReason = 20

// RunMigration fails when an added migration performs a destructive change
// without a `-- destructive-ok: <reason>` comment of at least 20 characters.
func RunMigration(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	opt, errText := parseMigrationArgs(args)
	if errText != "" {
		fmt.Fprint(stderr, errText)
		return 2
	}
	if opt.help {
		fmt.Fprint(stdout, migrationUsage)
		return 0
	}
	if !opt.baseSet {
		fmt.Fprintln(stderr, "error: --base is required")
		return 2
	}
	return runMigrationGate(ctx, opt.base, opt.root, stdout, stderr)
}

const migrationUsage = `usage: migration_destructive_gate --base REV [--root PATH]

Fail when an added migration SQL file performs a destructive operation
without a '-- destructive-ok: <reason>' comment.
`

type migrationOpts struct {
	base    string
	baseSet bool
	root    string
	help    bool
}

func parseMigrationArgs(args []string) (migrationOpts, string) {
	opt := migrationOpts{root: "."}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			opt.help = true
			return opt, ""
		case arg == "--base" || arg == "--root":
			if i+1 >= len(args) {
				return opt, fmt.Sprintf("error: %s requires a value\n", arg)
			}
			i++
			if arg == "--base" {
				opt.base, opt.baseSet = args[i], true
			} else {
				opt.root = args[i]
			}
		case strings.HasPrefix(arg, "--base="):
			opt.base, opt.baseSet = strings.TrimPrefix(arg, "--base="), true
		case strings.HasPrefix(arg, "--root="):
			opt.root = strings.TrimPrefix(arg, "--root=")
		default:
			return opt, fmt.Sprintf("error: unrecognized arguments: %s\n", arg)
		}
	}
	return opt, ""
}

func isMigrationSQL(rel string) bool {
	if !strings.HasSuffix(strings.ToLower(rel), ".sql") {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts[:len(parts)-1] {
		if part == "migrations" {
			return true
		}
	}
	return false
}

var destructiveOKRe = regexp.MustCompile(`^--\s*destructive-ok:(.*)$`)
var dollarTagRe = regexp.MustCompile(`^\$(?:[A-Za-z_][A-Za-z0-9_]*)?\$`)

type dollarBlock struct {
	outerStart int
	outerEnd   int
	bodyStart  int
	body       string
}

func scanLineComment(text, out []byte, start int) (int, int) {
	out[start] = ' '
	out[start+1] = ' '
	i := start + 2
	for i < len(text) && text[i] != '\n' {
		out[i] = ' '
		i++
	}
	raw := strings.TrimSpace(string(text[start:i]))
	okAt := -1
	if m := destructiveOKRe.FindStringSubmatch(raw); m != nil && utf8.RuneCountInString(strings.TrimSpace(m[1])) >= minDestructiveReason {
		okAt = start
	}
	return i, okAt
}

func scanBlockComment(text, out []byte, start int) int {
	out[start] = ' '
	out[start+1] = ' '
	i := start + 2
	depth := 1
	for i < len(text) && depth > 0 {
		if text[i] == '/' && i+1 < len(text) && text[i+1] == '*' {
			out[i] = ' '
			out[i+1] = ' '
			depth++
			i += 2
			continue
		}
		if text[i] == '*' && i+1 < len(text) && text[i+1] == '/' {
			out[i] = ' '
			out[i+1] = ' '
			depth--
			i += 2
			continue
		}
		if text[i] != '\n' {
			out[i] = ' '
		}
		i++
	}
	return i
}

func scanDollarBlock(text, out []byte, start int) (int, *dollarBlock) {
	tag := dollarTagRe.Find(text[start:])
	if tag == nil {
		return start + 1, nil
	}
	bodyStart := start + len(tag)
	rest := text[bodyStart:]
	rel := strings.Index(string(rest), string(tag))
	if rel < 0 {
		return bodyStart, nil
	}
	closeIdx := bodyStart + rel
	outerEnd := closeIdx + len(tag)
	for j := start; j < outerEnd; j++ {
		if text[j] != '\n' {
			out[j] = ' '
		}
	}
	return outerEnd, &dollarBlock{
		outerStart: start,
		outerEnd:   outerEnd,
		bodyStart:  bodyStart,
		body:       string(text[bodyStart:closeIdx]),
	}
}

func scanEscapedString(text, out []byte, start int) int {
	i := start + 2
	for i < len(text) {
		if text[i] == '\\' && i+1 < len(text) {
			if text[i] != '\n' {
				out[i] = ' '
			}
			if text[i+1] != '\n' {
				out[i+1] = ' '
			}
			i += 2
			continue
		}
		if text[i] == '\'' {
			if i+1 < len(text) && text[i+1] == '\'' {
				out[i] = ' '
				out[i+1] = ' '
				i += 2
				continue
			}
			return i + 1
		}
		if text[i] != '\n' {
			out[i] = ' '
		}
		i++
	}
	return i
}

func scanSingleQuoted(text, out []byte, start int) int {
	i := start + 1
	for i < len(text) {
		if text[i] == '\'' {
			if i+1 < len(text) && text[i+1] == '\'' {
				out[i] = ' '
				out[i+1] = ' '
				i += 2
				continue
			}
			return i + 1
		}
		if text[i] != '\n' {
			out[i] = ' '
		}
		i++
	}
	return i
}

func scanDoubleQuoted(text []byte, start int) int {
	i := start + 1
	for i < len(text) {
		if text[i] == '"' {
			if i+1 < len(text) && text[i+1] == '"' {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return i
}

func isIdentByte(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_' || c == '$'
}

func scanSQL(text string) (string, []int, []int, []dollarBlock) {
	raw := []byte(text)
	out := append([]byte(nil), raw...)
	var semicolons []int
	var okOffsets []int
	var blocks []dollarBlock
	n := len(raw)
	for i := 0; i < n; {
		ch := raw[i]
		if ch == '-' && i+1 < n && raw[i+1] == '-' {
			next, okAt := scanLineComment(raw, out, i)
			if okAt >= 0 {
				okOffsets = append(okOffsets, okAt)
			}
			i = next
			continue
		}
		if ch == '/' && i+1 < n && raw[i+1] == '*' {
			i = scanBlockComment(raw, out, i)
			continue
		}
		if ch == '$' && dollarTagRe.Match(raw[i:]) {
			next, block := scanDollarBlock(raw, out, i)
			if block != nil {
				blocks = append(blocks, *block)
			}
			i = next
			continue
		}
		if (ch == 'e' || ch == 'E') && i+1 < n && raw[i+1] == '\'' && (i == 0 || !isIdentByte(raw[i-1])) {
			// Python isalnum() is broader than ASCII. Treat a non-ASCII previous
			// byte as "alphanumeric" only when it is a UTF-8 continuation or lead,
			// matching unicode.IsLetter/IsDigit on the previous rune.
			if i > 0 && raw[i-1] >= 0x80 {
				r, _ := utf8.DecodeLastRune(raw[:i])
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					i++
					continue
				}
			}
			i = scanEscapedString(raw, out, i)
			continue
		}
		if ch == '\'' {
			i = scanSingleQuoted(raw, out, i)
			continue
		}
		if ch == '"' {
			i = scanDoubleQuoted(raw, i)
			continue
		}
		if ch == ';' {
			semicolons = append(semicolons, i)
		}
		i++
	}
	return string(out), semicolons, okOffsets, blocks
}

var (
	whereParenRe    = regexp.MustCompile(`(?i)\(|\)|\bWHERE\b`)
	dropTableRe     = regexp.MustCompile(`(?i)\bDROP\s+TABLE\b`)
	dropSchemaRe    = regexp.MustCompile(`(?i)\bDROP\s+SCHEMA\b`)
	dropDatabaseRe  = regexp.MustCompile(`(?i)\bDROP\s+DATABASE\b`)
	dropPartitionRe = regexp.MustCompile(`(?i)\bDROP\s+PARTITION\b`)
	dropCascadeRes  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bDROP\b[^,;()]*?\bCASCADE\b`),
		regexp.MustCompile(`(?i)\bDROP\b[^,;()]*?\([^)]*\)[^,;()]*?\bCASCADE\b`),
	}
	onDeletePrefixRe = regexp.MustCompile(`(?i)\bON\s+DELETE\s+$`)
	alterTableRe     = regexp.MustCompile(`(?i)\bALTER\s+TABLE\b`)
	dropKWRe         = regexp.MustCompile(`(?i)\bDROP\s+`)
	dropColForbidden = regexp.MustCompile(`(?i)^(?:CONSTRAINT\b|DEFAULT\b|NOT\s+NULL\b|EXPRESSION\b|IDENTITY\b|INDEX\b|PARTITION\b|PRIMARY\s+KEY\b|PROJECTION\b)`)
	dropColRestRe    = regexp.MustCompile(`(?i)^(?:COLUMN\s+)?(?:IF\s+EXISTS\s+)?(?:[A-Za-z_][A-Za-z0-9_]*|"[^"]+")`)
	alterUsingRe     = regexp.MustCompile(`(?is)\bALTER\s+(?:COLUMN\s+)?(?:[A-Za-z_][A-Za-z0-9_]*|"[^"]+")\s+(?:SET\s+DATA\s+)?TYPE\b.*?\bUSING\s*\(?\s*NULL\b`)
	alterUsingSkipRe = regexp.MustCompile(`(?i)^\bALTER\s+(?:COLUMN\s+)?(?:TABLE|INDEX|VIEW|SCHEMA|DATABASE|TYPE|SEQUENCE)\b`)
	alterModifyTTLRe = regexp.MustCompile(`(?is)\bALTER\s+TABLE\b.*?\bMODIFY\s+TTL\b`)
	alterCHDeleteRe  = regexp.MustCompile(`(?i)\bALTER\s+TABLE\s+(?:IF\s+EXISTS\s+)?(?:[A-Za-z_][A-Za-z0-9_]*|"[^"]+")(?:\.(?:[A-Za-z_][A-Za-z0-9_]*|"[^"]+"))?(?:\s+ON\s+CLUSTER\s+(?:[A-Za-z_][A-Za-z0-9_]*|"[^"]+"))?\s+DELETE\b`)
	truncateRe       = regexp.MustCompile(`(?i)\bTRUNCATE\b`)
	deleteFromRe     = regexp.MustCompile(`(?i)\bDELETE\s+FROM\b`)
	updateSetNullRe  = regexp.MustCompile(`(?is)\bUPDATE\b.*?\bSET\b.*?=\s*\(?\s*NULL\b`)
)

func parenDepthAt(text string, pos int) int {
	depth := 0
	if pos > len(text) {
		pos = len(text)
	}
	for _, r := range text[:pos] {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return depth
}

func hasWhereAfter(stmt string, kwStart, kwLen int) bool {
	kwDepth := parenDepthAt(stmt, kwStart)
	depth := kwDepth
	for _, loc := range whereParenRe.FindAllStringIndex(stmt[kwStart+kwLen:], -1) {
		tok := stmt[kwStart+kwLen+loc[0] : kwStart+kwLen+loc[1]]
		switch strings.ToUpper(tok) {
		case "(":
			depth++
		case ")":
			depth--
		default:
			if depth == kwDepth {
				return true
			}
		}
	}
	return false
}

type sqlFinding struct {
	line int
	kind string
}

func statementFindings(masked string, stmtStart, stmtEnd, lineOffset int) []sqlFinding {
	stmt := masked[stmtStart:stmtEnd]
	if strings.TrimSpace(stmt) == "" {
		return nil
	}
	var findings []sqlFinding
	matchedDrop := map[int]struct{}{}
	lineOf := func(rel int) int {
		return lineOffset + 1 + strings.Count(masked[:stmtStart+rel], "\n")
	}
	for _, item := range []struct {
		re   *regexp.Regexp
		kind string
	}{
		{dropTableRe, "DROP TABLE"},
		{dropSchemaRe, "DROP SCHEMA"},
		{dropDatabaseRe, "DROP DATABASE"},
		{dropPartitionRe, "DROP PARTITION"},
	} {
		for _, loc := range item.re.FindAllStringIndex(stmt, -1) {
			matchedDrop[loc[0]] = struct{}{}
			findings = append(findings, sqlFinding{lineOf(loc[0]), item.kind})
		}
	}
	if alterTableRe.MatchString(stmt) {
		for _, loc := range dropKWRe.FindAllStringIndex(stmt, -1) {
			rest := stmt[loc[1]:]
			if dropColForbidden.MatchString(rest) {
				continue
			}
			m := dropColRestRe.FindStringIndex(rest)
			if m == nil {
				continue
			}
			matchedDrop[loc[0]] = struct{}{}
			findings = append(findings, sqlFinding{lineOf(loc[0]), "DROP COLUMN"})
		}
		for _, loc := range alterUsingRe.FindAllStringIndex(stmt, -1) {
			frag := stmt[loc[0]:]
			if alterUsingSkipRe.MatchString(frag) {
				continue
			}
			findings = append(findings, sqlFinding{lineOf(loc[0]), "ALTER COLUMN ... TYPE ... USING NULL"})
		}
		for _, loc := range alterModifyTTLRe.FindAllStringIndex(stmt, -1) {
			findings = append(findings, sqlFinding{lineOf(loc[0]), "MODIFY TTL"})
		}
		for _, loc := range alterCHDeleteRe.FindAllStringIndex(stmt, -1) {
			findings = append(findings, sqlFinding{lineOf(loc[0]), "ALTER TABLE ... DELETE"})
		}
	}
	seenCascade := map[int]struct{}{}
	for _, re := range dropCascadeRes {
		for _, loc := range re.FindAllStringIndex(stmt, -1) {
			cascadeAt := loc[1] - len("CASCADE")
			if cascadeAt < loc[0] {
				continue
			}
			if !strings.EqualFold(stmt[cascadeAt:loc[1]], "CASCADE") {
				// The match is case-insensitive; locate the keyword at the end.
				tail := stmt[loc[0]:loc[1]]
				idx := strings.LastIndex(strings.ToLower(tail), "cascade")
				if idx < 0 {
					continue
				}
				cascadeAt = loc[0] + idx
			}
			if onDeletePrefixRe.MatchString(stmt[:cascadeAt]) {
				continue
			}
			if _, ok := seenCascade[loc[0]]; ok {
				continue
			}
			seenCascade[loc[0]] = struct{}{}
			if _, ok := matchedDrop[loc[0]]; ok {
				continue
			}
			findings = append(findings, sqlFinding{lineOf(loc[0]), "DROP ... CASCADE"})
		}
	}
	for _, loc := range truncateRe.FindAllStringIndex(stmt, -1) {
		findings = append(findings, sqlFinding{lineOf(loc[0]), "TRUNCATE"})
	}
	for _, loc := range deleteFromRe.FindAllStringIndex(stmt, -1) {
		if !hasWhereAfter(stmt, loc[0], len("DELETE")) {
			findings = append(findings, sqlFinding{lineOf(loc[0]), "DELETE FROM without WHERE"})
		}
	}
	for _, loc := range updateSetNullRe.FindAllStringIndex(stmt, -1) {
		if !hasWhereAfter(stmt, loc[0], len("UPDATE")) {
			findings = append(findings, sqlFinding{lineOf(loc[0]), "UPDATE ... SET ... = NULL without WHERE"})
		}
	}
	return findings
}

func scanFindings(text string, lineOffset int, inheritedOK, respectOK bool) []sqlFinding {
	masked, semicolons, okOffsets, blocks := scanSQL(text)
	boundaries := []int{0}
	for _, idx := range semicolons {
		boundaries = append(boundaries, idx+1)
	}
	boundaries = append(boundaries, len(text))
	var findings []sqlFinding
	for pos := 0; pos < len(boundaries)-1; pos++ {
		stmtStart := boundaries[pos]
		stmtEnd := boundaries[pos+1]
		stmtOK := inheritedOK
		if !stmtOK {
			for _, offset := range okOffsets {
				if stmtStart <= offset && offset < stmtEnd {
					stmtOK = true
					break
				}
			}
		}
		if !(respectOK && stmtOK) {
			findings = append(findings, statementFindings(masked, stmtStart, stmtEnd, lineOffset)...)
		}
		for _, block := range blocks {
			if stmtStart <= block.outerStart && block.outerStart < stmtEnd {
				nested := lineOffset + strings.Count(text[:block.bodyStart], "\n")
				findings = append(findings, scanFindings(block.body, nested, stmtOK, respectOK)...)
			}
		}
	}
	return findings
}

func findDestructiveStatements(text string) []sqlFinding {
	return scanFindings(text, 0, false, false)
}

func checkMigrationText(rel, text string) []string {
	var out []string
	for _, finding := range scanFindings(text, 0, false, true) {
		out = append(out, fmt.Sprintf("M1 %s:%d: %s without a destructive-ok comment", rel, finding.line, finding.kind))
	}
	return out
}

func runMigrationGate(ctx context.Context, baseRev, rootDir string, stdout, stderr io.Writer) int {
	info, err := os.Stat(rootDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: repository root does not exist: %s\n", rootDir)
		return 2
	}
	if invalidBase(baseRev) {
		fmt.Fprintf(stderr, "error: invalid base revision: %s\n", pyQuote(baseRev))
		return 2
	}
	top := runGit(ctx, rootDir, "rev-parse", "--show-toplevel")
	if top.Missing {
		gitMissing(stderr)
		return 2
	}
	if top.Code != 0 || strings.TrimSpace(top.Stdout) == "" {
		fmt.Fprintf(stderr, "error: %s is not inside a git repository: %s\n", rootDir, top.Stderr)
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
	mb := runGit(ctx, repoRoot, "merge-base", baseRev, "HEAD")
	if mb.Missing {
		gitMissing(stderr)
		return 2
	}
	mergeBase := strings.TrimSpace(mb.Stdout)
	if mb.Code != 0 || mergeBase == "" {
		fmt.Fprintf(stderr, "error: cannot compute merge-base between %s and HEAD: %s\n", baseRev, mb.Stderr)
		return 2
	}
	lsCur := runGit(ctx, repoRoot, "ls-files", "-z")
	if lsCur.Missing {
		gitMissing(stderr)
		return 2
	}
	if lsCur.Code != 0 {
		fmt.Fprintf(stderr, "error: git ls-files failed: %s\n", lsCur.Stderr)
		return 2
	}
	lsBase := runGit(ctx, repoRoot, "ls-tree", "-r", "-z", "--name-only", mergeBase)
	if lsBase.Missing {
		gitMissing(stderr)
		return 2
	}
	if lsBase.Code != 0 {
		fmt.Fprintf(stderr, "error: git ls-tree failed: %s\n", lsBase.Stderr)
		return 2
	}
	baseSet := map[string]struct{}{}
	for _, p := range nullSplit(lsBase.Stdout) {
		baseSet[p] = struct{}{}
	}
	var added []string
	for _, p := range nullSplit(lsCur.Stdout) {
		if _, ok := baseSet[p]; ok {
			continue
		}
		if isMigrationSQL(p) {
			added = append(added, p)
		}
	}
	sort.Strings(added)
	var findings []string
	checked := 0
	for _, rel := range added {
		full := filepath.Join(repoRoot, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil || info.IsDir() {
			continue
		}
		checked++
		data, err := os.ReadFile(full)
		if err != nil {
			fmt.Fprintf(stderr, "error: read %s: %s\n", rel, err.Error())
			return 2
		}
		findings = append(findings, checkMigrationText(rel, string(data))...)
	}
	if len(findings) > 0 {
		for _, finding := range findings {
			fmt.Fprintln(stdout, finding)
		}
		fmt.Fprintln(stdout, "\nAdd a '-- destructive-ok: <at least 20 chars explaining the backfill command, completion query, or why no data is lost>' comment line directly above or inside each flagged statement.")
		return 1
	}
	if checked == 0 {
		fmt.Fprintf(stdout, "OK: no migration files added relative to merge base %s\n", mergeBase)
		return 0
	}
	fmt.Fprintf(stdout, "OK: checked %d added migration file(s) relative to merge base %s, 0 findings\n", checked, mergeBase)
	return 0
}
