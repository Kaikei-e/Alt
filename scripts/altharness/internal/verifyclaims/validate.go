package verifyclaims

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ValidateResult holds the structured result of report or template validation.
type ValidateResult struct {
	Valid                bool    `json:"valid"`
	Path                 string  `json:"path"`
	ClaimsChecked        int     `json:"claims_checked"`
	EvidencePathsChecked int     `json:"evidence_paths_checked"`
	Errors               []Issue `json:"errors"`
}

var (
	tableSeparatorRE = regexp.MustCompile(`^\s*\|?(?:\s*:?-{2,}:?\s*\|)+\s*:?-{2,}:?\s*\|?\s*$`)
	mdLinkRE         = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
	backtickSpanRE   = regexp.MustCompile("`([^`]+)`")
	labelPathRE      = regexp.MustCompile(`(?i)(?:保存先|証拠パス|evidence(?:\s+path)?|output(?:\s+path)?|artifact|path)\s*[:：]\s*([^\s|,;）)]+)`)
	envPrefixRE      = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=\S+\s+`)
)

var placeholderTokens = map[string]struct{}{
	"": {}, "tbd": {}, "n/a": {}, "na": {}, "-": {}, "--": {}, "—": {},
	"todo": {}, "none": {}, "null": {}, "なし": {}, "無し": {},
	"未記入": {}, "未定": {}, "xxx": {}, "...": {},
}

var blanketClaimPhrases = []string{
	"影響ゼロ",
	"影響なし",
	"zero impact",
	"no runtime impact",
	"no impact",
	"動作変更なし",
	"挙動変更なし",
	"no behavior change",
	"behavior unchanged",
}

var broadScopePhrases = []string{
	"本番サービス",
	"全サービス",
	"全モジュール",
	"全体",
	"all services",
	"all modules",
	"production",
}

var unScopedBlanketPhrases = []string{
	"本番サービス実行時挙動への影響ゼロ",
	"本番サービスへの影響ゼロ",
	"全サービスへの影響ゼロ",
	"全サービス影響ゼロ",
	"zero impact on production",
	"zero impact across all",
}

var runnableCommandPrefixes = []string{
	"./",
	"go ",
	"cargo ",
	"wasm-tools ",
	"uv ",
	"bun ",
	"pytest ",
	"python3 ",
	"python ",
	"bash ",
	"sh ",
	"git ",
	"sha256sum ",
	"alt-harness ",
	"ast-grep ",
	"make ",
	"deno ",
	"docker ",
	"curl ",
	"rg ",
	"docdag ",
	"diff ",
	"cmp ",
}

var knownEvidenceExts = []string{
	".md", ".json", ".txt", ".log", ".wit", ".tsv", ".toml",
	".yaml", ".yml", ".out", ".diff", ".csv", ".wasm", ".sarif",
	".xml", ".html", ".pdf", ".png", ".svg",
}

var nonEvidenceSourceExts = map[string]struct{}{
	".go": {}, ".rs": {}, ".py": {}, ".sh": {},
}

var (
	htmlAnchorIDRE = regexp.MustCompile(`(?i)\bid\s*=\s*["']([^"']+)["']`)
	mdHeadingRE    = regexp.MustCompile(`^\s*#{1,6}\s+(.+?)\s*$`)
	htmlTagStripRE = regexp.MustCompile(`<[^>]+>`)
)

type mdTableRow struct {
	lineNo int
	cells  []string
}

type claimTable struct {
	headerLine  int
	claimCol    int
	commandCol  int // -1 when combined into evidence column
	evidenceCol int
	colCount    int
	rows        []mdTableRow
}

func normalizeLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

type fenceState struct {
	ch     byte
	length int
}

func (f fenceState) active() bool { return f.length >= 3 }

func parseFenceRun(line string) (ch byte, runLen int, rest string, ok bool) {
	trimmed := strings.TrimLeft(line, " \t")
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, "", false
	}
	ch = trimmed[0]
	n := 0
	for n < len(trimmed) && trimmed[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0, "", false
	}
	return ch, n, strings.TrimSpace(trimmed[n:]), true
}

