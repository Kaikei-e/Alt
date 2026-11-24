package ledgers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// RunPostmortem is the postmortem action-item ledger and CI gate.
// Exit 0 is clean, 1 is a gate finding, 2 is usage or git failure.
func RunPostmortem(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	ctx, stdout, stderr, code := prepare(ctx, stdout, stderr)
	if code != 0 {
		return code
	}
	opt, errText := parsePostmortemArgs(args)
	if errText != "" {
		fmt.Fprint(stderr, errText)
		return 2
	}
	if opt.help {
		fmt.Fprint(stdout, postmortemUsage)
		return 0
	}
	root := opt.root
	if root == "" {
		root = defaultRepoRoot()
	}
	root = resolvePath(root)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: root directory does not exist: %s\n", root)
		return 2
	}
	if opt.baseSet && !opt.check {
		fmt.Fprintln(stderr, "error: --base requires --check")
		return 2
	}
	if opt.check {
		var base *string
		if opt.baseSet {
			base = &opt.base
		}
		return runPostmortemCheck(ctx, root, base, stdout, stderr)
	}
	today := time.Now().In(time.Local)
	if opt.todaySet {
		parsed, ok := parseDate(opt.today)
		if !ok {
			fmt.Fprintf(stderr, "error: invalid --today date (expected YYYY-MM-DD): %s\n", pyQuote(opt.today))
			return 2
		}
		today = parsed
	}
	var match *regexp.Regexp
	if opt.matchSet {
		re, err := regexp.Compile(opt.match)
		if err != nil {
			fmt.Fprintf(stderr, "error: invalid --match regex %s: %s\n", pyQuote(opt.match), err.Error())
			return 2
		}
		match = re
	}
	return runLedger(root, ledgerFilter{
		onlyOpen:    opt.open,
		onlyOverdue: opt.overdue,
		today:       dateOnly(today),
		match:       match,
		onlyCarried: opt.carried,
	}, opt.json, stdout, stderr)
}

const postmortemUsage = `usage: postmortem_ledger [--root PATH] [--open] [--overdue] [--today YYYY-MM-DD]
                          [--match REGEX] [--carried] [--json] [--check] [--base REV]

Postmortem action-item ledger and CI gate.
`

type pmOpts struct {
	root     string
	open     bool
	overdue  bool
	today    string
	todaySet bool
	match    string
	matchSet bool
	carried  bool
	json     bool
	check    bool
	base     string
	baseSet  bool
	help     bool
}

func parsePostmortemArgs(args []string) (pmOpts, string) {
	var opt pmOpts
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			opt.help = true
			return opt, ""
		case arg == "--open":
			opt.open = true
		case arg == "--overdue":
			opt.overdue = true
		case arg == "--carried":
			opt.carried = true
		case arg == "--json":
			opt.json = true
		case arg == "--check":
			opt.check = true
		case arg == "--root" || arg == "--today" || arg == "--match" || arg == "--base":
			if i+1 >= len(args) {
				return opt, fmt.Sprintf("error: %s requires a value\n", arg)
			}
			i++
			val := args[i]
			switch arg {
			case "--root":
				opt.root = val
			case "--today":
				opt.today, opt.todaySet = val, true
			case "--match":
				opt.match, opt.matchSet = val, true
			default:
				opt.base, opt.baseSet = val, true
			}
		case strings.HasPrefix(arg, "--root="):
			opt.root = strings.TrimPrefix(arg, "--root=")
		case strings.HasPrefix(arg, "--today="):
			opt.today, opt.todaySet = strings.TrimPrefix(arg, "--today="), true
		case strings.HasPrefix(arg, "--match="):
			opt.match, opt.matchSet = strings.TrimPrefix(arg, "--match="), true
		case strings.HasPrefix(arg, "--base="):
			opt.base, opt.baseSet = strings.TrimPrefix(arg, "--base="), true
		default:
			return opt, fmt.Sprintf("error: unrecognized arguments: %s\n", arg)
		}
	}
	return opt, ""
}

const minReasonChars = 10

