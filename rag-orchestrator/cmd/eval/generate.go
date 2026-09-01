package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"

	"rag-orchestrator/eval"
)

// The golden case generator mines the live rag-db read-only and writes an
// untracked case file. This repository is public, so nothing it produces may be
// committed: every query, title and article id is read from the database at run
// time and none of it is embedded in this source. Cases whose selection needs a
// literal (the query text itself, an article title) get it from a SELECT, never
// from a constant here.
//
// The output is a draft. Relevance is resolved by full-text rank, which is weak
// for Japanese text, so each case is emitted with the evidence the generator
// used and the operator curates before trusting the numbers.

// articleScopePattern matches the scoped-query envelope the chat handler wraps
// around an article-scoped question.
var articleScopePattern = regexp.MustCompile(`(?s)^Regarding the article: (.*) \[articleId: ([0-9a-f-]{36})\]\s*Question:\s*(.*)$`)

type generateConfig struct {
	DSN            string
	OutputPath     string
	Since          time.Time
	MaxCases       int
	RelevantPerCas int
}

func (c generateConfig) validate() error {
	if strings.TrimSpace(c.DSN) == "" {
		return fmt.Errorf("generate: %s is required", ragDBDSNEnv)
	}
	if strings.TrimSpace(c.OutputPath) == "" {
		return fmt.Errorf("generate: output path is required")
	}
	if c.MaxCases <= 0 {
		return fmt.Errorf("generate: max cases must be positive, got %d", c.MaxCases)
	}
	if c.RelevantPerCas <= 0 {
		return fmt.Errorf("generate: relevant-per-case must be positive, got %d", c.RelevantPerCas)
	}
	return nil
}

// userTurn is one mined user message plus the turn the assistant produced for it.
type userTurn struct {
	ConversationID  string
	MessageID       string
	CreatedAt       time.Time
	Content         string
	History         []eval.HistoryMessage
	CitedArticleIDs []string
}

func runGenerate(ctx context.Context, cfg generateConfig, logger *slog.Logger) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	pool, err := pgxpool.New(ctx, cfg.DSN)
	if err != nil {
		return fmt.Errorf("connect rag-db: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping rag-db: %w", err)
	}

	turns, err := selectUserTurns(ctx, pool, cfg.Since, cfg.MaxCases)
	if err != nil {
		return err
	}
	if len(turns) == 0 {
		return fmt.Errorf("generate: no user turns found since %s", cfg.Since.Format(time.RFC3339))
	}

	degenerate, err := selectDegenerateArticles(ctx, pool)
	if err != nil {
		return err
	}
	duplicates, err := selectDuplicateArticles(ctx, pool)
	if err != nil {
		return err
	}
	alwaysForbidden := append(append([]string{}, degenerate...), duplicates...)
	logger.Info("index_hygiene_forbidden_resolved",
		"degenerate_titles", len(degenerate), "duplicate_extras", len(duplicates))

	cases := make([]eval.GoldenCase, 0, len(turns))
	for i, turn := range turns {
		gc, err := buildCase(ctx, pool, cfg, i, turn, alwaysForbidden)
		if err != nil {
			return err
		}
		logger.Info("case_generated",
			"case_id", gc.ID,
			"category", gc.Category,
			"relevant_articles", len(gc.Expected.RelevantArticleIDs),
			"forbidden_articles", len(gc.Expected.ForbiddenArticleIDs),
			"expect_no_answer", gc.Expected.ExpectNoAnswer)
		cases = append(cases, gc)
	}

	data, err := json.MarshalIndent(cases, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal generated cases: %w", err)
	}
	if err := os.WriteFile(cfg.OutputPath, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", cfg.OutputPath, err)
	}

	logger.Info("golden_cases_generated", "path", cfg.OutputPath, "cases", len(cases))
	return nil
}

