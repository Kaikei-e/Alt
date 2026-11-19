package evalconnections

import (
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
)

const (
	VerdictConnected    = "connected"
	VerdictNotConnected = "not_connected"
	VerdictUnsure       = "unsure"
)

func parseUUIDOrPanic(raw string) uuid.UUID {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		panic(fmt.Sprintf("evalconnections: invalid user UUID %q: %v", raw, err))
	}
	return id
}

// EvaluateRepresentations evaluates every representation against the scored sheet
// and chooses the representation and minimum similarity threshold proposing the
// most pairs while keeping connected_share >= 0.60 (unsure excluded from denominator).
func EvaluateRepresentations(rows []SheetRow, key KeyFile) (ScoreReport, error) {
	if len(key.Pairs) == 0 {
		return ScoreReport{}, fmt.Errorf("key file contains no pairs")
	}

	verdictsByPair := make(map[int]string, len(rows))
	for _, r := range rows {
		if _, dup := verdictsByPair[r.PairNumber]; dup {
			return ScoreReport{}, fmt.Errorf("duplicate pair_number %d in scoring sheet", r.PairNumber)
		}
		v := strings.ToLower(strings.TrimSpace(r.Verdict))
		switch v {
		case VerdictConnected, VerdictNotConnected, VerdictUnsure:
			verdictsByPair[r.PairNumber] = v
		case "":
			return ScoreReport{}, fmt.Errorf("pair_number %d has blank verdict (expected connected, not_connected, or unsure)", r.PairNumber)
		default:
			return ScoreReport{}, fmt.Errorf("pair_number %d has invalid verdict %q (expected connected, not_connected, or unsure)", r.PairNumber, r.Verdict)
		}
	}

	for _, kp := range key.Pairs {
		if _, ok := verdictsByPair[kp.PairNumber]; !ok {
			return ScoreReport{}, fmt.Errorf("scoring sheet is missing pair_number %d", kp.PairNumber)
		}
	}

	reports := make([]RepresentationReport, 0, len(CanonicalRepresentations))
	var best *Recommendation

	for _, rep := range CanonicalRepresentations {
		if status := key.Representations[rep]; status == StatusSkipped {
			reports = append(reports, RepresentationReport{
				Representation: rep,
				Status:         StatusSkipped,
			})
			continue
		}

		type scoredPair struct {
			sim     float64
			verdict string
		}
		var scored []scoredPair
		thresholdSet := make(map[float64]struct{})
		for _, kp := range key.Pairs {
			sim, ok := kp.Similarities[rep]
			if !ok {
				continue
			}
			scored = append(scored, scoredPair{
				sim:     sim,
				verdict: verdictsByPair[kp.PairNumber],
			})
			thresholdSet[sim] = struct{}{}
		}

		if len(scored) == 0 {
			reports = append(reports, RepresentationReport{
				Representation: rep,
				Status:         StatusSkipped,
			})
			continue
		}

		thresholds := make([]float64, 0, len(thresholdSet))
		for t := range thresholdSet {
			thresholds = append(thresholds, t)
		}
		sort.Float64s(thresholds)

		evals := make([]ThresholdEvaluation, 0, len(thresholds))
		for _, t := range thresholds {
			var connected, notConnected, unsure int
			for _, sp := range scored {
				if sp.sim < t {
					continue
				}
				switch sp.verdict {
				case VerdictConnected:
					connected++
				case VerdictNotConnected:
					notConnected++
				case VerdictUnsure:
					unsure++
				}
			}
			proposed := connected + notConnected + unsure
			judged := connected + notConnected
			share := 0.0
			if judged > 0 {
				share = float64(connected) / float64(judged)
			}
			meets := judged > 0 && share >= MinConnectedShare

			eval := ThresholdEvaluation{
				MinSimilarity:     t,
				ProposedCount:     proposed,
				JudgedCount:       judged,
				ConnectedCount:    connected,
				NotConnectedCount: notConnected,
				UnsureCount:       unsure,
				ConnectedShare:    share,
				MeetsTarget:       meets,
			}
			evals = append(evals, eval)

			if meets && isBetterCandidate(eval, best) {
				best = &Recommendation{
					Representation:    rep,
					MinSimilarity:     eval.MinSimilarity,
					ProposedCount:     eval.ProposedCount,
					JudgedCount:       eval.JudgedCount,
					ConnectedCount:    eval.ConnectedCount,
					NotConnectedCount: eval.NotConnectedCount,
					UnsureCount:       eval.UnsureCount,
					ConnectedShare:    eval.ConnectedShare,
				}
			}
		}

		reports = append(reports, RepresentationReport{
			Representation: rep,
			Status:         StatusAvailable,
			ScoredPairs:    len(scored),
			Thresholds:     evals,
		})
	}

	passed := best != nil
	msg := fmt.Sprintf("FAIL: no representation reached connected share >= %.2f; no UI should be built", MinConnectedShare)
	if passed {
		msg = fmt.Sprintf("PASS: representation=%s min_similarity=%.6f threshold_version=%d proposed=%d judged=%d connected_share=%.3f unsure=%d",
			best.Representation, best.MinSimilarity, ThresholdVersion, best.ProposedCount, best.JudgedCount, best.ConnectedShare, best.UnsureCount)
	}

	return ScoreReport{
		Passed:               passed,
		ThresholdVersion:     ThresholdVersion,
		TargetConnectedShare: MinConnectedShare,
		Recommendation:       best,
		Message:              msg,
		Representations:      reports,
	}, nil
}

