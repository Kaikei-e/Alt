// Package loopreview implements LOOP-04 adversarial review support:
// focused review brief generation, fail-closed report validation,
// decision-log recording, and 7-focus-area coverage verification.
package loopreview

import "alt/scripts/altharness/internal/screener"

const (
	exitOK    = 0
	exitFail  = 1
	exitUsage = 2

	briefSchemaVersion = "loop-04-brief/v1"
)

// Issue represents one structured validation finding.
type Issue struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// FocusItem defines a targeted LOOP-04 adversarial review area.
type FocusItem struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description,omitempty"`
	TargetPaths []string `json:"target_paths,omitempty"`
	CorpusIDs   []string `json:"corpus_ids,omitempty"`
}

// CorpusEntry is a concise CORPUS-01 summary item loaded from meta.toml.
type CorpusEntry struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Category string `json:"category"`
	Language string `json:"language"`
}

// DiffFile summarizes one changed path without dumping raw patch hunks.
type DiffFile struct {
	Status string `json:"status"`
	Path   string `json:"path"`
}

// DiffSummary contains file-level diff metadata and `git diff --stat` output.
type DiffSummary struct {
	Base  string     `json:"base"`
	Files []DiffFile `json:"files"`
	Stat  string     `json:"stat"`
}

// Brief is the focused adversarial review packet produced by `loop-review brief`.
type Brief struct {
	SchemaVersion      string               `json:"schema_version"`
	GeneratedAt        string               `json:"generated_at"`
	RepoRoot           string               `json:"repo_root"`
	BaseRev            string               `json:"base_rev,omitempty"`
	BaseCommit         string               `json:"base_commit,omitempty"`
	HeadCommit         string               `json:"head_commit,omitempty"`
	Rules              []string             `json:"rules"`
	FocusAreas         []FocusItem          `json:"focus_areas"`
	CorpusSummary      []CorpusEntry        `json:"corpus_summary"`
	ScreenerCandidates []screener.Candidate `json:"screener_candidates"`
	Diff               *DiffSummary         `json:"diff,omitempty"`
}

// BlastRadiusProof records runnable evidence proving a `safe` verdict.
type BlastRadiusProof struct {
	Assumption string `json:"assumption"`
	Command    string `json:"command"`
	ExitCode   *int   `json:"exit_code"`
	Evidence   string `json:"evidence"`
}

// FocusVerdict records the reviewer's verdict for one LOOP-04 focus area.
type FocusVerdict struct {
	FocusID          string            `json:"focus_id"`
	Title            string            `json:"title"`
	Status           string            `json:"status"`
	Summary          string            `json:"summary"`
	Evidence         string            `json:"evidence"`
	BlastRadiusProof *BlastRadiusProof `json:"blast_radius_proof,omitempty"`
}

// ReproProof records runnable reproduction evidence for a `fix` finding.
type ReproProof struct {
	Command       string `json:"command"`
	TestName      string `json:"test_name"`
	ExitCode      *int   `json:"exit_code"`
	ProofEvidence string `json:"proof_evidence"`
}

// Finding represents one triaged finding from an adversarial review.
type Finding struct {
	ID            string      `json:"id"`
	FocusID       string      `json:"focus_id"`
	ReviewerModel string      `json:"reviewer_model"`
	Bucket        string      `json:"bucket"`
	Summary       string      `json:"summary"`
	Rationale     string      `json:"rationale"`
	Evidence      string      `json:"evidence"`
	Repro         *ReproProof `json:"repro,omitempty"`
}

// Report is the LOOP-04 adversarial review report schema.
type Report struct {
	ReviewID      string         `json:"review_id"`
	BaseCommit    string         `json:"base_commit,omitempty"`
	HeadCommit    string         `json:"head_commit,omitempty"`
	AuthorModel   string         `json:"author_model"`
	Reviewers     []string       `json:"reviewers"`
	AutoApplied   bool           `json:"auto_applied"`
	FocusVerdicts []FocusVerdict `json:"focus_verdicts"`
	Findings      []Finding      `json:"findings"`
}

// ValidationResult is the structured outcome of `loop-review validate`.
type ValidationResult struct {
	Valid              bool           `json:"valid"`
	Path               string         `json:"path"`
	ReviewID           string         `json:"review_id,omitempty"`
	AuthorFamily       string         `json:"author_family,omitempty"`
	ReviewerFamilies   []string       `json:"reviewer_families"`
	FocusVerdictsCount int            `json:"focus_verdicts_count"`
	FindingsCount      int            `json:"findings_count"`
	BucketCounts       map[string]int `json:"bucket_counts"`
	Errors             []Issue        `json:"errors"`
}

