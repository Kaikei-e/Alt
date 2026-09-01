package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PrintReport prints a human-readable eval report to stdout.
func PrintReport(report EvalReport) {
	fmt.Println("=== Augur Eval Report ===")
	fmt.Printf("Timestamp: %s\n", report.Timestamp)
	fmt.Printf("Profile:   %s (embedder=%s/%dd rerank=%v %s alpha=%.2f)\n",
		report.Profile.Name, report.Profile.EmbedderModel, report.Profile.EmbedderDims,
		report.Profile.RerankEnabled, report.Profile.RerankModel, report.Profile.HybridAlpha)
	fmt.Printf("Cases: %d | Pass: %d | Fail: %d\n\n", report.CaseCount, report.PassCount, report.FailCount)

	r := report.Stages.Retrieval
	fmt.Printf("--- Retrieval Stage (%d scored cases) ---\n", r.CaseCount)
	fmt.Printf("  Recall@5 / @10 / @20: %.3f / %.3f / %.3f\n", r.MeanRecallAt5, r.MeanRecallAt10, r.MeanRecallAt20)
	fmt.Printf("  nDCG@10:              %.3f\n", r.MeanNDCGAt10)
	fmt.Printf("  MRR:                  %.3f\n", r.MeanMRR)
	fmt.Printf("  BM25 Zero Rate:       %.3f\n", r.BM25ZeroRate)
	fmt.Printf("  Forbidden Hit Rate:   %.3f\n", r.ForbiddenHitRate)

	rr := report.Stages.Rerank
	fmt.Printf("\n--- Rerank Stage (%d scored cases, applied %.0f%%) ---\n", rr.CaseCount, rr.AppliedRate*100)
	fmt.Printf("  nDCG@10 before:       %.3f\n", rr.MeanNDCGAt10Before)
	fmt.Printf("  nDCG@10 after:        %.3f\n", rr.MeanNDCGAt10After)
	fmt.Printf("  nDCG@10 delta:        %+.3f\n", rr.MeanNDCGAt10Delta)

	g := report.Stages.Generation
	fmt.Printf("\n--- Generation Stage (%d scored cases) ---\n", g.CaseCount)
	fmt.Printf("  Faithfulness:         %.3f\n", g.MeanFaithfulness)
	fmt.Printf("  Citation Correctness: %.3f\n", g.MeanCitationCorrectness)
	fmt.Printf("  Citation Recall:      %.3f\n", g.CitationRecall)
	fmt.Printf("  Forbidden Cite Rate:  %.3f\n", g.ForbiddenCitationRate)
	fmt.Printf("  No-Answer Honesty:    %.3f\n", g.NoAnswerHonestyRate)
	fmt.Printf("  Fallback Rate:        %.3f\n", g.FallbackRate)

	fmt.Println("\n--- Planning Metrics ---")
	fmt.Printf("  Intent Accuracy:    %.3f\n", report.Metrics.IntentAccuracy)
	fmt.Printf("  Clarify Precision:  %.3f\n", report.Metrics.ClarificationPrecision)
	fmt.Printf("  Follow-up Resolve:  %.3f\n", report.Metrics.FollowUpResolutionRate)
	fmt.Printf("  Top-1 Precision:    %.3f\n", report.Metrics.MeanTop1Precision)

	if len(report.Categories) > 0 {
		fmt.Println("\n--- Pass Rate by Category ---")
		for _, name := range KnownCategories {
			c, ok := report.Categories[name]
			if !ok || c.CaseCount == 0 {
				continue
			}
			fmt.Printf("  %-16s %2d/%2d (%.0f%%)\n", name, c.PassCount, c.CaseCount,
				float64(c.PassCount)/float64(c.CaseCount)*100)
		}
	}

	if report.FailCount > 0 {
		fmt.Println("\n--- Failed Cases ---")
		for _, v := range report.Verdicts {
			if !v.Passed {
				fmt.Printf("  FAIL %s:\n", v.CaseID)
				for _, f := range v.Failures {
					fmt.Printf("    - %s\n", f)
				}
			}
		}
	}
	fmt.Println()
}

