package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// LoadGoldenCases reads golden cases from a JSON file.
func LoadGoldenCases(path string) ([]GoldenCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read golden cases: %w", err)
	}
	var cases []GoldenCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("parse golden cases: %w", err)
	}
	return cases, nil
}

// RunOfflineEval runs the golden cases through verification and produces a report.
// results must be keyed by case ID.
func RunOfflineEval(cases []GoldenCase, results map[string]EvalResult) EvalReport {
	report := EvalReport{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		CaseCount: len(cases),
	}

	var (
		totalRecall       float64
		totalTop1         float64
		totalFaithfulness float64
		totalCitCorrect   float64
		bm25ZeroCount     int
		fallbackCount     int
		intentCorrect     int
		intentTotal       int
		clarifyCorrect    int
		clarifyTotal      int
		followUpResolved  int
		followUpTotal     int
		faithfulnessCount int
		citCorrectCount   int

		// Phase 0 additions
		structureAdherent int
		structureTotal    int
		totalPromptTokens float64
		promptTokenCount  int
	)

	for _, gc := range cases {
		result, ok := results[gc.ID]
		if !ok {
			report.Verdicts = append(report.Verdicts, CaseVerdict{
				CaseID:   gc.ID,
				Passed:   false,
				Failures: []string{"no result found for case"},
			})
			report.FailCount++
			continue
		}

		verdict := VerifyCase(gc, result)
		report.Verdicts = append(report.Verdicts, verdict)
		if verdict.Passed {
			report.PassCount++
		} else {
			report.FailCount++
		}

		// Aggregate metrics
		if len(gc.Expected.ExpectedTopicKeywords) > 0 {
			totalRecall += RecallAtK(gc.Expected.ExpectedTopicKeywords, result.RetrievedTitles, 20)
		}
		totalTop1 += Top1Precision(gc.Expected.ExpectedTopicKeywords, result.RetrievedTitles)

		if result.BM25HitCount == 0 {
			bm25ZeroCount++
		}
		if result.IsFallback {
			fallbackCount++
		}

		// Intent accuracy
		if gc.Expected.ExpectedIntent != "" {
			intentTotal++
			if result.IntentClassified == gc.Expected.ExpectedIntent {
				intentCorrect++
			}
		}

		// Clarification precision
		if gc.Expected.ShouldClarify || result.ClarificationAsked {
			clarifyTotal++
			if gc.Expected.ShouldClarify == result.ClarificationAsked {
				clarifyCorrect++
			}
		}

		// Follow-up resolution
		if len(gc.ConversationHistory) > 0 && !gc.Expected.ShouldClarify {
			followUpTotal++
			if !result.IsFallback && result.AnswerLength >= gc.Expected.MinAnswerLength {
				followUpResolved++
			}
		}

		// Faithfulness
		if len(gc.Expected.ExpectedEntities) > 0 && result.Answer != "" {
			// Build context from retrieved titles (simplified: use titles as context proxy)
			contexts := result.RetrievedTitles
			f := Faithfulness(result.Answer, contexts, gc.Expected.ExpectedEntities)
			totalFaithfulness += f
			faithfulnessCount++
		}

		// Citation correctness
		if len(result.CitedTitles) > 0 && len(gc.Expected.ExpectedTopicKeywords) > 0 {
			cc := CitationCorrectness(result.CitedTitles, gc.Expected.ExpectedTopicKeywords)
			totalCitCorrect += cc
			citCorrectCount++
		}

		// Instruction adherence: check if all expected structures are present
		if len(gc.Expected.ExpectedStructure) > 0 {
			structureTotal++
			allPresent := true
			for _, s := range gc.Expected.ExpectedStructure {
				if !strings.Contains(result.Answer, s) {
					allPresent = false
					break
				}
			}
			if allPresent {
				structureAdherent++
			}
		}

		// Prompt token tracking
		if result.PromptTokenCount > 0 {
			totalPromptTokens += float64(result.PromptTokenCount)
			promptTokenCount++
		}
	}

	n := float64(len(cases))
	if n > 0 {
		report.Metrics.MeanRecallAt20 = totalRecall / n
		report.Metrics.MeanTop1Precision = totalTop1 / n
		report.Metrics.BM25ZeroRate = float64(bm25ZeroCount) / n
		report.Metrics.FallbackRate = float64(fallbackCount) / n
	}
	if intentTotal > 0 {
		report.Metrics.IntentAccuracy = float64(intentCorrect) / float64(intentTotal)
	}
	if clarifyTotal > 0 {
		report.Metrics.ClarificationPrecision = float64(clarifyCorrect) / float64(clarifyTotal)
	}
	if followUpTotal > 0 {
		report.Metrics.FollowUpResolutionRate = float64(followUpResolved) / float64(followUpTotal)
	}
	if faithfulnessCount > 0 {
		report.Metrics.MeanFaithfulness = totalFaithfulness / float64(faithfulnessCount)
	}
	if citCorrectCount > 0 {
		report.Metrics.MeanCitationCorrectness = totalCitCorrect / float64(citCorrectCount)
	}
	if structureTotal > 0 {
		report.Metrics.InstructionAdherenceRate = float64(structureAdherent) / float64(structureTotal)
	}
	if promptTokenCount > 0 {
		report.Metrics.MeanPromptTokens = totalPromptTokens / float64(promptTokenCount)
	}

	return report
}

// SaveReport writes the eval report as JSON.
func SaveReport(report EvalReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