func toggleFence(cur fenceState, line string) (fenceState, bool) {
	ch, runLen, rest, ok := parseFenceRun(line)
	if !ok {
		return cur, false
	}
	if !cur.active() {
		// Opening backtick fence must not contain backticks in its info string per CommonMark.
		if ch == '`' && strings.ContainsRune(rest, '`') {
			return cur, false
		}
		return fenceState{ch: ch, length: runLen}, true
	}
	// Closing fence must match char, have length >= opening length, and no info string.
	if ch == cur.ch && runLen >= cur.length && rest == "" {
		return fenceState{}, true
	}
	return cur, false
}

func splitPipeRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	s = strings.TrimSuffix(s, "|")
	var cells []string
	var cur strings.Builder
	inCode := false
	escaped := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if escaped {
			cur.WriteByte(ch)
			escaped = false
			continue
		}
		if ch == '\\' && !inCode {
			escaped = true
			continue
		}
		if ch == '`' {
			inCode = !inCode
			cur.WriteByte(ch)
			continue
		}
		if ch == '|' && !inCode {
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	return cells
}

func classifyHeader(cells []string) (claimCol, commandCol, evidenceCol int, ok bool) {
	claimCol, commandCol, evidenceCol = -1, -1, -1
	for i, raw := range cells {
		c := strings.ToLower(strings.TrimSpace(raw))
		if claimCol < 0 && (strings.Contains(c, "主張") || strings.Contains(c, "claim")) {
			claimCol = i
			continue
		}
		if evidenceCol < 0 && (strings.Contains(c, "証拠") || strings.Contains(c, "evidence")) {
			evidenceCol = i
			continue
		}
		if commandCol < 0 && (strings.Contains(c, "コマンド") || strings.Contains(c, "command")) {
			commandCol = i
		}
	}
	return claimCol, commandCol, evidenceCol, claimCol >= 0 && evidenceCol >= 0
}

func parseClaimTables(lines []string) ([]claimTable, map[int]bool, []Issue) {
	var tables []claimTable
	inClaimRowLine := map[int]bool{}
	var issues []Issue

	var fence fenceState
	fenceOpenLine := 0
	for i := 0; i < len(lines); i++ {
		if nextFence, toggled := toggleFence(fence, lines[i]); toggled {
			if !fence.active() && nextFence.active() {
				fenceOpenLine = i + 1
			}
			fence = nextFence
			continue
		}
		if fence.active() {
			continue
		}
		if !strings.Contains(lines[i], "|") || i+1 >= len(lines) {
			continue
		}
		if !tableSeparatorRE.MatchString(lines[i+1]) {
			continue
		}
		headerCells := splitPipeRow(lines[i])
		claimCol, cmdCol, evCol, isClaim := classifyHeader(headerCells)
		if !isClaim {
			continue
		}
		tbl := claimTable{
			headerLine:  i + 1,
			claimCol:    claimCol,
			commandCol:  cmdCol,
			evidenceCol: evCol,
			colCount:    len(headerCells),
		}
		inClaimRowLine[i+1] = true
		inClaimRowLine[i+2] = true
		j := i + 2
		for j < len(lines) {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" || !strings.Contains(trimmed, "|") {
				break
			}
			if _, _, _, isF := parseFenceRun(trimmed); isF {
				break
			}
			cells := splitPipeRow(lines[j])
			if len(cells) != tbl.colCount {
				issues = append(issues, Issue{
					Code:    "MALFORMED_CLAIM_TABLE",
					Path:    fmt.Sprintf("line:%d", j+1),
					Message: fmt.Sprintf("claim table row has %d column(s), header declared %d", len(cells), tbl.colCount),
				})
			} else {
				tbl.rows = append(tbl.rows, mdTableRow{lineNo: j + 1, cells: cells})
				inClaimRowLine[j+1] = true
			}
			j++
		}
		tables = append(tables, tbl)
		i = j - 1
	}
	if fence.active() {
		issues = append(issues, Issue{
			Code:    "UNCLOSED_CODE_FENCE",
			Path:    fmt.Sprintf("line:%d", fenceOpenLine),
			Message: "unclosed fenced code block hides trailing report content from validation",
		})
	}
	return tables, inClaimRowLine, issues
}

