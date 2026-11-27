// Package loopeval implements LOOP-05: deterministic packet-template evaluation
// scenarios grounded in CORPUS-01, anti-observer-effect workspace staging,
// four-signal grading (CHECK-01, characterization test, LOOP-01 verify-handoff,
// and multi-agent JSONL transcript read-claim audit), and multi-trial
// before/after comparison with LOOP-03 decisions.tsv recording.
package loopeval

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

	defaultMinScenarios = 5
	defaultMinTrials    = 3
	defaultCharTimeoutS = 60
	timeoutExitCode     = 124
	outputSnippetLimit  = 400

	scenariosRelDir = "scripts/harness/eval/scenarios"
	corpusRelDir    = "scripts/harness/corpus"
)

var (
	scenarioIDRE  = regexp.MustCompile(`^E-(\d{2,})$`)
	corpusIDRE    = regexp.MustCompile(`^C-(\d{2,})$`)
	commitSHARE   = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	allZeroSHARE  = regexp.MustCompile(`^0+$`)
	neutralNameRE = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)
	nohupRE       = regexp.MustCompile("(?:^|[\\s`('\"])nohup\\s+")

	validLanguages = map[string]struct{}{
		"go":         {},
		"python":     {},
		"rust":       {},
		"typescript": {},
		"shell":      {},
	}

	ignoredScenarioRootFiles = map[string]struct{}{
		"README.md": {},
		".gitkeep":  {},
	}

	// forbiddenNeutralTokens enforces anti-observer-effect neutrality so
	// staged workspace names never signal to an agent that it is under test.
	forbiddenNeutralTokens = []string{
		"eval",
		"bench",
		"benchmark",
		"candidate",
		"rubric",
		"corpus",
		"grade",
		"grading",
		"pstack",
		"harness",
		"prompt-test",
		"experiment",
		"trial",
		"test-case",
	}

	// observerEffectPhrases detects explicit eval-framing leaks in packet.md.
	observerEffectPhrases = []string{
		"this is an eval",
		"evaluation rubric",
		"blinded judge",
		"eval scenario",
		"eval harness",
		"benchmark harness",
		"grading rubric",
		"you are being evaluated",
	}

	forbiddenAutofixPatterns = []struct {
		label string
		re    *regexp.Regexp
	}{
		{"golangci-lint --fix", regexp.MustCompile(`\bgolangci-lint\b[^\n;|&]*--fix\b`)},
		{"cargo clippy --fix", regexp.MustCompile(`\bcargo\s+clippy\b[^\n;|&]*--fix\b`)},
		{"cargo fix", regexp.MustCompile(`\bcargo\s+fix\b`)},
		{"ruff check --unsafe-fixes", regexp.MustCompile(`\bruff\s+check\b[^\n;|&]*--unsafe-fixes\b`)},
		{"biome --unsafe", regexp.MustCompile(`\bbiome\s+(?:check|lint)\b[^\n;|&]*--unsafe\b`)},
	}
)

// Issue is one deterministic validation, grading, or comparison finding.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Scenario represents one LOOP-05 evaluation scenario defined by scenario.toml.
type Scenario struct {
	ID                    string   `toml:"id" json:"id"`
	CorpusID              string   `toml:"corpus_id" json:"corpus_id"`
	Title                 string   `toml:"title" json:"title"`
	Language              string   `toml:"language" json:"language"`
	BaseCommit            string   `toml:"base_commit" json:"base_commit"`
	NeutralWorkspaceName  string   `toml:"neutral_workspace_name" json:"neutral_workspace_name"`
	TargetFile            string   `toml:"target_file" json:"target_file"`
	Check01ExpectedSignal string   `toml:"check01_expected_signal" json:"check01_expected_signal"`
	HasCharTest           bool     `toml:"has_char_test" json:"has_char_test"`
	CharTestCommand       string   `toml:"char_test_command" json:"char_test_command"`
	RequiredReads         []string `toml:"required_reads" json:"required_reads"`

	// Dir is the absolute path to the scenario directory (omitted from JSON).
	Dir string `toml:"-" json:"-"`
}

// ValidationReport is the structured output of `loop-eval validate`.
type ValidationReport struct {
	Valid         bool    `json:"valid"`
	ScenariosDir  string  `json:"scenarios_dir"`
	ScenarioCount int     `json:"scenario_count"`
	MinScenarios  int     `json:"min_scenarios"`
	Errors        []Issue `json:"errors"`
}

// StageResult is the structured output of `loop-eval stage`.
type StageResult struct {
	ScenarioID           string `json:"scenario_id"`
	WorkspaceDir         string `json:"workspace_dir"`
	NeutralWorkspaceName string `json:"neutral_workspace_name"`
	BaseCommit           string `json:"base_commit"`
	TargetFile           string `json:"target_file"`
}