var (
	pmFilenameRe     = regexp.MustCompile(`^(PM-\d{4}-\d{3})-(.+)\.md$`)
	pmIDRe           = regexp.MustCompile(`\b(PM-\d{4}-\d{3})\b`)
	pmShortIDRe      = regexp.MustCompile(`PM-(\d{3})`)
	h2Re             = regexp.MustCompile(`^##\s+(.*)$`)
	h3Re             = regexp.MustCompile(`^###\s+(.*)$`)
	fenceRe          = regexp.MustCompile(`^\s*` + "```")
	tableSepRe       = regexp.MustCompile(`^\s*\|(?:\s*:?-+:?\s*\|)+\s*$`)
	dateRe           = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	leadingDateRe    = regexp.MustCompile(`^\s*(\d{4}-\d{2}-\d{2})`)
	htmlCommentRe    = regexp.MustCompile(`(?s)<!--.*?-->`)
	patternHeadingRe = regexp.MustCompile(`^##\s+(FP-\d+)\s+`)
	fpIDRe           = regexp.MustCompile(`^FP-\d+$`)
	adrWikilinkRe    = regexp.MustCompile(`\[\[(\d{6})(?:\|[^\]\n]*)?\]\]`)
	prRefRe          = regexp.MustCompile(`(?i)PR\s*#\d+`)
	pullURLRe        = regexp.MustCompile(`https?://[^\s)>\]\"'` + "`" + `]+/pull/\d+`)
	commitHashRe     = regexp.MustCompile(`[0-9a-f]{7,40}`)
	backtickSpanRe   = regexp.MustCompile("`([^`\n]+)`")
	dropReasonRe     = regexp.MustCompile(`破棄理由\s*[:：]\s*(.+)`)
	failureItemRe    = regexp.MustCompile(`^\s*[-*]\s+(?:\*\*)?(失敗の型|再発|持ち越し|止められたはずの検査)(?:\*\*)?\s*[:：]\s*(?:\*\*)?\s*(.*)$`)
	checkboxDoneRe   = regexp.MustCompile(`(?i)\[x\]`)
	checklistItemRe  = regexp.MustCompile(`^\s*-\s+\[([ xX])\]\s+(.*)$`)
	checklistOwnerRe = regexp.MustCompile(`担当\s*(?:\*\*)?\s*[:：]\s*(?:\*\*)?\s*([^、,。\n*]+)`)
	checklistDueRe   = regexp.MustCompile(`期限\s*(?:\*\*)?\s*[:：]\s*(?:\*\*)?\s*(\d{4}-\d{2}-\d{2})`)
	listItemRe       = regexp.MustCompile(`^\s*[-*]\s+`)
	subheadTrimRe    = regexp.MustCompile(`\s*[（(][^）)]*[）)]\s*$`)
	outsideReasonRe  = regexp.MustCompile(`^[：:\s()（）・、。-]+`)
	fpSplitRe        = regexp.MustCompile(`[,、/\s]+`)
)

var failurePatternKeys = []string{"失敗の型", "再発", "持ち越し", "止められたはずの検査"}

var gateAllowedStatuses = map[string]struct{}{
	"TODO": {}, "進行中": {}, "完了": {}, "破棄": {},
}

var placeholderOwners = map[string]struct{}{
	"": {}, "要アサイン": {}, "tbd": {}, "未定": {}, "-": {}, "—": {},
}

var checkmarkChars = []string{"✓", "✔", "✅", "☑"}

var openTokens = map[string]struct{}{
	"todo": {}, "未着手": {}, "未": {}, "進行中": {}, "in progress": {}, "in_progress": {}, "open": {},
}
var doneTokens = map[string]struct{}{"done": {}, "完了": {}}
var droppedTokens = map[string]struct{}{"破棄": {}, "won't do": {}, "wont do": {}, "dropped": {}}

type actionRow struct {
	PostmortemID     string `json:"postmortem_id"`
	File             string `json:"file"`
	ItemID           string `json:"item_id"`
	Category         string `json:"category"`
	Action           string `json:"action"`
	Owner            string `json:"owner"`
	Due              string `json:"due"`
	Status           string `json:"status"`
	NormalisedStatus string `json:"normalised_status"`
}

type parsedActionItems struct {
	tableRows      []actionRow
	checklistRows  []actionRow
	unmappedTables int
}

func (p parsedActionItems) allRows() []actionRow {
	out := make([]actionRow, 0, len(p.tableRows)+len(p.checklistRows))
	out = append(out, p.tableRows...)
	out = append(out, p.checklistRows...)
	return out
}

func splitMDRow(line string) []string {
	s := strings.TrimSpace(line)
	if strings.HasPrefix(s, "|") {
		s = s[1:]
	}
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, `\|`) {
		s = s[:len(s)-1]
	}
	var cells []string
	var buf strings.Builder
	inCode := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && i+1 < len(s) && s[i+1] == '|' {
			buf.WriteByte('|')
			i++
			continue
		}
		if ch == '`' {
			if inCode {
				inCode = false
			} else if strings.Contains(s[i+1:], "`") {
				inCode = true
			}
			buf.WriteByte(ch)
			continue
		}
		if ch == '|' && !inCode {
			cells = append(cells, strings.TrimSpace(buf.String()))
			buf.Reset()
			continue
		}
		buf.WriteByte(ch)
	}
	cells = append(cells, strings.TrimSpace(buf.String()))
	return cells
}

func normaliseStatus(raw string) string {
	stripped := strings.TrimSpace(raw)
	if stripped == "" {
		return "unknown"
	}
	for _, mark := range checkmarkChars {
		if strings.Contains(stripped, mark) {
			return "done"
		}
	}
	if checkboxDoneRe.MatchString(stripped) {
		return "done"
	}
	head := stripped
	if i := strings.IndexAny(head, "（("); i >= 0 {
		head = head[:i]
	}
	head = strings.ToLower(strings.Trim(head, " *`_\t"))
	if _, ok := doneTokens[head]; ok {
		return "done"
	}
	if _, ok := droppedTokens[head]; ok {
		return "dropped"
	}
	if _, ok := openTokens[head]; ok {
		return "open"
	}
	return "unknown"
}