func isPlaceholderCell(s string) bool {
	cleaned := strings.TrimSpace(s)
	cleaned = strings.Trim(cleaned, "`*_~")
	cleaned = strings.TrimSpace(cleaned)
	_, ok := placeholderTokens[strings.ToLower(cleaned)]
	return ok
}

func matchBlanketPhrase(text string) string {
	lower := strings.ToLower(text)
	for _, phrase := range blanketClaimPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return phrase
		}
	}
	return ""
}

func isUnscopedBlanketClaim(claim string) bool {
	lower := strings.ToLower(strings.TrimSpace(claim))
	for _, phrase := range unScopedBlanketPhrases {
		if strings.Contains(lower, strings.ToLower(phrase)) {
			return true
		}
	}
	trimmed := strings.Trim(lower, "。．.!！`*_ ")
	for _, phrase := range blanketClaimPhrases {
		if trimmed == strings.ToLower(phrase) {
			return true
		}
	}
	if matchBlanketPhrase(lower) != "" {
		for _, scope := range broadScopePhrases {
			if strings.Contains(lower, strings.ToLower(scope)) {
				return true
			}
		}
	}
	return false
}

func looksLikeRunnableCommand(candidate string) bool {
	s := strings.TrimSpace(candidate)
	if s == "" {
		return false
	}
	// Unwrap optional outer subshell parentheses like `(cd dir && cargo clippy ...)`.
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	// Strip leading `cd <dir> && ` prefix before checking the actual command.
	if strings.HasPrefix(s, "cd ") {
		if idx := strings.Index(s, "&&"); idx >= 0 {
			s = strings.TrimSpace(s[idx+2:])
		}
	}
	for envPrefixRE.MatchString(s) {
		s = strings.TrimSpace(envPrefixRE.ReplaceAllString(s, ""))
	}
	for _, pref := range runnableCommandPrefixes {
		if strings.HasPrefix(s, pref) {
			return true
		}
	}
	return false
}

func hasRunnableCommand(text string) bool {
	for _, m := range backtickSpanRE.FindAllStringSubmatch(text, -1) {
		if looksLikeRunnableCommand(m[1]) {
			return true
		}
	}
	return false
}

func looksLikePathToken(token string) bool {
	t := strings.TrimSpace(token)
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
		return false
	}
	if strings.ContainsAny(t, " \t\r\n") {
		return false
	}
	// Always capture traversal / absolute / backslash tokens so security checks reject them.
	if strings.HasPrefix(t, "/") || strings.Contains(t, "..") || strings.Contains(t, `\`) {
		return true
	}
	if strings.HasPrefix(t, "-") {
		return false
	}
	base, _, _ := strings.Cut(t, "#")
	if base == "" {
		return false
	}
	if strings.Contains(base, "/") {
		return true
	}
	lower := strings.ToLower(base)
	for _, ext := range knownEvidenceExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func extractEvidencePaths(evidenceCell string) []string {
	var paths []string
	seen := map[string]struct{}{}
	add := func(raw string) {
		p := strings.TrimSpace(raw)
		p = strings.Trim(p, "`\"'<>")
		if p == "" || strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}

	// 1. Markdown links: [label](target)
	for _, m := range mdLinkRE.FindAllStringSubmatch(evidenceCell, -1) {
		add(m[1])
	}

	// 2. Backtick spans that are single-token path references (never runnable command spans).
	for _, loc := range backtickSpanRE.FindAllStringSubmatchIndex(evidenceCell, -1) {
		span := strings.TrimSpace(evidenceCell[loc[2]:loc[3]])
		if looksLikeRunnableCommand(span) && !strings.Contains(span, "..") {
			continue
		}
		if !looksLikePathToken(span) {
			continue
		}
		add(span)
	}

	// 3. Bare tokens explicitly introduced after 保存先:/Evidence:/Path:.
	for _, m := range labelPathRE.FindAllStringSubmatch(evidenceCell, -1) {
		tok := strings.Trim(m[1], "`\"'<>")
		if looksLikePathToken(tok) {
			add(tok)
		}
	}
	return paths
}