// selectUserTurns reads recent user messages together with the citations the
// assistant produced in reply. Ordering is by conversation then time so the
// preceding turns can be attached as history.
func selectUserTurns(ctx context.Context, pool *pgxpool.Pool, since time.Time, limit int) ([]userTurn, error) {
	const query = `
		WITH ordered AS (
			SELECT
				m.id,
				m.conversation_id,
				m.role,
				m.content,
				m.created_at,
				LEAD(m.role) OVER w  AS next_role,
				LEAD(m.citations) OVER w AS next_citations
			FROM augur_messages m
			WHERE m.created_at >= $1
			WINDOW w AS (PARTITION BY m.conversation_id ORDER BY m.created_at, m.id)
		)
		SELECT id, conversation_id, content, created_at,
		       CASE WHEN next_role = 'assistant' THEN next_citations ELSE '[]'::jsonb END
		FROM ordered
		WHERE role = 'user'
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := pool.Query(ctx, query, since, limit)
	if err != nil {
		return nil, fmt.Errorf("select user turns: %w", err)
	}
	defer rows.Close()

	var turns []userTurn
	for rows.Next() {
		var (
			turn      userTurn
			citations []struct {
				RefID string `json:"ref_id"`
			}
		)
		if err := rows.Scan(&turn.MessageID, &turn.ConversationID, &turn.Content, &turn.CreatedAt, &citations); err != nil {
			return nil, fmt.Errorf("scan user turn: %w", err)
		}
		for _, c := range citations {
			if c.RefID != "" {
				turn.CitedArticleIDs = append(turn.CitedArticleIDs, c.RefID)
			}
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user turns: %w", err)
	}

	for i := range turns {
		history, err := selectHistory(ctx, pool, turns[i].ConversationID, turns[i].CreatedAt)
		if err != nil {
			return nil, err
		}
		turns[i].History = history
	}
	return turns, nil
}

// selectHistory returns the two turns immediately preceding the given message.
func selectHistory(ctx context.Context, pool *pgxpool.Pool, conversationID string, before time.Time) ([]eval.HistoryMessage, error) {
	const query = `
		SELECT role, content
		FROM augur_messages
		WHERE conversation_id = $1 AND created_at < $2
		ORDER BY created_at DESC
		LIMIT 2`

	rows, err := pool.Query(ctx, query, conversationID, before)
	if err != nil {
		return nil, fmt.Errorf("select history: %w", err)
	}
	defer rows.Close()

	var reversed []eval.HistoryMessage
	for rows.Next() {
		var msg eval.HistoryMessage
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, fmt.Errorf("scan history: %w", err)
		}
		reversed = append(reversed, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate history: %w", err)
	}

	history := make([]eval.HistoryMessage, 0, len(reversed))
	for i := len(reversed) - 1; i >= 0; i-- {
		history = append(history, reversed[i])
	}
	return history, nil
}

// resolvedArticle is one candidate answer article with the metadata a case needs.
type resolvedArticle struct {
	ArticleID string
	Title     string
}

// resolveRelevantArticles ranks the corpus against the query text using two
// independent signals, because neither covers the ja/en mix on its own: the
// full-text index handles the space-delimited half, and character n-grams over
// titles handle the Japanese half, which `simple` tokenization collapses into
// one unusable token. The query text comes from the mined message, so no
// production wording is embedded in this program.
func resolveRelevantArticles(ctx context.Context, pool *pgxpool.Pool, queryText string, limit int) ([]resolvedArticle, error) {
	lexical, err := resolveByFullText(ctx, pool, queryText, limit)
	if err != nil {
		return nil, err
	}
	ngram, err := resolveByTitleNGrams(ctx, pool, queryText, limit)
	if err != nil {
		return nil, err
	}

	merged := make([]resolvedArticle, 0, limit)
	seen := make(map[string]bool, limit)
	for _, source := range [][]resolvedArticle{lexical, ngram} {
		for _, article := range source {
			if seen[article.ArticleID] || len(merged) >= limit {
				continue
			}
			seen[article.ArticleID] = true
			merged = append(merged, article)
		}
	}
	return merged, nil
}

func resolveByFullText(ctx context.Context, pool *pgxpool.Pool, queryText string, limit int) ([]resolvedArticle, error) {
	const query = `
		WITH q AS (SELECT websearch_to_tsquery('simple', $1) AS tsq)
		SELECT d.article_id, v.title, max(ts_rank(c.tsv, q.tsq)) AS rank
		FROM rag_chunks c
		JOIN q ON c.tsv @@ q.tsq
		JOIN rag_document_versions v ON v.id = c.version_id
		JOIN rag_documents d ON d.current_version_id = v.id
		GROUP BY d.article_id, v.title
		ORDER BY rank DESC
		LIMIT $2`
	return scanResolved(ctx, pool, "full-text", query, queryText, limit)
}

// resolveByTitleNGrams scores titles by the inverse document frequency of the
// query n-grams they contain, so shared grammatical filler ranks far below a
// proper noun that appears in only a handful of titles.
func resolveByTitleNGrams(ctx context.Context, pool *pgxpool.Pool, queryText string, limit int) ([]resolvedArticle, error) {
	grams := queryNGrams(queryText)
	if len(grams) == 0 {
		return nil, nil
	}

	const query = `
		WITH grams AS (SELECT unnest($1::text[]) AS g),
		hits AS (
			SELECT grams.g, d.article_id, v.title
			FROM grams
			JOIN rag_document_versions v ON v.title ILIKE '%' || grams.g || '%'
			JOIN rag_documents d ON d.current_version_id = v.id
		),
		df AS (SELECT g, count(*) AS n FROM hits GROUP BY g)
		SELECT h.article_id, h.title, sum(1.0 / df.n)::real AS score
		FROM hits h
		JOIN df ON df.g = h.g
		GROUP BY h.article_id, h.title
		ORDER BY score DESC
		LIMIT $2`
	return scanResolved(ctx, pool, "title n-gram", query, grams, limit)
}

func scanResolved(ctx context.Context, pool *pgxpool.Pool, what, query string, arg any, limit int) ([]resolvedArticle, error) {
	rows, err := pool.Query(ctx, query, arg, limit)
	if err != nil {
		return nil, fmt.Errorf("resolve relevant articles by %s: %w", what, err)
	}
	defer rows.Close()

	var out []resolvedArticle
	for rows.Next() {
		var (
			article resolvedArticle
			score   float32
		)
		if err := rows.Scan(&article.ArticleID, &article.Title, &score); err != nil {
			return nil, fmt.Errorf("scan %s candidate: %w", what, err)
		}
		out = append(out, article)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s candidates: %w", what, err)
	}
	return out, nil
}

// queryNGrams slices the query into probe strings: character trigrams for CJK
// runs, whole words for latin runs. Punctuation is dropped.
func queryNGrams(text string) []string {
	const gramSize = 3

	var (
		grams []string
		cjk   []rune
		latin []rune
	)
	flushCJK := func() {
		for i := 0; i+gramSize <= len(cjk); i++ {
			grams = append(grams, string(cjk[i:i+gramSize]))
		}
		if len(cjk) > 0 && len(cjk) < gramSize {
			grams = append(grams, string(cjk))
		}
		cjk = cjk[:0]
	}
	flushLatin := func() {
		if len(latin) >= gramSize {
			grams = append(grams, string(latin))
		}
		latin = latin[:0]
	}

	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana):
			flushLatin()
			cjk = append(cjk, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			latin = append(latin, r)
		default:
			flushCJK()
			flushLatin()
		}
	}
	flushCJK()
	flushLatin()

	return dedupeExcluding(grams, nil)
}

// selectArticleTitle looks up the current title of one article.
func selectArticleTitle(ctx context.Context, pool *pgxpool.Pool, articleID string) (string, error) {
	const query = `
		SELECT coalesce(v.title, '')
		FROM rag_documents d
		JOIN rag_document_versions v ON v.id = d.current_version_id
		WHERE d.article_id = $1`

	var title string
	if err := pool.QueryRow(ctx, query, articleID).Scan(&title); err != nil {
		return "", fmt.Errorf("select article title %s: %w", articleID, err)
	}
	return title, nil
}

// selectDegenerateArticles finds documents whose title carries no information —
// leftover test rows that lexical search keeps surfacing for testing queries.
func selectDegenerateArticles(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	const query = `
		SELECT d.article_id
		FROM rag_documents d
		JOIN rag_document_versions v ON v.id = d.current_version_id
		WHERE char_length(btrim(coalesce(v.title, ''))) <= 6
		LIMIT 50`
	return selectArticleIDs(ctx, pool, query, "degenerate titles")
}

// selectDuplicateArticles finds the redundant copies of documents that were
// ingested several times under the same title, keeping the newest of each group.
func selectDuplicateArticles(ctx context.Context, pool *pgxpool.Pool) ([]string, error) {
	const query = `
		WITH ranked AS (
			SELECT d.article_id,
			       row_number() OVER (PARTITION BY v.title ORDER BY v.created_at DESC) AS copy_rank,
			       count(*) OVER (PARTITION BY v.title) AS copies
			FROM rag_documents d
			JOIN rag_document_versions v ON v.id = d.current_version_id
			WHERE char_length(btrim(coalesce(v.title, ''))) > 6
		)
		SELECT article_id FROM ranked WHERE copies > 3 AND copy_rank > 1 LIMIT 50`
	return selectArticleIDs(ctx, pool, query, "duplicate copies")
}

func selectArticleIDs(ctx context.Context, pool *pgxpool.Pool, query, what string) ([]string, error) {
	rows, err := pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("select %s: %w", what, err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan %s: %w", what, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", what, err)
	}
	return ids, nil
}

func buildCase(ctx context.Context, pool *pgxpool.Pool, cfg generateConfig, index int, turn userTurn, alwaysForbidden []string) (eval.GoldenCase, error) {
	gc := eval.GoldenCase{
		ID:                  fmt.Sprintf("mined-%03d-%s", index+1, turn.CreatedAt.UTC().Format("20060102")),
		ConversationHistory: turn.History,
		Tags:                []string{"mined", "needs-curation"},
		Note:                "generated from the live corpus; curate category and relevance before trusting the numbers",
	}

	queryText := turn.Content
	if m := articleScopePattern.FindStringSubmatch(turn.Content); m != nil {
		articleID := m[2]
		title, err := selectArticleTitle(ctx, pool, articleID)
		if err != nil {
			return eval.GoldenCase{}, err
		}
		gc.ArticleScope = &eval.ArticleScopeInfo{ArticleID: articleID, Title: title}
		queryText = strings.TrimSpace(m[3])
	}
	gc.Query = queryText

	// An article-scoped follow-up asks about the scoped article, not the corpus,
	// so its relevance set is the scope itself.
	var resolved []resolvedArticle
	if gc.ArticleScope != nil {
		resolved = []resolvedArticle{{ArticleID: gc.ArticleScope.ArticleID, Title: gc.ArticleScope.Title}}
	} else {
		var err error
		resolved, err = resolveRelevantArticles(ctx, pool, queryText, cfg.RelevantPerCas)
		if err != nil {
			return eval.GoldenCase{}, err
		}
	}

	relevantIDs := make([]string, 0, len(resolved))
	corpusLanguages := make(map[string]bool, 2)
	for _, article := range resolved {
		relevantIDs = append(relevantIDs, article.ArticleID)
		corpusLanguages[detectLanguage(article.Title)] = true
	}

	gc.Language = eval.LanguagePair{
		Query:  detectLanguage(queryText),
		Corpus: summarizeLanguages(corpusLanguages),
	}
	gc.Category = deriveCategory(gc, len(relevantIDs))

	forbidden := dedupeExcluding(append(append([]string{}, alwaysForbidden...), driftCandidates(turn.CitedArticleIDs, relevantIDs)...), relevantIDs)

	gc.Expected = eval.ExpectedBehavior{
		RetrievalScope:      retrievalScope(gc),
		RelevantArticleIDs:  relevantIDs,
		ForbiddenArticleIDs: forbidden,
		RequiresCitations:   len(relevantIDs) > 0,
		ExpectNoAnswer:      len(relevantIDs) == 0,
	}
	if len(relevantIDs) > 0 {
		gc.Expected.MinExpectedRecall = 1.0 / float64(len(relevantIDs))
	}
	if gc.ArticleScope != nil {
		gc.Expected.ExpectedCitationArticleIDs = []string{gc.ArticleScope.ArticleID}
	}
	if gc.Expected.ExpectNoAnswer {
		gc.Expected.ForbiddenArticleIDs = nil
	}

	return gc, nil
}

// driftCandidates are the articles the assistant actually cited that the
// full-text resolver did not consider relevant — the concrete drift a rerun
// must stop reproducing.
func driftCandidates(cited, relevant []string) []string {
	relevantSet := make(map[string]bool, len(relevant))
	for _, id := range relevant {
		relevantSet[id] = true
	}
	var drift []string
	for _, id := range cited {
		if !relevantSet[id] {
			drift = append(drift, id)
		}
	}
	return drift
}

func dedupeExcluding(ids []string, exclude []string) []string {
	excluded := make(map[string]bool, len(exclude))
	for _, id := range exclude {
		excluded[id] = true
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || excluded[id] || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func deriveCategory(gc eval.GoldenCase, relevantCount int) string {
	switch {
	case relevantCount == 0:
		return eval.CategoryNoAnswer
	case gc.ArticleScope != nil:
		return eval.CategoryArticleScoped
	case len(gc.ConversationHistory) > 0:
		return eval.CategoryFollowUp
	case gc.IsCrossLingual():
		return eval.CategoryCrossLingual
	default:
		return eval.CategoryRecallMiss
	}
}

func retrievalScope(gc eval.GoldenCase) string {
	if gc.ArticleScope != nil {
		return "article_only"
	}
	return "global"
}

// detectLanguage classifies text by whether it carries CJK characters. The
// corpus is a ja/en mix, so this two-way split is all the language pair needs.
func detectLanguage(text string) string {
	for _, r := range text {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) {
			return "ja"
		}
	}
	return "en"
}

func summarizeLanguages(languages map[string]bool) string {
	switch {
	case len(languages) == 0:
		return ""
	case len(languages) > 1:
		return eval.LanguageMixed
	case languages["ja"]:
		return "ja"
	default:
		return "en"
	}
}