func subheadingToCategory(heading string) string {
	if strings.Contains(heading, "カテゴリの説明") {
		return ""
	}
	return strings.TrimSpace(subheadTrimRe.ReplaceAllString(heading, ""))
}

func mapColumns(header []string) map[string]int {
	mapping := map[string]int{}
	for idx, col := range header {
		c := strings.TrimSpace(col)
		switch {
		case (c == "#" || c == "ID") && !hasKey(mapping, "item_id"):
			mapping["item_id"] = idx
		case c == "カテゴリ" && !hasKey(mapping, "category"):
			mapping["category"] = idx
		case (c == "アクション" || c == "内容") && !hasKey(mapping, "action"):
			mapping["action"] = idx
		case (c == "担当" || c == "担当者") && !hasKey(mapping, "owner"):
			mapping["owner"] = idx
		case c == "期限" && !hasKey(mapping, "due"):
			mapping["due"] = idx
		case (c == "状態" || c == "ステータス" || c == "Status" || c == "status") && !hasKey(mapping, "status"):
			mapping["status"] = idx
		}
	}
	return mapping
}

func hasKey(m map[string]int, k string) bool {
	_, ok := m[k]
	return ok
}

func rowCell(colMap map[string]int, cells []string, key string) string {
	idx, ok := colMap[key]
	if !ok || idx >= len(cells) {
		return ""
	}
	return cells[idx]
}

func parseMDTableRows(lines []string, start int, pmID, filename, currentCat string) ([]actionRow, int, int) {
	header := splitMDRow(lines[start])
	colMap := mapColumns(header)
	i := start + 2
	total := len(lines)
	if _, ok := colMap["action"]; !ok {
		for i < total && strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "|") {
			i++
		}
		return nil, 1, i
	}
	var rows []actionRow
	for i < total && strings.HasPrefix(strings.TrimLeft(lines[i], " \t"), "|") {
		cells := splitMDRow(lines[i])
		category := rowCell(colMap, cells, "category")
		if category == "" {
			category = currentCat
		}
		status := rowCell(colMap, cells, "status")
		rows = append(rows, actionRow{
			PostmortemID:     pmID,
			File:             filename,
			ItemID:           rowCell(colMap, cells, "item_id"),
			Category:         category,
			Action:           rowCell(colMap, cells, "action"),
			Owner:            rowCell(colMap, cells, "owner"),
			Due:              rowCell(colMap, cells, "due"),
			Status:           status,
			NormalisedStatus: normaliseStatus(status),
		})
		i++
	}
	return rows, 0, i
}

func parseChecklistBody(lines []string, start int, initial string) (string, int) {
	parts := []string{initial}
	i := start
	total := len(lines)
	for i < total {
		nxt := lines[i]
		if strings.TrimSpace(nxt) == "" || fenceRe.MatchString(nxt) || h2Re.MatchString(nxt) || h3Re.MatchString(nxt) ||
			checklistItemRe.MatchString(nxt) || strings.HasPrefix(strings.TrimLeft(nxt, " \t"), "|") || listItemRe.MatchString(nxt) {
			break
		}
		parts = append(parts, strings.TrimSpace(nxt))
		i++
	}
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " "), i
}

func parseActionItems(pmID, filename, text string) parsedActionItems {
	lines := strings.Split(text, "\n")
	// Drop a trailing empty slice element produced by a final newline so
	// blank-line handling matches Python splitlines().
	if len(lines) > 0 && lines[len(lines)-1] == "" && strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	inAI := false
	inFence := false
	currentCat := ""
	var tableRows []actionRow
	var checklistRows []actionRow
	unmapped := 0
	for i := 0; i < len(lines); {
		line := lines[i]
		if fenceRe.MatchString(line) {
			inFence = !inFence
			i++
			continue
		}
		if inFence {
			i++
			continue
		}
		if m := h2Re.FindStringSubmatch(line); m != nil {
			inAI = strings.Contains(m[1], "アクションアイテム")
			currentCat = ""
			i++
			continue
		}
		if !inAI {
			i++
			continue
		}
		if m := h3Re.FindStringSubmatch(line); m != nil {
			currentCat = subheadingToCategory(m[1])
			i++
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "|") && i+1 < len(lines) && tableSepRe.MatchString(lines[i+1]) {
			parsed, delta, next := parseMDTableRows(lines, i, pmID, filename, currentCat)
			tableRows = append(tableRows, parsed...)
			unmapped += delta
			i = next
			continue
		}
		if m := checklistItemRe.FindStringSubmatch(line); m != nil {
			mark := m[1]
			action, next := parseChecklistBody(lines, i+1, strings.TrimSpace(m[2]))
			owner, due := extractChecklistOwnerDue(action)
			statusNorm := "open"
			if mark == "x" || mark == "X" {
				statusNorm = "done"
			}
			checklistRows = append(checklistRows, actionRow{
				PostmortemID:     pmID,
				File:             filename,
				ItemID:           fmt.Sprintf("%d", len(checklistRows)+1),
				Category:         currentCat,
				Action:           action,
				Owner:            owner,
				Due:              due,
				Status:           "[" + mark + "]",
				NormalisedStatus: statusNorm,
			})
			i = next
			continue
		}
		i++
	}
	return parsedActionItems{tableRows: tableRows, checklistRows: checklistRows, unmappedTables: unmapped}
}