func markdownAnchorExists(filePath, fragment string) bool {
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(fragment))
	if want == "" {
		return true
	}
	content := string(raw)
	for _, m := range htmlAnchorIDRE.FindAllStringSubmatch(content, -1) {
		if strings.ToLower(strings.TrimSpace(m[1])) == want {
			return true
		}
	}
	for _, line := range normalizeLines(content) {
		m := mdHeadingRE.FindStringSubmatch(line)
		if len(m) < 2 {
			continue
		}
		heading := htmlTagStripRE.ReplaceAllString(m[1], "")
		if slugifyHeading(heading) == want {
			return true
		}
	}
	return false
}

func slugifyHeading(heading string) string {
	s := strings.ToLower(strings.TrimSpace(heading))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r == ' ' || r == '\t' || r == '-':
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r >= 0x80:
			b.WriteRune(r)
			prevDash = false
		}
	}
	return strings.TrimRight(b.String(), "-")
}

func validateReport(reportPath, repoRoot string) (ValidateResult, error) {
	raw, err := os.ReadFile(reportPath)
	if err != nil {
		return ValidateResult{}, err
	}
	res := ValidateResult{
		Path:   reportPath,
		Errors: []Issue{},
	}
	if !utf8.Valid(raw) {
		res.Errors = append(res.Errors, Issue{
			Code:    "INVALID_UTF8",
			Path:    reportPath,
			Message: "report must be valid UTF-8",
		})
		return res, nil
	}
	lines := normalizeLines(string(raw))
	tables, claimRowLines, parseIssues := parseClaimTables(lines)
	res.Errors = append(res.Errors, parseIssues...)

	if len(tables) == 0 {
		res.Errors = append(res.Errors, Issue{
			Code:    "MISSING_CLAIM_TABLE",
			Path:    reportPath,
			Message: "implementation report must contain a Claim/Evidence Markdown table (主張 / 証拠 or Claim / Evidence)",
		})
	}

	// Scan prose outside claim table rows and fenced code blocks for blanket claims.
	var fence fenceState
	for idx, line := range lines {
		lineNo := idx + 1
		if nextFence, toggled := toggleFence(fence, line); toggled {
			fence = nextFence
			continue
		}
		if fence.active() || claimRowLines[lineNo] {
			continue
		}
		if phrase := matchBlanketPhrase(line); phrase != "" {
			res.Errors = append(res.Errors, Issue{
				Code:    "BLANKET_UNBACKED_CLAIM",
				Path:    fmt.Sprintf("line:%d", lineNo),
				Message: fmt.Sprintf("blanket claim %q is forbidden outside per-change claim/evidence table rows", phrase),
			})
		}
	}

	reportResolved := ""
	if absRep, aerr := filepath.Abs(reportPath); aerr == nil {
		if rRep, rerr := filepath.EvalSymlinks(absRep); rerr == nil {
			reportResolved = rRep
		} else {
			reportResolved = absRep
		}
	}

	for _, tbl := range tables {
		if len(tbl.rows) == 0 {
			res.Errors = append(res.Errors, Issue{
				Code:    "MISSING_CLAIM_ROWS",
				Path:    fmt.Sprintf("line:%d", tbl.headerLine),
				Message: "claim/evidence table must contain at least one claim row",
			})
			continue
		}
		for _, row := range tbl.rows {
			res.ClaimsChecked++
			loc := fmt.Sprintf("line:%d", row.lineNo)
			claimCell := row.cells[tbl.claimCol]
			evidenceCell := row.cells[tbl.evidenceCol]
			cmdSource := evidenceCell
			if tbl.commandCol >= 0 {
				cmdSource = row.cells[tbl.commandCol]
			}

			if isPlaceholderCell(claimCell) {
				res.Errors = append(res.Errors, Issue{
					Code:    "PLACEHOLDER_CELL",
					Path:    loc,
					Message: fmt.Sprintf("claim cell must not be empty or a placeholder (%q)", claimCell),
				})
			}
			if isPlaceholderCell(evidenceCell) {
				res.Errors = append(res.Errors, Issue{
					Code:    "PLACEHOLDER_CELL",
					Path:    loc,
					Message: fmt.Sprintf("evidence cell must not be empty or a placeholder (%q)", evidenceCell),
				})
				continue
			}
			if tbl.commandCol >= 0 && isPlaceholderCell(cmdSource) {
				res.Errors = append(res.Errors, Issue{
					Code:    "PLACEHOLDER_CELL",
					Path:    loc,
					Message: fmt.Sprintf("command cell must not be empty or a placeholder (%q)", cmdSource),
				})
			}

			cmdOK := hasRunnableCommand(cmdSource)
			if !cmdOK {
				res.Errors = append(res.Errors, Issue{
					Code:    "MISSING_REPRO_COMMAND",
					Path:    loc,
					Message: "evidence row must include a re-executable verification command (not proxy text alone)",
				})
			}

			paths := extractEvidencePaths(evidenceCell)
			validPaths := 0
			if len(paths) == 0 {
				res.Errors = append(res.Errors, Issue{
					Code:    "MISSING_EVIDENCE_PATH",
					Path:    loc,
					Message: "evidence row must reference at least one repo-relative evidence file path",
				})
			} else {
				for _, rawP := range paths {
					res.EvidencePathsChecked++
					filePart, fragPart, _ := strings.Cut(rawP, "#")
					full, unsafeReason, err := resolveSafeRepoPath(repoRoot, filePart)
					if err != nil {
						res.Errors = append(res.Errors, Issue{
							Code:    "UNSAFE_EVIDENCE_PATH",
							Path:    loc,
							Message: fmt.Sprintf("evidence path %q rejected: %s", rawP, unsafeReason),
						})
						continue
					}
					ext := strings.ToLower(filepath.Ext(filePart))
					if _, isSrc := nonEvidenceSourceExts[ext]; isSrc {
						res.Errors = append(res.Errors, Issue{
							Code:    "NON_EVIDENCE_ARTIFACT",
							Path:    loc,
							Message: fmt.Sprintf("source/script file %q is not an evidence output artifact", filePart),
						})
						continue
					}
					st, statErr := os.Lstat(full)
					if statErr != nil || !st.Mode().IsRegular() {
						res.Errors = append(res.Errors, Issue{
							Code:    "MISSING_EVIDENCE_FILE",
							Path:    loc,
							Message: fmt.Sprintf("referenced evidence file does not exist as a regular file: %q", filePart),
						})
						continue
					}
					if st.Size() == 0 {
						res.Errors = append(res.Errors, Issue{
							Code:    "EMPTY_EVIDENCE_FILE",
							Path:    loc,
							Message: fmt.Sprintf("referenced evidence file is 0 bytes: %q", filePart),
						})
						continue
					}
					fullResolved := full
					if rf, rerr := filepath.EvalSymlinks(full); rerr == nil {
						fullResolved = rf
					}
					if reportResolved != "" && fullResolved == reportResolved && strings.TrimSpace(fragPart) == "" {
						res.Errors = append(res.Errors, Issue{
							Code:    "SELF_REFERENTIAL_EVIDENCE",
							Path:    loc,
							Message: fmt.Sprintf("self-referential evidence path %q requires a verified #anchor to an evidence log section", rawP),
						})
						continue
					}
					if strings.TrimSpace(fragPart) != "" && ext == ".md" && !markdownAnchorExists(full, fragPart) {
						res.Errors = append(res.Errors, Issue{
							Code:    "MISSING_EVIDENCE_ANCHOR",
							Path:    loc,
							Message: fmt.Sprintf("referenced Markdown anchor #%s not found in %q", fragPart, filePart),
						})
						continue
					}
					validPaths++
				}
			}

			if isUnscopedBlanketClaim(claimCell) {
				res.Errors = append(res.Errors, Issue{
					Code:    "BLANKET_UNBACKED_CLAIM",
					Path:    loc,
					Message: fmt.Sprintf("global blanket claim %q must be split into scoped per-change claims", claimCell),
				})
			} else if phrase := matchBlanketPhrase(claimCell); phrase != "" && (!cmdOK || validPaths == 0) {
				res.Errors = append(res.Errors, Issue{
					Code:    "BLANKET_UNBACKED_CLAIM",
					Path:    loc,
					Message: fmt.Sprintf("claim containing %q requires both a re-executable command and an existing evidence path", phrase),
				})
			}
		}
	}

	res.Valid = len(res.Errors) == 0
	return res, nil
}

