// Augur eval benchmark runner — calls the live pipeline via Connect-RPC and produces an eval report.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"strings"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"connectrpc.com/connect"

	"rag-orchestrator/eval"
)

func main() {
	goldenPath := "eval/testdata/golden_cases.json"
	reportPath := "eval/testdata/baseline_report.json"
	augurAddr := "http://localhost:9011"

	if len(os.Args) > 1 {
		augurAddr = os.Args[1]
	}

	cases, err := eval.LoadGoldenCases(goldenPath)
	if err != nil {
		log.Fatalf("failed to load golden cases: %v", err)
	}

	client := augurv2connect.NewAugurServiceClient(
		http.DefaultClient,
		augurAddr,
	)

	results := make(map[string]eval.EvalResult, len(cases))

	for _, gc := range cases {
		fmt.Printf("--- %s: %s\n", gc.ID, gc.Query)
		result := runCase(client, gc)
		results[gc.ID] = result

		status := "OK"
		if result.IsFallback {
			status = "FALLBACK"
		}
		fmt.Printf("    %s | answer_len=%d citations=%d\n",
			status, result.AnswerLength, result.CitationCount)
		if result.IsFallback {
			fmt.Printf("    reason: %s\n", result.FallbackReason)
		}
	}

	report := eval.RunOfflineEval(cases, results)
	fmt.Println()
	eval.PrintReport(report)

	if err := eval.SaveReport(report, reportPath); err != nil {
		log.Fatalf("failed to save report: %v", err)
	}
	fmt.Printf("Report saved to %s\n", reportPath)

	// Save detailed markdown report
	mdPath := "eval/testdata/baseline_detailed.md"
	if err := writeDetailedMarkdown(mdPath, cases, results, report); err != nil {
		log.Fatalf("failed to write detailed report: %v", err)
	}
	fmt.Printf("Detailed report saved to %s\n", mdPath)
}

func writeDetailedMarkdown(path string, cases []eval.GoldenCase, results map[string]eval.EvalResult, report eval.EvalReport) error {
	var sb strings.Builder

	sb.WriteString("# Augur Baseline Eval Report\n\n")
	sb.WriteString(fmt.Sprintf("**Timestamp:** %s\n\n", report.Timestamp))
	sb.WriteString(fmt.Sprintf("**Cases:** %d | **Pass:** %d | **Fail:** %d | **Pass Rate:** %.0f%%\n\n",
		report.CaseCount, report.PassCount, report.FailCount,
		float64(report.PassCount)/float64(report.CaseCount)*100))

	sb.WriteString("---\n\n## Aggregate Metrics\n\n")
	sb.WriteString("### Retrieval\n\n")
	sb.WriteString("| Metric | Value |\n|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Mean Recall@20 | %.3f |\n", report.Metrics.MeanRecallAt20))
	sb.WriteString(fmt.Sprintf("| Mean nDCG@10 | %.3f |\n", report.Metrics.MeanNDCGAt10))
	sb.WriteString(fmt.Sprintf("| Mean Top-1 Precision | %.3f |\n", report.Metrics.MeanTop1Precision))
	sb.WriteString(fmt.Sprintf("| BM25 Zero Rate | %.3f |\n", report.Metrics.BM25ZeroRate))

	sb.WriteString("\n### Planning\n\n")
	sb.WriteString("| Metric | Value |\n|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Intent Accuracy | %.3f |\n", report.Metrics.IntentAccuracy))
	sb.WriteString(fmt.Sprintf("| Clarification Precision | %.3f |\n", report.Metrics.ClarificationPrecision))
	sb.WriteString(fmt.Sprintf("| Follow-up Resolution Rate | %.3f |\n", report.Metrics.FollowUpResolutionRate))

	sb.WriteString("\n### Generation\n\n")
	sb.WriteString("| Metric | Value |\n|--------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Mean Faithfulness | %.3f |\n", report.Metrics.MeanFaithfulness))
	sb.WriteString(fmt.Sprintf("| Mean Citation Correctness | %.3f |\n", report.Metrics.MeanCitationCorrectness))
	sb.WriteString(fmt.Sprintf("| Unsupported Claim Rate | %.3f |\n", report.Metrics.UnsupportedClaimRate))
	sb.WriteString(fmt.Sprintf("| Fallback Rate | %.3f |\n", report.Metrics.FallbackRate))

	sb.WriteString("\n---\n\n## Per-Case Results\n\n")

	for _, gc := range cases {
		result, ok := results[gc.ID]
		if !ok {
			continue
		}

		verdict := findVerdict(report.Verdicts, gc.ID)
		status := "PASS"
		if !verdict.Passed {
			status = "FAIL"
		}

		sb.WriteString(fmt.Sprintf("### %s `%s`\n\n", status, gc.ID))
		sb.WriteString(fmt.Sprintf("**Query:** %s\n\n", gc.Query))

		if len(gc.ConversationHistory) > 0 {
			sb.WriteString("**Conversation History:**\n\n")
			for _, msg := range gc.ConversationHistory {
				sb.WriteString(fmt.Sprintf("- **%s:** %s\n", msg.Role, msg.Content))
			}
			sb.WriteString("\n")
		}

		if len(gc.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("**Tags:** %s\n\n", strings.Join(gc.Tags, ", ")))
		}

		// Result summary table
		sb.WriteString("| Field | Value |\n|-------|-------|\n")
		sb.WriteString(fmt.Sprintf("| Answer Length | %d runes |\n", result.AnswerLength))
		sb.WriteString(fmt.Sprintf("| Citations | %d |\n", result.CitationCount))
		sb.WriteString(fmt.Sprintf("| Fallback | %v |\n", result.IsFallback))
		if result.FallbackReason != "" {
			sb.WriteString(fmt.Sprintf("| Fallback Reason | %s |\n", result.FallbackReason))
		}
		if len(result.RetrievedTitles) > 0 {
			sb.WriteString(fmt.Sprintf("| Retrieved Titles | %s |\n", strings.Join(result.RetrievedTitles, "; ")))
		}
		if len(result.CitedTitles) > 0 {
			sb.WriteString(fmt.Sprintf("| Cited Titles | %s |\n", strings.Join(result.CitedTitles, "; ")))
		}
		sb.WriteString("\n")

		// Expectations vs actual
		if !verdict.Passed {
			sb.WriteString("**Failures:**\n\n")
			for _, f := range verdict.Failures {
				sb.WriteString(fmt.Sprintf("- %s\n", f))
			}
			sb.WriteString("\n")
		}

		// Answer text (truncated to 500 chars for readability)
		if result.Answer != "" {
			answerPreview := result.Answer
			runes := []rune(answerPreview)
			if len(runes) > 500 {
				answerPreview = string(runes[:500]) + "..."
			}
			sb.WriteString("<details><summary>Answer (preview)</summary>\n\n")
			sb.WriteString("```\n")
			sb.WriteString(answerPreview)
			sb.WriteString("\n```\n\n</details>\n\n")
		}

		sb.WriteString("---\n\n")
	}

	sb.WriteString("## Analysis\n\n")
	sb.WriteString("### Systemic Issues\n\n")

	// Count failure types
	citationFail := 0
	intentFail := 0
	lengthFail := 0
	clarifyFail := 0
	for _, v := range report.Verdicts {
		for _, f := range v.Failures {
			if strings.Contains(f, "citations") {
				citationFail++
			}
			if strings.Contains(f, "intent") {
				intentFail++
			}
			if strings.Contains(f, "answer length") {
				lengthFail++
			}
			if strings.Contains(f, "clarification") {
				clarifyFail++
			}
		}
	}
	sb.WriteString(fmt.Sprintf("| Issue | Count | Impact |\n|-------|-------|--------|\n"))
	sb.WriteString(fmt.Sprintf("| Citation not returned in stream | %d/%d | done event の citations が空。rag-orchestrator → frontend の citation 伝搬に問題 |\n", citationFail, report.CaseCount))
	sb.WriteString(fmt.Sprintf("| Intent not exposed in response | %d/%d | StreamChat が intent debug 情報を返していない |\n", intentFail, report.CaseCount))
	sb.WriteString(fmt.Sprintf("| Answer too short | %d/%d | follow-up・topic-shift で retrieval が不十分 |\n", lengthFail, report.CaseCount))
	sb.WriteString(fmt.Sprintf("| Clarification not triggered | %d/%d | ConversationPlanner が曖昧クエリで clarification を返さない |\n", clarifyFail, report.CaseCount))

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func findVerdict(verdicts []eval.CaseVerdict, caseID string) eval.CaseVerdict {
	for _, v := range verdicts {
		if v.CaseID == caseID {
			return v
		}
	}
	return eval.CaseVerdict{CaseID: caseID, Passed: false, Failures: []string{"not found"}}
}