func extractChecklistOwnerDue(action string) (string, string) {
	owner := ""
	due := ""
	if m := checklistOwnerRe.FindStringSubmatch(action); m != nil {
		owner = strings.TrimSpace(m[1])
	}
	if m := checklistDueRe.FindStringSubmatch(action); m != nil {
		due = m[1]
	}
	return owner, due
}

func listPostmortemFiles(pmDir string) ([]string, []string, error) {
	entries, err := os.ReadDir(pmDir)
	if err != nil {
		return nil, nil, fmt.Errorf("read postmortems: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var ids []string
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := pmFilenameRe.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		ids = append(ids, m[1])
		paths = append(paths, filepath.Join(pmDir, entry.Name()))
	}
	return ids, paths, nil
}

func parseDate(raw string) (time.Time, bool) {
	s := strings.TrimSpace(raw)
	if !dateRe.MatchString(s) {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func parseLeadingDate(raw string) (time.Time, bool) {
	m := leadingDateRe.FindStringSubmatch(raw)
	if m == nil {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", m[1])
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func rowCitesOtherPM(row actionRow) bool {
	for _, cited := range pmIDRe.FindAllStringSubmatch(row.Action, -1) {
		if cited[1] != row.PostmortemID {
			return true
		}
	}
	parts := strings.Split(row.PostmortemID, "-")
	ownShort := parts[len(parts)-1]
	for _, loc := range pmShortIDRe.FindAllStringSubmatchIndex(row.Action, -1) {
		if !shortIDBounded(row.Action, loc[0], loc[1]) {
			continue
		}
		sub := pmShortIDRe.FindStringSubmatch(row.Action[loc[0]:loc[1]])
		if sub != nil && sub[1] != ownShort {
			return true
		}
	}
	return false
}

func shortIDBounded(s string, start, end int) bool {
	if start > 0 {
		c := s[start-1]
		if isASCIIWordOrDash(c) || c == '_' {
			return false
		}
	}
	if end < len(s) {
		c := s[end]
		if c >= '0' && c <= '9' || c == '-' {
			return false
		}
	}
	return true
}

func isASCIIWordOrDash(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '-'
}

type ledgerFilter struct {
	onlyOpen    bool
	onlyOverdue bool
	today       time.Time
	match       *regexp.Regexp
	onlyCarried bool
}

func filterRows(rows []actionRow, flt ledgerFilter) []actionRow {
	var result []actionRow
	for _, row := range rows {
		if flt.onlyOpen && row.NormalisedStatus != "open" && row.NormalisedStatus != "unknown" {
			continue
		}
		if flt.onlyOverdue {
			if row.NormalisedStatus != "open" && row.NormalisedStatus != "unknown" {
				continue
			}
			due, ok := parseLeadingDate(row.Due)
			if !ok || !due.Before(flt.today) {
				continue
			}
		}
		if flt.match != nil && !flt.match.MatchString(row.Action) && !flt.match.MatchString(row.Owner) && !flt.match.MatchString(row.File) {
			continue
		}
		if flt.onlyCarried && !rowCitesOtherPM(row) {
			continue
		}
		result = append(result, row)
	}
	return result
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	if limit <= 0 {
		return "…"
	}
	rs := []rune(text)
	return string(rs[:limit-1]) + "…"
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}

func runLedger(root string, flt ledgerFilter, asJSON bool, stdout, stderr io.Writer) int {
	pmDir := filepath.Join(root, "docs", "postmortems")
	info, err := os.Stat(pmDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: postmortems directory does not exist: %s\n", pmDir)
		return 2
	}
	ids, paths, err := listPostmortemFiles(pmDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 2
	}
	var all []actionRow
	var unparsed []string
	for i, pmID := range ids {
		text, err := os.ReadFile(paths[i])
		if err != nil {
			fmt.Fprintf(stderr, "error: read %s: %s\n", paths[i], err.Error())
			return 2
		}
		parsed := parseActionItems(pmID, filepath.Base(paths[i]), string(text))
		rows := parsed.allRows()
		if len(rows) == 0 {
			unparsed = append(unparsed, filepath.Base(paths[i]))
		} else {
			all = append(all, rows...)
		}
	}
	filtered := filterRows(all, flt)
	if filtered == nil {
		filtered = []actionRow{}
	}
	if unparsed == nil {
		unparsed = []string{}
	}
	counts := map[string]int{
		"postmortems": len(ids),
		"rows":        len(filtered),
		"open":        countStatus(filtered, "open"),
		"done":        countStatus(filtered, "done"),
		"dropped":     countStatus(filtered, "dropped"),
		"unknown":     countStatus(filtered, "unknown"),
		"unparsed":    len(unparsed),
	}
	if asJSON {
		payload := struct {
			Rows     []actionRow   `json:"rows"`
			Unparsed []string      `json:"unparsed"`
			Summary  ledgerSummary `json:"summary"`
			Note     string        `json:"note"`
		}{
			Rows:     filtered,
			Unparsed: unparsed,
			Summary: ledgerSummary{
				Postmortems: counts["postmortems"],
				Rows:        counts["rows"],
				Open:        counts["open"],
				Done:        counts["done"],
				Dropped:     counts["dropped"],
				Unknown:     counts["unknown"],
				Unparsed:    counts["unparsed"],
			},
			Note: "statuses are as recorded in the postmortem files and may lag behind the code",
		}
		enc := json.NewEncoder(stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(payload); err != nil {
			fmt.Fprintf(stderr, "error: encode json: %s\n", err.Error())
			return 2
		}
		return 0
	}
	if len(filtered) > 0 {
		fmt.Fprintln(stdout, "PM          | ID   | Cat      | Norm    | Due        | Owner            | Action")
		fmt.Fprintln(stdout, "------------+------+----------+---------+------------+------------------+-------")
		for _, r := range filtered {
			id := r.ItemID
			if id == "" {
				id = "-"
			}
			cat := r.Category
			if cat == "" {
				cat = "-"
			}
			due := r.Due
			if due == "" {
				due = "-"
			}
			owner := r.Owner
			if owner == "" {
				owner = "-"
			}
			fmt.Fprintf(stdout, "%s | %s | %s | %s | %s | %s | %s\n",
				padRight(r.PostmortemID, 11),
				padRight(truncateRunes(id, 4), 4),
				padRight(truncateRunes(cat, 8), 8),
				padRight(r.NormalisedStatus, 7),
				padRight(truncateRunes(due, 10), 10),
				padRight(truncateRunes(owner, 16), 16),
				truncateRunes(r.Action, 72),
			)
		}
	} else {
		fmt.Fprintln(stdout, "(no matching action items)")
	}
	if len(unparsed) > 0 {
		fmt.Fprintf(stdout, "\nUnparsed (%d): %s\n", len(unparsed), strings.Join(unparsed, ", "))
	}
	fmt.Fprintln(stdout, "\nNote: statuses are as recorded in the postmortem files and may lag behind the code.")
	fmt.Fprintf(stdout, "Summary: postmortems=%d rows=%d open=%d done=%d dropped=%d unknown=%d unparsed=%d\n",
		counts["postmortems"], counts["rows"], counts["open"], counts["done"], counts["dropped"], counts["unknown"], counts["unparsed"])
	return 0
}

type ledgerSummary struct {
	Postmortems int `json:"postmortems"`
	Rows        int `json:"rows"`
	Open        int `json:"open"`
	Done        int `json:"done"`
	Dropped     int `json:"dropped"`
	Unknown     int `json:"unknown"`
	Unparsed    int `json:"unparsed"`
}

func countStatus(rows []actionRow, status string) int {
	n := 0
	for _, r := range rows {
		if r.NormalisedStatus == status {
			n++
		}
	}
	return n
}

func loadValidFPIDs(root string) (map[string]struct{}, bool) {
	path := filepath.Join(root, ".claude", "skills", "failure-pattern-review", "references", "patterns.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	ids := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := patternHeadingRe.FindStringSubmatch(line); m != nil {
			ids[m[1]] = struct{}{}
		}
	}
	return ids, true
}

func parseReadmeIndexIDs(readmePath string) (map[string]struct{}, error) {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, err
	}
	ids := map[string]struct{}{}
	for _, line := range strings.Split(string(data), "\n") {
		stripped := strings.TrimLeft(line, " \t")
		if !strings.HasPrefix(stripped, "|") || tableSepRe.MatchString(line) {
			continue
		}
		for _, m := range pmIDRe.FindAllStringSubmatch(line, -1) {
			ids[m[1]] = struct{}{}
		}
	}
	return ids, nil
}

func hasCompletionEvidence(row actionRow, root string) bool {
	text := row.Action + " " + row.Status
	for _, m := range adrWikilinkRe.FindAllStringSubmatch(text, -1) {
		if fileExists(filepath.Join(root, "docs", "ADR", m[1]+".md")) {
			return true
		}
	}
	if boundedMatch(text, prRefRe, asciiWordOrUnderscore, asciiWordOrUnderscore) || boundedMatch(text, pullURLRe, nil, pullRightBoundary) {
		return true
	}
	for _, loc := range commitHashRe.FindAllStringIndex(text, -1) {
		if !commitBounded(text, loc[0], loc[1]) {
			continue
		}
		tok := text[loc[0]:loc[1]]
		if hasHexLetter(tok) && hasDigit(tok) {
			return true
		}
	}
	rootResolved := resolvePath(root)
	for _, m := range backtickSpanRe.FindAllStringSubmatch(text, -1) {
		cand := strings.TrimSpace(m[1])
		if !strings.Contains(cand, "/") || strings.ContainsAny(cand, " \t\n") ||
			strings.HasPrefix(cand, "http://") || strings.HasPrefix(cand, "https://") || strings.HasPrefix(cand, "/") {
			continue
		}
		target := resolvePath(filepath.Join(root, strings.TrimRight(cand, "/")))
		rel, err := filepath.Rel(rootResolved, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if _, err := os.Stat(target); err == nil {
			return true
		}
	}
	return false
}

func asciiWordOrUnderscore(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'
}

func pullRightBoundary(c byte) bool {
	return asciiWordOrUnderscore(c) || c == '/'
}

func boundedMatch(text string, re *regexp.Regexp, left, right func(byte) bool) bool {
	for _, loc := range re.FindAllStringIndex(text, -1) {
		if left != nil && loc[0] > 0 && left(text[loc[0]-1]) {
			continue
		}
		if right != nil && loc[1] < len(text) && right(text[loc[1]]) {
			continue
		}
		return true
	}
	return false
}

func commitBounded(text string, start, end int) bool {
	if start > 0 {
		c := text[start-1]
		if isASCIIWordOrDash(c) {
			return false
		}
	}
	if end < len(text) {
		c := text[end]
		if isASCIIWordOrDash(c) {
			return false
		}
	}
	return true
}

func hasHexLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'a' && s[i] <= 'f' {
			return true
		}
	}
	return false
}

func hasDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func parseFailurePatternSection(text string) (bool, bool, map[string]string) {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" && strings.HasSuffix(text, "\n") {
		lines = lines[:len(lines)-1]
	}
	var headings []string
	inSection := false
	found := false
	inFence := false
	fields := map[string]string{}
	for _, line := range lines {
		if fenceRe.MatchString(line) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := h2Re.FindStringSubmatch(line); m != nil {
			heading := strings.TrimSpace(m[1])
			headings = append(headings, heading)
			if heading == "失敗の型と再発" {
				inSection = true
				found = true
			} else if inSection {
				inSection = false
			}
			continue
		}
		if !inSection {
			continue
		}
		if m := failureItemRe.FindStringSubmatch(line); m != nil {
			val := strings.TrimSpace(htmlCommentRe.ReplaceAllString(m[2], ""))
			fields[m[1]] = val
		}
	}
	immediately := false
	if found && containsString(headings, "根本原因分析") {
		rca := indexOf(headings, "根本原因分析")
		fp := indexOf(headings, "失敗の型と再発")
		immediately = fp == rca+1
	}
	return found, immediately, fields
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func indexOf(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}

func validateTableRows(rel string, parsed parsedActionItems, root string) []string {
	var findings []string
	if parsed.unmappedTables > 0 {
		findings = append(findings, fmt.Sprintf("P1 %s: table under 'アクションアイテム' could not be mapped (missing action column)", rel))
	}
	if len(parsed.tableRows) == 0 {
		findings = append(findings, fmt.Sprintf("P1 %s: action-item table under 'アクションアイテム' is missing or has no rows", rel))
		return findings
	}
	for idx, row := range parsed.tableRows {
		label := row.ItemID
		if label == "" {
			label = fmt.Sprintf("row#%d", idx+1)
		}
		if _, bad := placeholderOwners[strings.ToLower(strings.TrimSpace(row.Owner))]; bad {
			findings = append(findings, fmt.Sprintf("P2 %s (%s): owner is empty or placeholder (%s)", rel, label, pyQuote(row.Owner)))
		}
		if _, ok := parseDate(row.Due); !ok {
			findings = append(findings, fmt.Sprintf("P3 %s (%s): due date %s is not valid YYYY-MM-DD", rel, label, pyQuote(row.Due)))
		}
		status := strings.Trim(row.Status, " *\t")
		if _, ok := gateAllowedStatuses[status]; !ok {
			findings = append(findings, fmt.Sprintf("P4 %s (%s): status %s must be one of TODO, 進行中, 完了, 破棄", rel, label, pyQuote(row.Status)))
		} else if status == "完了" && !hasCompletionEvidence(row, root) {
			findings = append(findings, fmt.Sprintf("P5 %s (%s): completed item cites no evidence (existing [[NNNNNN]], PR #N or /pull/N, 7-40 hex commit, or existing backtick repo path)", rel, label))
		} else if status == "破棄" {
			reason := ""
			if m := dropReasonRe.FindStringSubmatch(row.Action); m != nil {
				reason = strings.TrimRight(strings.TrimSpace(m[1]), "）)")
			}
			if utf8.RuneCountInString(reason) < minReasonChars {
				findings = append(findings, fmt.Sprintf("P12 %s (%s): 破棄 item must state '破棄理由:' followed by at least 10 characters in the action cell", rel, label))
			}
		}
	}
	return findings
}

func validateFailurePattern(rel, text string, existing map[string]struct{}, validFP map[string]struct{}) []string {
	found, after, fields := parseFailurePatternSection(text)
	if !found {
		return []string{fmt.Sprintf("P6 %s: missing section '## 失敗の型と再発'", rel)}
	}
	var findings []string
	if !after {
		findings = append(findings, fmt.Sprintf("P6 %s: section '## 失敗の型と再発' must immediately follow '## 根本原因分析'", rel))
	}
	for _, key := range failurePatternKeys {
		val, ok := fields[key]
		if !ok || strings.TrimSpace(val) == "" {
			findings = append(findings, fmt.Sprintf("P6 %s: section '## 失敗の型と再発' missing or empty '%s:'", rel, key))
		}
	}
	fpVal := strings.TrimSpace(fields["失敗の型"])
	if strings.HasPrefix(fpVal, "型の外") {
		reason := strings.TrimSpace(outsideReasonRe.ReplaceAllString(fpVal[len("型の外"):], ""))
		if utf8.RuneCountInString(reason) < minReasonChars {
			findings = append(findings, fmt.Sprintf("P7 %s: 失敗の型 '型の外' must include a reason of at least 10 characters", rel))
		}
	} else if fpVal != "" {
		tokens := splitNonEmpty(fpSplitRe.Split(fpVal, -1))
		bad := len(tokens) == 0
		if !bad {
			for _, t := range tokens {
				if !fpIDRe.MatchString(t) {
					bad = true
					break
				}
			}
		}
		if bad {
			findings = append(findings, fmt.Sprintf("P7 %s: 失敗の型 (%s) must be a list of FP-NN ids or start with '型の外'", rel, pyQuote(fpVal)))
		} else {
			var unknown []string
			for _, t := range tokens {
				if _, ok := validFP[t]; !ok {
					unknown = append(unknown, t)
				}
			}
			if len(unknown) > 0 {
				findings = append(findings, fmt.Sprintf("P7 %s: 失敗の型 cites unknown pattern id(s): %s", rel, strings.Join(unknown, ", ")))
			}
		}
	}
	for _, refKey := range []string{"再発", "持ち越し"} {
		refVal := strings.TrimSpace(fields[refKey])
		if refVal == "" {
			continue
		}
		for _, m := range pmIDRe.FindAllStringSubmatch(refVal, -1) {
			if _, ok := existing[m[1]]; !ok {
				findings = append(findings, fmt.Sprintf("P8 %s: %s cites nonexistent postmortem %s", rel, refKey, m[1]))
			}
		}
	}
	return findings
}

func splitNonEmpty(parts []string) []string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func validateAddedPostmortem(pmID, path, root string, existing, validFP map[string]struct{}) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return []string{fmt.Sprintf("P1 docs/postmortems/%s: %s", filepath.Base(path), err.Error())}
	}
	text := string(data)
	rel := "docs/postmortems/" + filepath.Base(path)
	parsed := parseActionItems(pmID, filepath.Base(path), text)
	findings := validateTableRows(rel, parsed, root)
	findings = append(findings, validateFailurePattern(rel, text, existing, validFP)...)
	return findings
}

func resolveMergeBase(ctx context.Context, repoRoot, baseRev string, stderr io.Writer) (string, int) {
	if invalidBase(baseRev) {
		fmt.Fprintf(stderr, "error: invalid base revision: %s\n", pyQuote(baseRev))
		return "", 2
	}
	top := runGit(ctx, repoRoot, "rev-parse", "--show-toplevel")
	if top.Missing {
		gitMissing(stderr)
		return "", 2
	}
	if top.Err != nil && top.Code == 0 {
		fmt.Fprintf(stderr, "error: %s\n", top.Err.Error())
		return "", 2
	}
	if top.Code != 0 {
		fmt.Fprintf(stderr, "error: %s is not inside a git repository: %s\n", repoRoot, top.Stderr)
		return "", 2
	}
	rev := runGit(ctx, repoRoot, "rev-parse", "--verify", "--quiet", baseRev+"^{commit}")
	if rev.Missing {
		gitMissing(stderr)
		return "", 2
	}
	if rev.Code != 0 || strings.TrimSpace(rev.Stdout) == "" {
		fmt.Fprintf(stderr, "error: unknown base revision: %s\n", baseRev)
		return "", 2
	}
	mb := runGit(ctx, repoRoot, "merge-base", baseRev, "HEAD")
	if mb.Missing {
		gitMissing(stderr)
		return "", 2
	}
	if mb.Code != 0 || strings.TrimSpace(mb.Stdout) == "" {
		fmt.Fprintf(stderr, "error: git merge-base failed for %s: %s\n", baseRev, mb.Stderr)
		return "", 2
	}
	return strings.TrimSpace(mb.Stdout), 0
}

func collectAddedRelPaths(ctx context.Context, repoRoot, mergeBase string, stderr io.Writer) ([]string, int) {
	diff := runGit(ctx, repoRoot, "diff", "--no-renames", "--diff-filter=A", "--name-only", "-z", mergeBase, "--", "docs/postmortems")
	if diff.Missing {
		gitMissing(stderr)
		return nil, 2
	}
	if diff.Code != 0 {
		fmt.Fprintf(stderr, "error: git diff failed: %s\n", diff.Stderr)
		return nil, 2
	}
	untracked := runGit(ctx, repoRoot, "ls-files", "--others", "--exclude-standard", "-z", "--", "docs/postmortems")
	if untracked.Missing {
		gitMissing(stderr)
		return nil, 2
	}
	if untracked.Code != 0 {
		fmt.Fprintf(stderr, "error: git ls-files failed: %s\n", untracked.Stderr)
		return nil, 2
	}
	set := map[string]struct{}{}
	for _, token := range nullSplit(diff.Stdout) {
		set[token] = struct{}{}
	}
	for _, token := range nullSplit(untracked.Stdout) {
		set[token] = struct{}{}
	}
	rels := make([]string, 0, len(set))
	for rel := range set {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	return rels, 0
}

func runPostmortemCheck(ctx context.Context, root string, base *string, stdout, stderr io.Writer) int {
	pmDir := filepath.Join(root, "docs", "postmortems")
	info, err := os.Stat(pmDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "error: postmortems directory does not exist: %s\n", pmDir)
		return 2
	}
	readmePath := filepath.Join(pmDir, "README.md")
	if !fileExists(readmePath) {
		fmt.Fprintf(stderr, "error: postmortems README does not exist: %s\n", readmePath)
		return 2
	}
	ids, paths, err := listPostmortemFiles(pmDir)
	if err != nil {
		fmt.Fprintf(stderr, "error: %s\n", err.Error())
		return 2
	}
	existing := map[string]struct{}{}
	for _, id := range ids {
		existing[id] = struct{}{}
	}
	readmeIDs, err := parseReadmeIndexIDs(readmePath)
	if err != nil {
		fmt.Fprintf(stderr, "error: read postmortems README: %s\n", err.Error())
		return 2
	}
	var findings []string
	if base != nil {
		added, code := checkAddedPostmortems(ctx, root, *base, existing, stdout, stderr)
		if code != 0 {
			return code
		}
		findings = append(findings, added...)
	}
	for i, pmID := range ids {
		if _, ok := readmeIDs[pmID]; !ok {
			findings = append(findings, fmt.Sprintf("P9 docs/postmortems/%s: %s has no row in docs/postmortems/README.md", filepath.Base(paths[i]), pmID))
		}
	}
	var missing []string
	for id := range readmeIDs {
		if _, ok := existing[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	for _, cited := range missing {
		findings = append(findings, fmt.Sprintf("P10 docs/postmortems/README.md: %s has a row in README.md but no file in docs/postmortems/", cited))
	}
	for _, finding := range findings {
		fmt.Fprintln(stdout, finding)
	}
	fmt.Fprintf(stdout, "Summary: %d finding(s)\n", len(findings))
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func checkAddedPostmortems(ctx context.Context, root, baseRev string, existing map[string]struct{}, stdout, stderr io.Writer) ([]string, int) {
	mergeBase, code := resolveMergeBase(ctx, root, baseRev, stderr)
	if code != 0 {
		return nil, code
	}
	rels, code := collectAddedRelPaths(ctx, root, mergeBase, stderr)
	if code != 0 {
		return nil, code
	}
	pmDir := resolvePath(filepath.Join(root, "docs", "postmortems"))
	rootResolved := resolvePath(root)
	var validAdded []string
	var invalidAdded []string
	for _, rel := range rels {
		p := resolvePath(filepath.Join(root, filepath.FromSlash(rel)))
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(p), ".md") {
			continue
		}
		if p == filepath.Join(pmDir, "README.md") || samePath(p, filepath.Join(pmDir, "README.md")) {
			continue
		}
		if filepath.Dir(p) == pmDir && pmFilenameRe.MatchString(filepath.Base(p)) {
			validAdded = append(validAdded, p)
		} else {
			invalidAdded = append(invalidAdded, p)
		}
	}
	var findings []string
	for _, bad := range invalidAdded {
		rel, err := filepath.Rel(rootResolved, bad)
		if err != nil {
			rel = bad
		}
		findings = append(findings, fmt.Sprintf("P11 %s: Markdown file under docs/postmortems/ must be named PM-YYYY-NNN-<slug>.md (except README.md)", filepath.ToSlash(rel)))
	}
	if len(validAdded) > 0 {
		validFP, ok := loadValidFPIDs(root)
		if !ok {
			fmt.Fprintln(stderr, "error: patterns reference not found at .claude/skills/failure-pattern-review/references/patterns.md")
			return nil, 2
		}
		for _, path := range validAdded {
			m := pmFilenameRe.FindStringSubmatch(filepath.Base(path))
			if m == nil {
				fmt.Fprintf(stderr, "error: unexpected non-postmortem filename: %s\n", filepath.Base(path))
				return nil, 2
			}
			findings = append(findings, validateAddedPostmortem(m[1], path, root, existing, validFP)...)
		}
	} else if len(invalidAdded) == 0 {
		fmt.Fprintf(stdout, "No added postmortems relative to merge base %s; running index consistency check.\n", mergeBase)
	}
	return findings, 0
}

func samePath(a, b string) bool {
	aa := resolvePath(a)
	bb := resolvePath(b)
	return aa == bb
}

func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

func defaultRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "scripts", "altharness", "alt-harness")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "docs", "postmortems")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd
		}
		dir = parent
	}
}
