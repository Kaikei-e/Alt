// Package loopcorrect implements LOOP-07: the /correct cadence engine
// (`alt-harness loop-correct`) for scanning recurring failures from
// decisions.tsv, enforcing pstack-style rung-ladder disposition validation,
// recording Markdown/JSON/TSV artifacts, and scaffolding CORPUS-01 cases.
package loopcorrect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	defaultThreshold        = 2
	minHigherRungReasonRune = 12
	minReasonLetters        = 8
	minDistinctReasonRunes  = 4
	maxFallbackClauseRunes  = 48

	astGrepRulesRel = "scripts/lint/ast-grep/rules"
	corpusDirRel    = "scripts/harness/corpus"
	altHarnessRel   = "scripts/altharness/alt-harness"
	ruleLedgerRel   = ".agents/rules/rule-ledger.md"
)

// rungOrder defines the strict LOOP-07 /correct precedence ladder from highest
// (structural prevention) to lowest (prose rule ledger).
var rungOrder = []string{"ARCH", "CHECK", "VERIFY", "LEDGER"}

var rungRank = map[string]int{
	"ARCH":   1,
	"CHECK":  2,
	"VERIFY": 3,
	"LEDGER": 4,
}

var (
	milestoneIDRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{1,63}$`)
	corpusIDRE      = regexp.MustCompile(`^C-(\d{2,})$`)
	astGrepIDRE     = regexp.MustCompile(`^\s*id\s*:\s*([A-Za-z0-9_.-]+)\s*$`)
	ruleLedgerIDRE  = regexp.MustCompile(`^R-\d{2,}$`)
	ruleLedgerRowRE = regexp.MustCompile(`^\|\s*(R-\d{2,})\s*\|`)
)

var archAltHarnessSubcommands = map[string]struct{}{
	"sync-go":   {},
	"pki":       {},
	"pyver":     {},
	"layers":    {},
	"generated": {},
}

var checkAltHarnessSubcommands = map[string]struct{}{
	"screener":          {},
	"ratchet":           {},
	"suppressions":      {},
	"suppression-diff":  {},
	"canary":            {},
	"entrypoint-canary": {},
	"workflow":          {},
	"hygiene":           {},
	"generated":         {},
	"preflight":         {},
	"selfcheck":         {},
	"lint":              {},
	"lang":              {},
	"adr-gate":          {},
	"migration-gate":    {},
	"postmortem":        {},
	"loop-review":       {},
	"loop-correct":      {},
}

var verifyAltHarnessSubcommands = map[string]struct{}{
	"verify-claims": {},
	"verify-diff":   {},
	"verify-golden": {},
	"verify-live":   {},
	"flaky-hunt":    {},
	"packet":        {},
	"host-final":    {},
	"corpus":        {},
	"pytest-rand":   {},
	"loop-eval":     {},
}

var archAllowedFileExts = map[string]struct{}{
	".go":   {},
	".py":   {},
	".rs":   {},
	".ts":   {},
	".toml": {},
	".yml":  {},
	".yaml": {},
	".json": {},
	".sh":   {},
}

// validAltHarnessSubcommands mirrors the canonical alt-harness command set
// (including LOOP-04..07 commands introduced by ADR-001039) so enforcement_ref
// validation catches phantom subcommands without importing package main.
var validAltHarnessSubcommands = map[string]struct{}{
	"lint":              {},
	"lang":              {},
	"layers":            {},
	"host-final":        {},
	"link-naming":       {},
	"sync-go":           {},
	"pki":               {},
	"pyver":             {},
	"workflow":          {},
	"hygiene":           {},
	"ratchet":           {},
	"suppressions":      {},
	"suppression-diff":  {},
	"generated":         {},
	"screener":          {},
	"packet":            {},
	"corpus":            {},
	"selfcheck":         {},
	"canary":            {},
	"entrypoint-canary": {},
	"postmortem":        {},
	"migration-gate":    {},
	"adr-gate":          {},
	"guard-exec":        {},
	"guard-scope":       {},
	"guard-ops":         {},
	"guard-protected":   {},
	"guard-event-time":  {},
	"guard-adr-read":    {},
	"harness-run":       {},
	"agy-hook":          {},
	"gate":              {},
	"preflight":         {},
	"pytest-rand":       {},
	"verify-claims":     {},
	"verify-diff":       {},
	"verify-golden":     {},
	"verify-live":       {},
	"flaky-hunt":        {},
	"loop-review":       {},
	"loop-eval":         {},
	"loop-correct":      {},
}

// Disposition is the input JSON contract for `loop-correct validate` and `record`.
type Disposition struct {
	MilestoneID string             `json:"milestone_id"`
	Service     string             `json:"service"`
	Classes     []ClassDisposition `json:"classes"`
}

// ClassDisposition records one recurring failure class and its rung-ladder choice.
type ClassDisposition struct {
	ClassID             string            `json:"class_id"`
	Title               string            `json:"title"`
	Occurrences         int               `json:"occurrences"`
	Evidence            []string          `json:"evidence"`
	CorpusID            string            `json:"corpus_id"`
	ChosenRung          string            `json:"chosen_rung"`
	HigherRungsRejected map[string]string `json:"higher_rungs_rejected"`
	EnforcementRef      string            `json:"enforcement_ref"`
	Notes               string            `json:"notes"`
}

// ValidationIssue represents one fail-closed validation failure.
type ValidationIssue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationReport is the structured output of `loop-correct validate`.
type ValidationReport struct {
	Valid       bool              `json:"valid"`
	File        string            `json:"file"`
	MilestoneID string            `json:"milestone_id,omitempty"`
	Service     string            `json:"service,omitempty"`
	ClassCount  int               `json:"class_count"`
	Issues      []ValidationIssue `json:"issues"`
}

// ClassSummary aggregates failures belonging to one failure class in `scan`.
type ClassSummary struct {
	ClassKey        string         `json:"class_key"`
	MatchSource     string         `json:"match_source"`
	Count           int            `json:"count"`
	CorpusMapped    bool           `json:"corpus_mapped"`
	CorpusIDs       []string       `json:"corpus_ids"`
	Evidence        []string       `json:"evidence"`
	Timestamps      []string       `json:"timestamps"`
	Phases          []string       `json:"phases"`
	Results         map[string]int `json:"results"`
	SampleDecisions []string       `json:"sample_decisions"`
	SourceFiles     []string       `json:"source_files"`
}

// ScanReport is the structured output of `loop-correct scan`.
type ScanReport struct {
	Threshold            int            `json:"threshold"`
	TotalFailuresScanned int            `json:"total_failures_scanned"`
	FilesScanned         []string       `json:"files_scanned"`
	RecurringClasses     []ClassSummary `json:"recurring_classes"`
	SingleOccurrences    []ClassSummary `json:"single_occurrences"`
}

// RecordReport is the structured output of `loop-correct record`.
type RecordReport struct {
	Recorded      bool   `json:"recorded"`
	MilestoneID   string `json:"milestone_id"`
	Service       string `json:"service"`
	MarkdownPath  string `json:"markdown_path"`
	JSONPath      string `json:"json_path"`
	DecisionsFile string `json:"decisions_file"`
	EvidenceRef   string `json:"evidence_ref"`
	RowsAppended  int    `json:"rows_appended"`
}

// ScaffoldReport is the structured output of `loop-correct scaffold-corpus`.
type ScaffoldReport struct {
	CorpusID   string `json:"corpus_id"`
	ClassID    string `json:"class_id"`
	Title      string `json:"title"`
	Language   string `json:"language"`
	Category   string `json:"category"`
	Dir        string `json:"dir"`
	MetaPath   string `json:"meta_path"`
	BeforePath string `json:"before_path"`
	AfterPath  string `json:"after_path"`
	Written    bool   `json:"written"`
	MetaTOML   string `json:"meta_toml"`
}

func marshalPrettyJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

func sanitizeSingleLine(s string) string {
	replacer := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", " ")
	cleaned := replacer.Replace(s)
	return strings.Join(strings.Fields(cleaned), " ")
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func rejectSymlinkChainUnder(baseDir, targetPath string) error {
	cleanBase := filepath.Clean(baseDir)
	cleanTarget := filepath.Clean(targetPath)
	rel, err := filepath.Rel(cleanBase, cleanTarget)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return fmt.Errorf("path %s escapes base directory %s", targetPath, baseDir)
	}
	if rel == "." {
		return nil
	}
	cur := cleanBase
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("stat path %s: %w", cur, err)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path must not traverse symlink: %s", cur)
		}
	}
	return nil
}

func rejectSymlinkOrDir(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s %s: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory: %s", label, path)
	}
	return nil
}

func rejectSymlinkOrNonDir(path, label string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s %s: %w", label, path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s must not be a symlink: %s", label, path)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists and is not a directory: %s", label, path)
	}
	return nil
}

func isWithinDir(path, root string) bool {
	cleanPath := filepath.Clean(path)
	cleanRoot := filepath.Clean(root)
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func resolveExistingPath(p string) string {
	if p == "" {
		return ""
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

func readAllInRoot(root *os.Root, name string) ([]byte, error) {
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func readFileInParentRoot(path string) ([]byte, error) {
	clean := filepath.Clean(path)
	root, err := os.OpenRoot(filepath.Dir(clean))
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return readAllInRoot(root, filepath.Base(clean))
}

func writef(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, args ...any) {
	_, _ = fmt.Fprintln(w, args...)
}

func writeStr(w io.Writer, s string) {
	_, _ = io.WriteString(w, s)
}