// MetricDelta is one metric moved between two runs.
type MetricDelta struct {
	Name   string  `json:"name"`
	Stage  string  `json:"stage"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Delta  float64 `json:"delta"`
}

// ReportDiff is the machine-readable comparison of two eval runs. It is what an
// A/B decision is made from: same golden set, two profiles, one delta table.
type ReportDiff struct {
	BeforeProfile   string        `json:"before_profile"`
	AfterProfile    string        `json:"after_profile"`
	BeforeTimestamp string        `json:"before_timestamp"`
	AfterTimestamp  string        `json:"after_timestamp"`
	BeforeCaseCount int           `json:"before_case_count"`
	AfterCaseCount  int           `json:"after_case_count"`
	Metrics         []MetricDelta `json:"metrics"`
	NewlyPassing    []string      `json:"newly_passing,omitempty"`
	NewlyFailing    []string      `json:"newly_failing,omitempty"`
}

// ComputeDiff builds the per-stage delta table and the verdict moves between two runs.
func ComputeDiff(before, after EvalReport) ReportDiff {
	diff := ReportDiff{
		BeforeProfile:   before.Profile.Name,
		AfterProfile:    after.Profile.Name,
		BeforeTimestamp: before.Timestamp,
		AfterTimestamp:  after.Timestamp,
		BeforeCaseCount: before.CaseCount,
		AfterCaseCount:  after.CaseCount,
	}

	add := func(stage, name string, b, a float64) {
		diff.Metrics = append(diff.Metrics, MetricDelta{
			Name:   name,
			Stage:  stage,
			Before: b,
			After:  a,
			Delta:  a - b,
		})
	}

	add("overall", "overall.pass_rate", passRate(before), passRate(after))

	br, ar := before.Stages.Retrieval, after.Stages.Retrieval
	add("retrieval", "retrieval.recall@5", br.MeanRecallAt5, ar.MeanRecallAt5)
	add("retrieval", "retrieval.recall@10", br.MeanRecallAt10, ar.MeanRecallAt10)
	add("retrieval", "retrieval.recall@20", br.MeanRecallAt20, ar.MeanRecallAt20)
	add("retrieval", "retrieval.ndcg@10", br.MeanNDCGAt10, ar.MeanNDCGAt10)
	add("retrieval", "retrieval.mrr", br.MeanMRR, ar.MeanMRR)
	add("retrieval", "retrieval.bm25_zero_rate", br.BM25ZeroRate, ar.BM25ZeroRate)
	add("retrieval", "retrieval.forbidden_hit_rate", br.ForbiddenHitRate, ar.ForbiddenHitRate)

	brr, arr := before.Stages.Rerank, after.Stages.Rerank
	add("rerank", "rerank.ndcg@10_before", brr.MeanNDCGAt10Before, arr.MeanNDCGAt10Before)
	add("rerank", "rerank.ndcg@10_after", brr.MeanNDCGAt10After, arr.MeanNDCGAt10After)
	add("rerank", "rerank.ndcg@10_delta", brr.MeanNDCGAt10Delta, arr.MeanNDCGAt10Delta)

	bg, ag := before.Stages.Generation, after.Stages.Generation
	add("generation", "generation.faithfulness", bg.MeanFaithfulness, ag.MeanFaithfulness)
	add("generation", "generation.citation_correctness", bg.MeanCitationCorrectness, ag.MeanCitationCorrectness)
	add("generation", "generation.citation_recall", bg.CitationRecall, ag.CitationRecall)
	add("generation", "generation.forbidden_citation_rate", bg.ForbiddenCitationRate, ag.ForbiddenCitationRate)
	add("generation", "generation.no_answer_honesty", bg.NoAnswerHonestyRate, ag.NoAnswerHonestyRate)
	add("generation", "generation.fallback_rate", bg.FallbackRate, ag.FallbackRate)

	add("planning", "planning.intent_accuracy", before.Metrics.IntentAccuracy, after.Metrics.IntentAccuracy)
	add("planning", "planning.follow_up_resolution", before.Metrics.FollowUpResolutionRate, after.Metrics.FollowUpResolutionRate)

	beforeSet := verdictSet(before.Verdicts)
	afterSet := verdictSet(after.Verdicts)
	for _, v := range after.Verdicts {
		prev, ok := beforeSet[v.CaseID]
		if !ok {
			continue
		}
		switch {
		case !prev && afterSet[v.CaseID]:
			diff.NewlyPassing = append(diff.NewlyPassing, v.CaseID)
		case prev && !afterSet[v.CaseID]:
			diff.NewlyFailing = append(diff.NewlyFailing, v.CaseID)
		}
	}

	return diff
}

// String renders the diff for a terminal.
func (d ReportDiff) String() string {
	var sb strings.Builder
	sb.WriteString("=== Augur Eval Diff ===\n")
	fmt.Fprintf(&sb, "Before: %s @ %s (%d cases)\n", d.BeforeProfile, d.BeforeTimestamp, d.BeforeCaseCount)
	fmt.Fprintf(&sb, "After:  %s @ %s (%d cases)\n\n", d.AfterProfile, d.AfterTimestamp, d.AfterCaseCount)

	stage := ""
	for _, m := range d.Metrics {
		if m.Stage != stage {
			stage = m.Stage
			fmt.Fprintf(&sb, "[%s]\n", stage)
		}
		diffMetric(&sb, m.Name, m.Before, m.After)
	}

	if len(d.NewlyPassing) > 0 {
		sb.WriteString("\nNewly Passing:\n")
		for _, id := range d.NewlyPassing {
			fmt.Fprintf(&sb, "  + %s\n", id)
		}
	}
	if len(d.NewlyFailing) > 0 {
		sb.WriteString("\nNewly Failing:\n")
		for _, id := range d.NewlyFailing {
			fmt.Fprintf(&sb, "  - %s\n", id)
		}
	}

	return sb.String()
}

// DiffReports compares two reports and renders the differences.
func DiffReports(before, after EvalReport) string {
	return ComputeDiff(before, after).String()
}

// SaveDiff writes the A/B comparison as JSON.
func SaveDiff(diff ReportDiff, path string) error {
	data, err := json.MarshalIndent(diff, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal diff: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write diff %s: %w", path, err)
	}
	return nil
}

// LoadReport reads a saved JSON report.
func LoadReport(path string) (EvalReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return EvalReport{}, fmt.Errorf("read report: %w", err)
	}
	var report EvalReport
	if err := json.Unmarshal(data, &report); err != nil {
		return EvalReport{}, fmt.Errorf("parse report: %w", err)
	}
	return report, nil
}

func passRate(r EvalReport) float64 {
	if r.CaseCount == 0 {
		return 0
	}
	return float64(r.PassCount) / float64(r.CaseCount)
}

func diffMetric(sb *strings.Builder, name string, before, after float64) {
	delta := after - before
	arrow := "→"
	if delta > 0.001 {
		arrow = "↑"
	} else if delta < -0.001 {
		arrow = "↓"
	}
	fmt.Fprintf(sb, "  %-34s %.3f %s %.3f (Δ %+.3f)\n", name, before, arrow, after, delta)
}

func verdictSet(verdicts []CaseVerdict) map[string]bool {
	m := make(map[string]bool, len(verdicts))
	for _, v := range verdicts {
		m[v.CaseID] = v.Passed
	}
	return m
}
