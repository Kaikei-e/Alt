// eval-connections is a read-only CLI for sampling 48 article pairs from
// rag-db and scoring human verdicts to choose a similarity representation and
// minimum threshold.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"rag-orchestrator/internal/adapter/rag_augur"
	"rag-orchestrator/internal/adapter/repository"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/evalconnections"
	"rag-orchestrator/internal/infra"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/httpclient"
	"rag-orchestrator/internal/usecase"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type sampleFlags struct {
	readIDsPath   string
	outDir        string
	seed          int64
	titlesPath    string
	summariesPath string
	userID        string
}

type scoreFlags struct {
	sheetPath   string
	keyPath     string
	jsonOnly    bool
	jsonOutPath string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:          "eval-connections",
		Short:        "Read-only CLI to sample and score reading-connection article pairs",
		SilenceUsage: true,
	}

	var sf sampleFlags
	sampleCmd := &cobra.Command{
		Use:   "sample",
		Short: "Sample 48 article pairs and write a blind scoring sheet CSV and JSON key",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSample(sf)
		},
	}
	sampleCmd.Flags().StringVar(&sf.readIDsPath, "read-ids", "", "path to file containing one read article ID per line (required)")
	sampleCmd.Flags().StringVar(&sf.outDir, "out", "", "output directory for scoring_sheet.csv and key.json (required)")
	sampleCmd.Flags().Int64Var(&sf.seed, "seed", 1, "deterministic PRNG seed")
	sampleCmd.Flags().StringVar(&sf.titlesPath, "titles", "", "optional TSV file (article_id<TAB>title[<TAB>url]) to fill/override rag-db titles")
	sampleCmd.Flags().StringVar(&sf.summariesPath, "summaries", "", "optional TSV file (article_id<TAB>summary) to evaluate summary_vector via the embedder")
	sampleCmd.Flags().StringVar(&sf.userID, "user-id", "", "owner user UUID in rag_documents (or set EVAL_USER_ID)")

	var scf scoreFlags
	scoreCmd := &cobra.Command{
		Use:   "score",
		Short: "Score a completed CSV sheet against the JSON key and pick representation + threshold",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScore(scf, os.Stdout)
		},
	}
	scoreCmd.Flags().StringVar(&scf.sheetPath, "sheet", "", "path to scored CSV sheet (required)")
	scoreCmd.Flags().StringVar(&scf.keyPath, "key", "", "path to JSON key file produced by sample (required)")
	scoreCmd.Flags().BoolVar(&scf.jsonOnly, "json", false, "emit JSON output only")
	scoreCmd.Flags().StringVar(&scf.jsonOutPath, "out", "", "optional path to write the JSON report")

	rootCmd.AddCommand(sampleCmd, scoreCmd)
	return rootCmd
}

