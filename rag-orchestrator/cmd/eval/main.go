// Augur eval benchmark runner — replays golden cases through the live pipeline
// via Connect-RPC and reports retrieval, rerank and generation scores separately.
//
// Two profiles run over the same golden set produce two reports that -baseline
// turns into an A/B delta table.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
	"unicode/utf8"

	"connectrpc.com/connect"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	"rag-orchestrator/eval"
)

// evalUserID is the synthetic caller identity for eval runs against
// StreamChat, which requires an authenticated X-Alt-User-Id header.
const evalUserID = "00000000-0000-0000-0000-000000000001"

// ragDBDSNEnv holds the read-only connection string the golden case generator uses.
const ragDBDSNEnv = "EVAL_RAG_DB_DSN"

const (
	defaultGoldenPath   = "eval/testdata/golden_cases.json"
	defaultProfilesPath = "eval/testdata/profiles.json"
	defaultGeneratePath = "eval/testdata/golden_cases.local.json"
	caseTimeout         = 120 * time.Second
)

type options struct {
	profileName  string
	profilesPath string
	goldenPath   string
	addr         string
	reportPath   string
	baselinePath string
	diffPath     string
	generate     bool
	generateOut  string
	generateFrom string
	generateMax  int
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	opts := parseFlags()

	if opts.generate {
		since, err := time.Parse("2006-01-02", opts.generateFrom)
		if err != nil {
			logger.Error("invalid_generate_since", "value", opts.generateFrom, "error", err)
			os.Exit(1)
		}
		cfg := generateConfig{
			DSN:            os.Getenv(ragDBDSNEnv),
			OutputPath:     opts.generateOut,
			Since:          since,
			MaxCases:       opts.generateMax,
			RelevantPerCas: 5,
		}
		if err := runGenerate(context.Background(), cfg, logger); err != nil {
			logger.Error("generate_failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := run(opts, logger); err != nil {
		logger.Error("eval_failed", "error", err)
		os.Exit(1)
	}
}

func parseFlags() options {
	var opts options
	flag.StringVar(&opts.profileName, "profile", "baseline", "profile name to run, from the profiles file")
	flag.StringVar(&opts.profilesPath, "profiles", defaultProfilesPath, "path to the A/B profile definitions")
	flag.StringVar(&opts.goldenPath, "golden", defaultGoldenPath,
		"path to the golden case file; "+eval.GoldenCasesPathEnv+" overrides this default")
	flag.StringVar(&opts.addr, "addr", "", "Augur address; defaults to the profile's augur_addr")
	flag.StringVar(&opts.reportPath, "report", "",
		"write the JSON report here; empty prints only, because a report over the real corpus carries article titles and answers")
	flag.StringVar(&opts.baselinePath, "baseline", "", "report to diff this run against")
	flag.StringVar(&opts.diffPath, "diff", "", "write the A/B diff JSON here")
	flag.BoolVar(&opts.generate, "generate", false, "mine the live corpus into a local golden case file instead of running the eval")
	flag.StringVar(&opts.generateOut, "generate-out", defaultGeneratePath, "output path for -generate (kept out of version control)")
	flag.StringVar(&opts.generateFrom, "generate-since", "2026-01-01", "mine conversations from this date (YYYY-MM-DD)")
	flag.IntVar(&opts.generateMax, "generate-max", 60, "maximum number of cases to mine")
	flag.Parse()
	return opts
}

func run(opts options, logger *slog.Logger) error {
	profiles, err := eval.LoadProfiles(opts.profilesPath)
	if err != nil {
		return err
	}
	profile, err := profiles.Select(opts.profileName)
	if err != nil {
		return err
	}
	if opts.addr != "" {
		profile.AugurAddr = opts.addr
	}

	goldenPath := eval.ResolveGoldenCasesPath(opts.goldenPath)
	cases, err := eval.LoadGoldenCases(goldenPath)
	if err != nil {
		return err
	}

	logger.Info("eval_started",
		"profile", profile.Name,
		"embedder_model", profile.Embedder.Model,
		"rerank_enabled", profile.Rerank.Enabled,
		"augur_addr", profile.AugurAddr,
		"golden_path", goldenPath,
		"cases", len(cases))

	// StreamChat is a long-lived response; the per-case context deadline bounds
	// it instead of a client-wide timeout.
	client := augurv2connect.NewAugurServiceClient(&http.Client{}, profile.AugurAddr)

	results := make(map[string]eval.EvalResult, len(cases))
	for _, gc := range cases {
		result := runCase(client, gc, logger)
		results[gc.ID] = result

		logger.Info("case_completed",
			"case_id", gc.ID,
			"category", gc.Category,
			"fallback", result.IsFallback,
			"fallback_reason", result.FallbackReason,
			"answer_runes", result.AnswerLength,
			"citations", result.CitationCount)
	}

	report := eval.RunOfflineEval(cases, results)
	report.Profile = profile.Summary()
	eval.PrintReport(report)

	if opts.reportPath != "" {
		if err := eval.SaveReport(report, opts.reportPath); err != nil {
			return err
		}
		logger.Info("report_saved", "path", opts.reportPath)
	}

	if opts.baselinePath != "" {
		baseline, err := eval.LoadReport(opts.baselinePath)
		if err != nil {
			return err
		}
		diff := eval.ComputeDiff(baseline, report)
		fmt.Print(diff.String())
		if opts.diffPath != "" {
			if err := eval.SaveDiff(diff, opts.diffPath); err != nil {
				return err
			}
			logger.Info("diff_saved", "path", opts.diffPath)
		}
	}

	return nil
}

func runCase(client augurv2connect.AugurServiceClient, gc eval.GoldenCase, logger *slog.Logger) eval.EvalResult {
	result := eval.EvalResult{CaseID: gc.ID}

	ctx, cancel := context.WithTimeout(context.Background(), caseTimeout)
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

	req := connect.NewRequest(&augurv2.StreamChatRequest{
		Messages: msgs,
	})
	// The handler rejects unauthenticated StreamChat calls (extractUserID).
	// A fixed eval user id keeps every golden-case run attributable to the
	// same synthetic account without minting one per case.
	req.Header().Set("X-Alt-User-Id", evalUserID)

	stream, err := client.StreamChat(ctx, req)
	if err != nil {
		result.IsFallback = true
		result.FallbackReason = fmt.Sprintf("stream error: %v", err)
		return result
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil {
			logger.Warn("stream_close_failed", "case_id", gc.ID, "error", closeErr)
		}
	}()

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
					result.CitedArticleIDs = append(result.CitedArticleIDs, c.RefId)
				}
				result.CitationCount = len(d.Citations)
				result.IntentClassified = d.Intent
			}
		case "meta":
			if m := resp.GetMeta(); m != nil {
				for _, c := range m.Citations {
					result.RetrievedTitles = append(result.RetrievedTitles, c.Title)
					result.RetrievedArticleIDs = append(result.RetrievedArticleIDs, c.RefId)
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
		case "clarification":
			result.ClarificationAsked = true
		}
	}

	if err := stream.Err(); err != nil && !errors.Is(err, context.Canceled) {
		if !result.IsFallback {
			result.IsFallback = true
			result.FallbackReason = fmt.Sprintf("stream read: %v", err)
		}
	}

	result.Answer = fullAnswer
	result.AnswerLength = utf8.RuneCountInString(fullAnswer)

	// StreamChat exposes the retrieval set in meta order only; the pre-rerank
	// ordering is not on the wire, so the rerank delta is measurable through the
	// in-process PipelineAdapter rather than this replay path.
	result.PreRerankArticleIDs = nil

	return result
}