func runCase(client augurv2connect.AugurServiceClient, gc eval.GoldenCase) eval.EvalResult {
	result := eval.EvalResult{CaseID: gc.ID}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var msgs []*augurv2.ChatMessage

	for _, msg := range gc.ConversationHistory {
		msgs = append(msgs, &augurv2.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	query := gc.Query
	if gc.ArticleScope != nil {
		query = fmt.Sprintf("Regarding the article: %s [articleId: %s]\n\nQuestion:\n%s",
			gc.ArticleScope.Title, gc.ArticleScope.ArticleID, gc.Query)
	}
	msgs = append(msgs, &augurv2.ChatMessage{
		Role:    "user",
		Content: query,
	})

	stream, err := client.StreamChat(ctx, connect.NewRequest(&augurv2.StreamChatRequest{
		Messages: msgs,
	}))
	if err != nil {
		result.IsFallback = true
		result.FallbackReason = fmt.Sprintf("stream error: %v", err)
		return result
	}
	defer stream.Close()

	var fullAnswer string
	for stream.Receive() {
		resp := stream.Msg()
		switch resp.Kind {
		case "delta":
			delta := resp.GetDelta()
			if delta != "" {
				fullAnswer += delta
			}
		case "done":
			if d := resp.GetDone(); d != nil {
				if d.Answer != "" {
					fullAnswer = d.Answer
				}
				for _, c := range d.Citations {
					result.CitedTitles = append(result.CitedTitles, c.Title)
				}
				result.CitationCount = len(d.Citations)
				result.IntentClassified = d.Intent
			}
		case "meta":
			if m := resp.GetMeta(); m != nil {
				for _, c := range m.Citations {
					result.RetrievedTitles = append(result.RetrievedTitles, c.Title)
				}
			}
		case "fallback":
			code := resp.GetFallbackCode()
			if code != "" {
				result.IsFallback = true
				result.FallbackReason = code
			}
		case "error":
			msg := resp.GetErrorMessage()
			if msg != "" {
				result.IsFallback = true
				result.FallbackReason = msg
			}
		}
	}

	if err := stream.Err(); err != nil {
		if !result.IsFallback {
			result.IsFallback = true
			result.FallbackReason = fmt.Sprintf("stream read: %v", err)
		}
	}

	result.Answer = fullAnswer
	result.AnswerLength = utf8.RuneCountInString(fullAnswer)

	return result
}