func runSample(flags sampleFlags) error {
	if strings.TrimSpace(flags.readIDsPath) == "" {
		return fmt.Errorf("--read-ids is required")
	}
	if strings.TrimSpace(flags.outDir) == "" {
		return fmt.Errorf("--out is required")
	}

	userID := strings.TrimSpace(flags.userID)
	if userID == "" {
		userID = strings.TrimSpace(os.Getenv("EVAL_USER_ID"))
	}
	if userID == "" {
		return fmt.Errorf("--user-id or EVAL_USER_ID is required")
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil || parsedUserID == uuid.Nil {
		return fmt.Errorf("invalid --user-id %q: must be a non-nil UUID", userID)
	}

	dbCfg, err := config.LoadDBConfig()
	if err != nil {
		return fmt.Errorf("load database config: %w", err)
	}

	var encoder domain.VectorEncoder
	var summaries map[string]string
	if strings.TrimSpace(flags.summariesPath) != "" {
		embCfg, err := config.LoadEmbedderConfig()
		if err != nil {
			return fmt.Errorf("load embedder config for --summaries: %w", err)
		}
		summaries, err = loadSummariesFile(flags.summariesPath)
		if err != nil {
			return err
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
		httpClient := httpclient.NewPooledClient(time.Duration(embCfg.Timeout) * time.Second)
		encoder = rag_augur.NewOllamaEmbedder(embCfg.URL, embCfg.Model, embCfg.Timeout, logger, embCfg.InferenceToken, httpClient)
	}

	readIDs, err := loadReadIDsFile(flags.readIDsPath)
	if err != nil {
		return err
	}

	var titleOverrides map[string]domain.RagDocumentMetadata
	if strings.TrimSpace(flags.titlesPath) != "" {
		titleOverrides, err = loadTitlesFile(flags.titlesPath)
		if err != nil {
			return err
		}
	}

	ctx := context.Background()
	pool, err := infra.NewPostgresDB(ctx, dbCfg.DSN(), infra.PoolConfig{
		MaxConns: dbCfg.MaxConns,
		MinConns: dbCfg.MinConns,
	})
	if err != nil {
		return fmt.Errorf("connect to rag-db: %w", err)
	}
	defer pool.Close()

	repo := repository.NewCandidateSimilarityRepository(pool)
	simUC := usecase.NewCandidateSimilarityUsecase(repo)
	runner := evalconnections.NewSampleRunner(simUC, repo, encoder)

	out, err := runner.Run(ctx, evalconnections.SampleOptions{
		UserID:         parsedUserID.String(),
		ReadIDs:        readIDs,
		OutDir:         flags.outDir,
		Seed:           flags.seed,
		TitleOverrides: titleOverrides,
		Summaries:      summaries,
	})
	if err != nil {
		return err
	}

	fmt.Printf("Sampled %d pairs from %d indexed read articles (summary_vector: %s)\n",
		out.PairCount, out.IndexedReadArticles, out.SummaryVectorStatus)
	fmt.Printf("Scoring sheet: %s\n", out.SheetPath)
	fmt.Printf("Key file:      %s\n", out.KeyPath)
	return nil
}

func runScore(flags scoreFlags, stdout io.Writer) error {
	if strings.TrimSpace(flags.sheetPath) == "" {
		return fmt.Errorf("--sheet is required")
	}
	if strings.TrimSpace(flags.keyPath) == "" {
		return fmt.Errorf("--key is required")
	}

	sheetFile, err := os.Open(flags.sheetPath) // #nosec G304 -- operator CLI input path
	if err != nil {
		return fmt.Errorf("open sheet %s: %w", flags.sheetPath, err)
	}
	defer func() { _ = sheetFile.Close() }()
	rows, err := evalconnections.ReadScoringSheetCSV(sheetFile)
	if err != nil {
		return err
	}

	keyFile, err := os.Open(flags.keyPath) // #nosec G304 -- operator CLI input path
	if err != nil {
		return fmt.Errorf("open key %s: %w", flags.keyPath, err)
	}
	defer func() { _ = keyFile.Close() }()
	key, err := evalconnections.ReadKeyJSON(keyFile)
	if err != nil {
		return err
	}

	report, err := evalconnections.EvaluateRepresentations(rows, key)
	if err != nil {
		return err
	}

	jsonBytes, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal score report: %w", err)
	}
	jsonBytes = append(jsonBytes, '\n')

	if strings.TrimSpace(flags.jsonOutPath) != "" {
		if err := os.WriteFile(flags.jsonOutPath, jsonBytes, 0o600); err != nil {
			return fmt.Errorf("write JSON report %s: %w", flags.jsonOutPath, err)
		}
	}

	if flags.jsonOnly {
		_, err = stdout.Write(jsonBytes)
		return err
	}

	if _, err := io.WriteString(stdout, evalconnections.FormatScoreReport(report)); err != nil {
		return err
	}
	if _, err := io.WriteString(stdout, "\n--- JSON ---\n"); err != nil {
		return err
	}
	_, err = stdout.Write(jsonBytes)
	return err
}

func loadReadIDsFile(path string) ([]string, error) {
	f, err := os.Open(path) // #nosec G304 -- operator CLI input path
	if err != nil {
		return nil, fmt.Errorf("open read-ids %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return evalconnections.ParseReadIDs(f)
}

func loadTitlesFile(path string) (map[string]domain.RagDocumentMetadata, error) {
	f, err := os.Open(path) // #nosec G304 -- operator CLI input path
	if err != nil {
		return nil, fmt.Errorf("open titles %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return evalconnections.ParseTitlesTSV(f)
}

func loadSummariesFile(path string) (map[string]string, error) {
	f, err := os.Open(path) // #nosec G304 -- operator CLI input path
	if err != nil {
		return nil, fmt.Errorf("open summaries %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	return evalconnections.ParseSummariesTSV(f)
}