// RecordResult is the structured outcome of `loop-review record`.
type RecordResult struct {
	Recorded             bool   `json:"recorded"`
	ReviewID             string `json:"review_id"`
	SavedReportPath      string `json:"saved_report_path"`
	DecisionsFile        string `json:"decisions_file"`
	FindingsRecorded     int    `json:"findings_recorded"`
	FocusVerdictsWritten int    `json:"focus_verdicts_recorded"`
}

// CoverageOptions configures `loop-review check-coverage`.
type CoverageOptions struct {
	StrictResolved bool
}

// CoverageResult is the structured outcome of `loop-review check-coverage`.
type CoverageResult struct {
	Covered        bool        `json:"covered"`
	StrictResolved bool        `json:"strict_resolved,omitempty"`
	DecisionsFile  string      `json:"decisions_file"`
	TotalRequired  int         `json:"total_required"`
	CoveredCount   int         `json:"covered_count"`
	CoveredIDs     []string    `json:"covered_ids"`
	MissingIDs     []string    `json:"missing_ids"`
	MissingItems   []FocusItem `json:"missing_items"`
	Error          string      `json:"error,omitempty"`
}

var canonicalFocusAreas = []FocusItem{
	{
		ID:          "F-01",
		Title:       "errorlint が書き換えた条件式",
		Description: "errors.As 変換時に && 条件や ! 否定演算子が脱落していないか検証する",
		TargetPaths: []string{
			"alt-backend/app/middleware/dos_protection_middleware.go",
			"alt-backend/app/connect/errorhandler/error_handler.go",
			"alt-backend/app/orchestrator/rest/rest_feeds/summarization/helpers.go",
			"altctl/internal/config/config.go",
		},
		CorpusIDs: []string{"C-01", "C-02"},
	},
	{
		ID:          "F-02",
		Title:       "protogetter の nil ガード",
		Description: "RPC 応答が nil の場合にゼロ値でサイレント継続せず fail-fast することを検証する",
		TargetPaths: []string{
			"pre-processor/app",
			"search-indexer/app",
			"alt-backend/app",
		},
		CorpusIDs: []string{"C-04"},
	},
	{
		ID:          "F-03",
		Title:       "auth-hub の設定ファイル読み込み",
		Description: "シークレット・設定ファイル読み込みの fail-fast・ファイル権限・タイムアウト不変条件を検証する",
		TargetPaths: []string{
			"auth-hub/config/config.go",
			"auth-hub/cmd/auth-hub/main.go",
		},
		CorpusIDs: []string{"C-11"},
	},
	{
		ID:          "F-04",
		Title:       "altctl のパス封じ込め",
		Description: "altctl のパス結合における .. トラバーサル拒否・symlink 脱出防止・.env 権限 0600 を検証する",
		TargetPaths: []string{
			"altctl/internal",
		},
		CorpusIDs: []string{"C-11"},
	},
	{
		ID:          "F-05",
		Title:       "nilerr の縮退箇所",
		Description: "err != nil で nil を返す縮退箇所が意図されたフォールバックであり障害握りつぶしでないことを検証する",
		TargetPaths: []string{
			"alt-backend/app",
			"pre-processor/app",
			"rag-orchestrator",
			"search-indexer/app",
		},
		CorpusIDs: []string{"C-17"},
	},
	{
		ID:          "F-06",
		Title:       "news-creator のゲートウェイ",
		Description: "PrioritySemaphore / OllamaGateway / SSE ストリームの例外絞り込み・QueueFullError 伝播・終端保証を検証する",
		TargetPaths: []string{
			"news-creator/app/news_creator/gateway",
			"news-creator/app/news_creator/handler/sse_stream.py",
		},
		CorpusIDs: []string{"C-06", "C-07"},
	},
	{
		ID:          "F-07",
		Title:       "長大関数の分割",
		Description: "recap-subworker / recap-worker の長大関数分割における前後同値性と一時足場残留ゼロを検証する",
		TargetPaths: []string{
			"docs/review/verify-02-differential-evidence.md",
			"recap-subworker/recap_subworker",
			"recap-worker/recap-worker",
		},
		CorpusIDs: []string{"C-06"},
	},
}

// CanonicalFocusAreas returns a defensive copy of the 7 LOOP-04 focus areas.
func CanonicalFocusAreas() []FocusItem {
	out := make([]FocusItem, len(canonicalFocusAreas))
	for i, item := range canonicalFocusAreas {
		copied := item
		if len(item.TargetPaths) > 0 {
			copied.TargetPaths = append([]string(nil), item.TargetPaths...)
		}
		if len(item.CorpusIDs) > 0 {
			copied.CorpusIDs = append([]string(nil), item.CorpusIDs...)
		}
		out[i] = copied
	}
	return out
}

func canonicalFocusMap() map[string]FocusItem {
	m := make(map[string]FocusItem, len(canonicalFocusAreas))
	for _, item := range canonicalFocusAreas {
		m[item.ID] = item
	}
	return m
}