func isBetterCandidate(cand ThresholdEvaluation, current *Recommendation) bool {
	if current == nil {
		return true
	}
	if cand.ProposedCount != current.ProposedCount {
		return cand.ProposedCount > current.ProposedCount
	}
	if cand.ConnectedShare != current.ConnectedShare {
		return cand.ConnectedShare > current.ConnectedShare
	}
	return cand.JudgedCount > current.JudgedCount
}

// FormatScoreReport renders a human-readable summary of the score report.
func FormatScoreReport(report ScoreReport) string {
	var sb strings.Builder
	sb.WriteString("=== Reading Connections Evaluation ===\n")
	if report.Passed && report.Recommendation != nil {
		rec := report.Recommendation
		fmt.Fprintf(&sb, "Result:            PASS\n")
		fmt.Fprintf(&sb, "Representation:    %s\n", rec.Representation)
		fmt.Fprintf(&sb, "Min Similarity:    %.6f\n", rec.MinSimilarity)
		fmt.Fprintf(&sb, "Threshold Version: %d\n", report.ThresholdVersion)
		fmt.Fprintf(&sb, "Proposed Pairs:    %d (judged=%d, connected=%d, not_connected=%d, unsure=%d, connected_share=%.3f)\n\n",
			rec.ProposedCount, rec.JudgedCount, rec.ConnectedCount, rec.NotConnectedCount, rec.UnsureCount, rec.ConnectedShare)
	} else {
		fmt.Fprintf(&sb, "Result:            FAIL\n")
		fmt.Fprintf(&sb, "Threshold Version: %d\n", report.ThresholdVersion)
		fmt.Fprintf(&sb, "Decision:          no representation reached %.0f%% connected share; no UI should be built\n\n", report.TargetConnectedShare*100)
	}

	for _, rr := range report.Representations {
		if rr.Status == StatusSkipped {
			fmt.Fprintf(&sb, "- %s: skipped\n", rr.Representation)
			continue
		}
		fmt.Fprintf(&sb, "- %s (%d scored pairs):\n", rr.Representation, rr.ScoredPairs)
		for _, th := range rr.Thresholds {
			fmt.Fprintf(&sb, "    min_sim=%.6f proposed=%2d judged=%2d connected=%2d unsure=%2d share=%.3f meets=%v\n",
				th.MinSimilarity, th.ProposedCount, th.JudgedCount, th.ConnectedCount, th.UnsureCount, th.ConnectedShare, th.MeetsTarget)
		}
	}
	return sb.String()
}