// ScreenerCandidate mirrors screener.Candidate for JSON serialization.
type ScreenerCandidate struct {
	ID       string `json:"id"`
	File     string `json:"file"`
	Function string `json:"function"`
	Kind     string `json:"kind"`
	Before   string `json:"before"`
	After    string `json:"after"`
}

// Check01Signal captures Signal 1 (CHECK-01 behavior-change screener).
type Check01Signal struct {
	Passed                  bool                `json:"passed"`
	ExpectedSignal          string              `json:"expected_signal"`
	ExpectedSignalTriggered bool                `json:"expected_signal_triggered"`
	CandidatesCount         int                 `json:"candidates_count"`
	Candidates              []ScreenerCandidate `json:"candidates"`
	Errors                  []Issue             `json:"errors"`
}

// CharTestSignal captures Signal 2 (offline deterministic characterization test).
type CharTestSignal struct {
	Passed        bool    `json:"passed"`
	Command       string  `json:"command"`
	ExitCode      int     `json:"exit_code"`
	DurationS     float64 `json:"duration_s"`
	OutputSnippet string  `json:"output_snippet"`
	Errors        []Issue `json:"errors"`
}

// Loop01Signal captures Signal 3 (LOOP-01 packet verify-handoff).
type Loop01Signal struct {
	Passed   bool    `json:"passed"`
	ExitCode int     `json:"exit_code"`
	NoRerun  bool    `json:"no_rerun"`
	Errors   []Issue `json:"errors"`
}

// TranscriptAuditSignal captures Signal 4 (JSONL tool-call file-read verification).
type TranscriptAuditSignal struct {
	Evaluated        bool     `json:"evaluated"`
	Passed           bool     `json:"passed"`
	ExpectedReads    []string `json:"expected_reads"`
	ObservedReads    []string `json:"observed_reads"`
	UnverifiedClaims []string `json:"unverified_claims"`
	Fidelity         float64  `json:"fidelity"`
	Errors           []Issue  `json:"errors"`
}

// GradeSignals aggregates the four deterministic grading signals.
type GradeSignals struct {
	Check01             Check01Signal         `json:"check01"`
	CharTest            CharTestSignal        `json:"char_test"`
	Loop01              Loop01Signal          `json:"loop01"`
	TranscriptReadAudit TranscriptAuditSignal `json:"transcript_read_audit"`
}

// GradeResult is the structured output of `loop-eval grade`.
type GradeResult struct {
	ScenarioID           string       `json:"scenario_id"`
	CorpusID             string       `json:"corpus_id,omitempty"`
	NeutralWorkspaceName string       `json:"neutral_workspace_name,omitempty"`
	Workspace            string       `json:"workspace,omitempty"`
	BaseCommit           string       `json:"base_commit,omitempty"`
	Score                int          `json:"score"`
	MaxScore             int          `json:"max_score"`
	Passed               bool         `json:"passed"`
	Signals              GradeSignals `json:"signals"`
}

// ScenarioComparison holds per-scenario before/after trial statistics.
type ScenarioComparison struct {
	ScenarioID              string   `json:"scenario_id"`
	BeforeTrials            int      `json:"before_trials"`
	AfterTrials             int      `json:"after_trials"`
	BeforePassRate          float64  `json:"before_pass_rate"`
	AfterPassRate           float64  `json:"after_pass_rate"`
	DeltaPassRate           float64  `json:"delta_pass_rate"`
	BeforeMeanScore         float64  `json:"before_mean_score"`
	AfterMeanScore          float64  `json:"after_mean_score"`
	DeltaMeanScore          float64  `json:"delta_mean_score"`
	BeforeReadAuditFidelity *float64 `json:"before_read_audit_fidelity"`
	AfterReadAuditFidelity  *float64 `json:"after_read_audit_fidelity"`
	DeltaReadAuditFidelity  *float64 `json:"delta_read_audit_fidelity"`
	Status                  string   `json:"status"`
}

