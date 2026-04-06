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
	fmt.Printf("Cases: %d | Pass: %d | Fail: %d\n\n", report.CaseCount, report.PassCount, report.FailCount)

	fmt.Println("--- Retrieval Metrics ---")
	fmt.Printf("  Mean Recall@20:     %.3f\n", report.Metrics.MeanRecallAt20)
	fmt.Printf("  Mean Top-1 Prec:    %.3f\n", report.Metrics.MeanTop1Precision)
	fmt.Printf("  BM25 Zero Rate:     %.3f\n", report.Metrics.BM25ZeroRate)
	fmt.Printf("  Mean nDCG@10:       %.3f\n", report.Metrics.MeanNDCGAt10)

	fmt.Println("\n--- Planning Metrics ---")
	fmt.Printf("  Intent Accuracy:    %.3f\n", report.Metrics.IntentAccuracy)
	fmt.Printf("  Clarify Precision:  %.3f\n", report.Metrics.ClarificationPrecision)
	fmt.Printf("  Follow-up Resolve:  %.3f\n", report.Metrics.FollowUpResolutionRate)

	fmt.Println("\n--- Generation Metrics ---")
	fmt.Printf("  Mean Faithfulness:  %.3f\n", report.Metrics.MeanFaithfulness)
	fmt.Printf("  Mean Cite Correct:  %.3f\n", report.Metrics.MeanCitationCorrectness)
	fmt.Printf("  Unsupported Claims: %.3f\n", report.Metrics.UnsupportedClaimRate)
	fmt.Printf("  Fallback Rate:      %.3f\n", report.Metrics.FallbackRate)

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

// DiffReports compares two reports and prints differences.
func DiffReports(before, after EvalReport) string {
	var sb strings.Builder
	sb.WriteString("=== Augur Eval Diff ===\n")
	sb.WriteString(fmt.Sprintf("Before: %s (%d cases)\n", before.Timestamp, before.CaseCount))
	sb.WriteString(fmt.Sprintf("After:  %s (%d cases)\n\n", after.Timestamp, after.CaseCount))

	diffMetric(&sb, "Pass Rate", passRate(before), passRate(after))
	diffMetric(&sb, "Recall@20", before.Metrics.MeanRecallAt20, after.Metrics.MeanRecallAt20)
	diffMetric(&sb, "Top-1 Precision", before.Metrics.MeanTop1Precision, after.Metrics.MeanTop1Precision)
	diffMetric(&sb, "BM25 Zero Rate", before.Metrics.BM25ZeroRate, after.Metrics.BM25ZeroRate)
	diffMetric(&sb, "Intent Accuracy", before.Metrics.IntentAccuracy, after.Metrics.IntentAccuracy)
	diffMetric(&sb, "Follow-up Resolve", before.Metrics.FollowUpResolutionRate, after.Metrics.FollowUpResolutionRate)
	diffMetric(&sb, "Faithfulness", before.Metrics.MeanFaithfulness, after.Metrics.MeanFaithfulness)
	diffMetric(&sb, "Fallback Rate", before.Metrics.FallbackRate, after.Metrics.FallbackRate)

	// Newly passing / newly failing
	beforeSet := verdictSet(before.Verdicts)
	afterSet := verdictSet(after.Verdicts)

	var newlyPassing, newlyFailing []string
	for id, passed := range afterSet {
		if prev, ok := beforeSet[id]; ok {
			if !prev && passed {
				newlyPassing = append(newlyPassing, id)
			} else if prev && !passed {
				newlyFailing = append(newlyFailing, id)
			}
		}
	}
	if len(newlyPassing) > 0 {
		sb.WriteString("\nNewly Passing:\n")
		for _, id := range newlyPassing {
			sb.WriteString(fmt.Sprintf("  + %s\n", id))
		}
	}
	if len(newlyFailing) > 0 {
		sb.WriteString("\nNewly Failing:\n")
		for _, id := range newlyFailing {
			sb.WriteString(fmt.Sprintf("  - %s\n", id))
		}
	}

	return sb.String()
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
	sb.WriteString(fmt.Sprintf("  %-20s %.3f %s %.3f (Δ %+.3f)\n", name, before, arrow, after, delta))
}

func verdictSet(verdicts []CaseVerdict) map[string]bool {
	m := make(map[string]bool, len(verdicts))
	for _, v := range verdicts {
		m[v.CaseID] = v.Passed
	}
	return m
}