func checkTemplate(repoRoot string) ValidateResult {
	res := ValidateResult{
		Path:   filepath.Join(repoRoot, filepath.FromSlash(templateRelPath)),
		Errors: []Issue{},
	}
	full, unsafeReason, err := resolveSafeRepoPath(repoRoot, templateRelPath)
	if err != nil {
		res.Errors = append(res.Errors, Issue{
			Code:    "UNSAFE_TEMPLATE_PATH",
			Path:    templateRelPath,
			Message: unsafeReason,
		})
		return res
	}
	st, err := os.Lstat(full)
	if err != nil || !st.Mode().IsRegular() {
		res.Errors = append(res.Errors, Issue{
			Code:    "MISSING_TEMPLATE",
			Path:    templateRelPath,
			Message: "implementation report template is missing",
		})
		return res
	}
	raw, err := os.ReadFile(full)
	if err != nil {
		res.Errors = append(res.Errors, Issue{
			Code:    "UNREADABLE_TEMPLATE",
			Path:    templateRelPath,
			Message: err.Error(),
		})
		return res
	}
	if !utf8.Valid(raw) {
		res.Errors = append(res.Errors, Issue{
			Code:    "INVALID_UTF8",
			Path:    templateRelPath,
			Message: "template must be valid UTF-8",
		})
		return res
	}
	text := string(raw)
	lines := normalizeLines(text)

	hasTitle := false
	hasOverview := false
	hasClaimsHeading := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			hasTitle = true
		}
		if strings.HasPrefix(trimmed, "## ") && (strings.Contains(trimmed, "変更概要") || strings.Contains(strings.ToLower(trimmed), "summary")) {
			hasOverview = true
		}
		if strings.HasPrefix(trimmed, "## ") && ((strings.Contains(trimmed, "主張") && strings.Contains(trimmed, "証拠")) || (strings.Contains(strings.ToLower(trimmed), "claim") && strings.Contains(strings.ToLower(trimmed), "evidence"))) {
			hasClaimsHeading = true
		}
	}
	if !hasTitle {
		res.Errors = append(res.Errors, Issue{
			Code:    "MISSING_HEADING",
			Path:    templateRelPath,
			Message: "template must include a top-level '# ' heading",
		})
	}
	if !hasOverview {
		res.Errors = append(res.Errors, Issue{
			Code:    "MISSING_HEADING",
			Path:    templateRelPath,
			Message: "template must include a '## ' change overview heading (変更概要 / Summary)",
		})
	}
	if !hasClaimsHeading {
		res.Errors = append(res.Errors, Issue{
			Code:    "MISSING_HEADING",
			Path:    templateRelPath,
			Message: "template must include a '## ' claims & evidence heading (主張と証拠 / Claims & Evidence)",
		})
	}

	tables, claimRowLines, parseIssues := parseClaimTables(lines)
	res.Errors = append(res.Errors, parseIssues...)
	if len(tables) == 0 {
		res.Errors = append(res.Errors, Issue{
			Code:    "MISSING_CLAIM_TABLE",
			Path:    templateRelPath,
			Message: "template must contain a Claim/Evidence Markdown table (主張 / 証拠)",
		})
	} else {
		rowCount := 0
		for _, tbl := range tables {
			rowCount += len(tbl.rows)
		}
		res.ClaimsChecked = rowCount
		if rowCount == 0 {
			res.Errors = append(res.Errors, Issue{
				Code:    "MISSING_CLAIM_ROWS",
				Path:    templateRelPath,
				Message: "template claim/evidence table must include at least one example row",
			})
		}
	}

	var fence fenceState
	for idx, line := range lines {
		lineNo := idx + 1
		if nextFence, toggled := toggleFence(fence, line); toggled {
			fence = nextFence
			continue
		}
		if fence.active() || claimRowLines[lineNo] {
			continue
		}
		if phrase := matchBlanketPhrase(line); phrase != "" {
			res.Errors = append(res.Errors, Issue{
				Code:    "BLANKET_UNBACKED_CLAIM",
				Path:    fmt.Sprintf("line:%d", lineNo),
				Message: fmt.Sprintf("template prose should not contain literal blanket claim phrase %q", phrase),
			})
		}
	}

	res.Valid = len(res.Errors) == 0
	return res
}