// CompareReport is the structured output of `loop-eval compare`.
type CompareReport struct {
	Valid                   bool                 `json:"valid"`
	TemplateVersion         string               `json:"template_version,omitempty"`
	MinTrials               int                  `json:"min_trials"`
	BeforeSource            string               `json:"before_source"`
	AfterSource             string               `json:"after_source"`
	Verdict                 string               `json:"verdict"`
	BeforePassRate          float64              `json:"before_pass_rate"`
	AfterPassRate           float64              `json:"after_pass_rate"`
	DeltaPassRate           float64              `json:"delta_pass_rate"`
	BeforeMeanScore         float64              `json:"before_mean_score"`
	AfterMeanScore          float64              `json:"after_mean_score"`
	DeltaMeanScore          float64              `json:"delta_mean_score"`
	BeforeReadAuditFidelity *float64             `json:"before_read_audit_fidelity"`
	AfterReadAuditFidelity  *float64             `json:"after_read_audit_fidelity"`
	DeltaReadAuditFidelity  *float64             `json:"delta_read_audit_fidelity"`
	Scenarios               []ScenarioComparison `json:"scenarios"`
	Improvements            []string             `json:"improvements"`
	Regressions             []string             `json:"regressions"`
	RecordedArtifact        string               `json:"recorded_artifact,omitempty"`
	DecisionsFile           string               `json:"decisions_file,omitempty"`
	Errors                  []Issue              `json:"errors"`
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("encode json: %w", err)
	}
	return buf.Bytes(), nil
}

func isSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode()&os.ModeSymlink != 0
}

func isRegularNonSymlink(path string) bool {
	st, err := os.Lstat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode()&os.ModeSymlink == 0
}

func unsafeRelPath(rel string) bool {
	if strings.TrimSpace(rel) == "" {
		return true
	}
	if strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, "./") || strings.Contains(rel, `\`) || strings.Contains(rel, "//") {
		return true
	}
	if strings.HasSuffix(rel, "/") {
		return true
	}
	for _, seg := range strings.Split(rel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

func normalizeRelPath(p string) string {
	cleaned := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	var parts []string
	for _, seg := range strings.Split(cleaned, "/") {
		if seg == "" || seg == "." {
			continue
		}
		parts = append(parts, seg)
	}
	return strings.Join(parts, "/")
}

func resolvePath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("abs %s: %w", path, err)
	}
	var missing []string
	cur := abs
	for {
		resolved, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve %s: %w", path, err)
		}
		missing = append(missing, filepath.Base(cur))
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(abs), nil
		}
		cur = parent
	}
}

func withinDir(path, dir string) bool {
	rp, err1 := resolvePath(path)
	rd, err2 := resolvePath(dir)
	if err1 != nil || err2 != nil {
		return false
	}
	if rp == rd {
		return true
	}
	rel, err := filepath.Rel(rd, rp)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func matchForbiddenAutofix(command string) string {
	for _, pat := range forbiddenAutofixPatterns {
		if pat.re.MatchString(command) {
			return pat.label
		}
	}
	return ""
}

func hasBackgroundExecution(command string) bool {
	if nohupRE.MatchString(command) {
		return true
	}
	var quote byte
	escaped := false
	for i := 0; i < len(command); i++ {
		ch := command[i]
		if escaped {
			escaped = false
			continue
		}
		if quote == '"' && ch == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == '#' && (i == 0 || command[i-1] == ' ' || command[i-1] == '\t') {
			break
		}
		if ch == '&' {
			var prev, next byte
			if i > 0 {
				prev = command[i-1]
			}
			if i+1 < len(command) {
				next = command[i+1]
			}
			if prev == '>' || prev == '<' {
				continue
			}
			if next == '&' || next == '>' {
				i++
				continue
			}
			return true
		}
	}
	return false
}

func checkNeutralName(name string) (string, bool) {
	if !neutralNameRE.MatchString(name) {
		return fmt.Sprintf("neutral_workspace_name %q must match ^[a-z][a-z0-9-]{2,31}$", name), false
	}
	lower := strings.ToLower(name)
	for _, tok := range forbiddenNeutralTokens {
		if strings.Contains(lower, tok) {
			return fmt.Sprintf("neutral_workspace_name %q contains forbidden observer-effect substring %q", name, tok), false
		}
	}
	return "", true
}

func checkObserverEffectLeaks(text string) []string {
	lower := strings.ToLower(text)
	var leaks []string
	for _, phrase := range observerEffectPhrases {
		if strings.Contains(lower, phrase) {
			leaks = append(leaks, phrase)
		}
	}
	return leaks
}

func tailRunes(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[len(r)-limit:])
}

func readFileContainment(dir, rel string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	f, err := root.Open(filepath.FromSlash(rel))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

func readRegularFile(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return readFileContainment(filepath.Dir(abs), filepath.Base(abs))
}

func writeFileContainment(dir, rel string, data []byte, perm os.FileMode) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	f, err := root.OpenFile(filepath.FromSlash(rel), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	_, wErr := f.Write(data)
	cErr := f.Close()
	if wErr != nil {
		return wErr
	}
	return cErr
}
