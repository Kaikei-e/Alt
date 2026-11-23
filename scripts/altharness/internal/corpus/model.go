package corpus

import (
	"context"
	"time"
)

// Origin is the [origin] table of a corpus case.
type Origin struct {
	Branch     string
	BaseCommit string
	Files      []string
	Commits    []string
	SourceRef  string
	Notes      string
}

// Detection is the [detection] table of a corpus case.
type Detection struct {
	PlannedChecks   []string
	ActiveChecks    []string
	CurrentlyCaught bool
	CurrentMethod   string
	ExpectedSignal  string
	ExpectedSignals map[string]string
}

// Case is one validated corpus case directory.
type Case struct {
	CaseID         string
	CaseDir        string
	Title          string
	Category       string
	Language       string
	BeforeFile     string
	AfterFile      string
	SupportFiles   []string
	SupportSet     map[string]struct{}
	EvaluationMode string
	Origin         Origin
	Detection      Detection
}

// CheckOutcome is one runner invocation.
type CheckOutcome struct {
	ExitCode    int
	Output      string
	RunnerError string
}

// Outcome is the runner result name used by the check executors.
type Outcome = CheckOutcome

type runner struct {
	mode      string
	runSingle func(ctx context.Context, c Case, phase, repoRoot string, timeoutSecs int) Outcome
	runDiff   func(ctx context.Context, c Case, beforePath, afterPath, repoRoot string) Outcome
}

// SingleFunc evaluates one phase ("before" or "after").
type SingleFunc func(ctx context.Context, c Case, phase, repoRoot string, timeout time.Duration) CheckOutcome

// DiffFunc evaluates a before/after pair. The baseline call passes the before path twice.
type DiffFunc func(ctx context.Context, c Case, beforePath, afterPath, repoRoot string) CheckOutcome

// RegisteredRunner is a single-state or pair runner. A nil function fails closed.
type RegisteredRunner struct {
	Mode      string
	RunSingle SingleFunc
	RunDiff   DiffFunc
}

// CaseEvaluation is the result of every active check on one case.
type CaseEvaluation struct {
	CaseID string
	Title  string
	Status string // caught | expected-miss | fail
	Detail string
}
