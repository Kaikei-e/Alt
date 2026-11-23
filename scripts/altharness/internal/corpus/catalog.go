package corpus

import "regexp"

const (
	ExitOK    = 0
	ExitFail  = 1
	ExitUsage = 2

	DefaultMinCases         = 22
	MaxSnippetBytes         = 8192
	MaxSnippetLines         = 120
	MinNotesNonSpaceLen     = 10
	OutputSnippetLimit      = 400
	SubprocessTimeoutSecond = 60
	subprocessTimeoutSecs   = SubprocessTimeoutSecond
	Verify01MinCommits      = 2
	WorktreeUncommitted     = "worktree-uncommitted"
)

var (
	caseDirRE          = regexp.MustCompile(`^C-(\d{2,})$`)
	commitSHARe        = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	verify01TestFileRE = regexp.MustCompile(`(?i)(?:^|/)(?:` +
		`[^/]+_test\.go|` +
		`tests?/[^/].*\.py|test_[^/]+\.py|test-[^/]+\.py|[^/]+_test\.py|` +
		`[^/]*test[^/]*\.sh|` +
		`(?:__tests__|tests|e2e)/.*\.(?:[cm]?[jt]sx?|svelte)|` +
		`[^/]+\.(?:spec|test)\.(?:[cm]?[jt]sx?|svelte)|` +
		`[^/]+_test\.[cm]?[jt]sx?|` +
		`tests/.*\.rs|tests\.rs|[^/]+_tests?\.rs` +
		`)$`)
)

func setOf(items ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(items))
	for _, item := range items {
		out[item] = struct{}{}
	}
	return out
}

func has(set map[string]struct{}, key string) bool {
	_, ok := set[key]
	return ok
}

// ValidCategories is the closed set of corpus case categories.
var ValidCategories = setOf(
	"autofix-logic-corruption",
	"unrequested-behavior-change",
	"control-flow-drop",
	"type-check-evasion",
	"test-isolation-hazard",
	"signature-override-drift",
	"test-assertion-weakening",
	"generated-code-tampering",
	"silent-error-swallow",
	"worktree-hygiene",
	"scanner-silent-pass",
	"blanket-suppression",
)

// ValidLanguages is the closed set of snippet languages.
var ValidLanguages = setOf("go", "python", "rust", "typescript", "shell", "binary")

// ValidEvaluationModes is "after" or "diff".
var ValidEvaluationModes = setOf("after", "diff")

// ImplementedChecks are checks the corpus schema treats as implemented.
var ImplementedChecks = setOf(
	"pyrefly",
	"REPO-02",
	"REPO-03",
	"ARCH-01",
	"ARCH-02",
	"ARCH-03",
	"ARCH-04",
	"ARCH-05",
	"ARCH-06",
	"CHECK-01",
	"CHECK-02",
	"CHECK-03",
	"CHECK-04",
	"CHECK-05",
	"CHECK-06",
	"CHECK-07",
	"VERIFY-01",
	"LOOP-01",
	"LOOP-02",
	"LOOP-03",
	"EXEC-01",
	"EXEC-02",
	"EXEC-03",
)

// DiffChecks compare a before/after pair instead of a single phase file.
var DiffChecks = setOf("CHECK-01", "CHECK-05", "CHECK-06", "VERIFY-01")

// UnimplementedCorpusChecks is empty when the Python module exports an empty set.
var UnimplementedCorpusChecks = map[string]struct{}{}

// ProcessChecks are process-level checks. VERIFY-01 and EXEC-01 are members.
var ProcessChecks = setOf(
	"VERIFY-01",
	"VERIFY-02",
	"VERIFY-03",
	"VERIFY-04",
	"VERIFY-05",
	"VERIFY-06",
	"LOOP-01",
	"LOOP-02",
	"LOOP-03",
	"LOOP-04",
	"LOOP-05",
	"LOOP-06",
	"LOOP-07",
	"EXEC-01",
	"EXEC-02",
	"EXEC-03",
	"EXEC-04",
)

// UnimplementedChecks is UNIMPLEMENTED_CORPUS_CHECKS union (PROCESS_CHECKS - IMPLEMENTED_CHECKS).
func UnimplementedChecks() map[string]struct{} {
	out := map[string]struct{}{}
	for k := range UnimplementedCorpusChecks {
		out[k] = struct{}{}
	}
	for k := range ProcessChecks {
		if !has(ImplementedChecks, k) {
			out[k] = struct{}{}
		}
	}
	return out
}

func knownCheck(id string) bool {
	return has(ImplementedChecks, id) || has(UnimplementedCorpusChecks, id) || has(ProcessChecks, id)
}

var pyreflyFallbackServices = []string{
	"recap-subworker",
	"news-creator/app",
	"tag-generator/app",
	"acolyte-orchestrator",
	"metrics",
	"dashboard",
	"rerank-server",
	"recap-evaluator",
	"tts-speaker",
}

var repo02RequiredSupport = []string{"sample.go", "before.sarif", "after.sarif"}

var ignoredCorpusRootFiles = setOf("README.md", ".gitkeep")

var repo03ConfigPatterns = []struct {
	re   *regexp.Regexp
	name string
}{
	{regexp.MustCompile(`^(?:(?:before|after)[._-]?)?pyproject\.toml$`), "pyproject.toml"},
	{regexp.MustCompile(`^(?:(?:before|after)[._-]?)?Cargo\.toml$`), "Cargo.toml"},
	{regexp.MustCompile(`^(?:(?:before|after)[._-]?)?\.?golangci\.yml$`), ".golangci.yml"},
	{regexp.MustCompile(`^(?:(?:before|after)[._-]?)?\.?golangci\.yaml$`), ".golangci.yaml"},
}
