package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strings"
	"unicode"

	"rag-orchestrator/internal/domain"
)

type articleScopedStrategy struct {
	docRepo       domain.RagDocumentRepository
	chunkRepo     domain.RagChunkRepository
	queryExpander domain.QueryExpander
	logger        *slog.Logger
}

// NewArticleScopedStrategy creates a strategy that retrieves all chunks for a specific article.
// queryExpander is optional; when provided, non-English follow-up queries are translated
// to English before BM25 reranking so cross-language matching works correctly.
func NewArticleScopedStrategy(
	docRepo domain.RagDocumentRepository,
	chunkRepo domain.RagChunkRepository,
	logger *slog.Logger,
	queryExpander ...domain.QueryExpander,
) RetrievalStrategy {
	var qe domain.QueryExpander
	if len(queryExpander) > 0 {
		qe = queryExpander[0]
	}
	return &articleScopedStrategy{
		docRepo:       docRepo,
		chunkRepo:     chunkRepo,
		queryExpander: qe,
		logger:        logger,
	}
}

func (s *articleScopedStrategy) Name() string { return "article_scoped" }

func (s *articleScopedStrategy) Retrieve(ctx context.Context, input RetrieveContextInput, intent QueryIntent) (*RetrieveContextOutput, error) {
	doc, err := s.docRepo.GetByArticleID(ctx, intent.ArticleID)
	if err != nil {
		return nil, fmt.Errorf("get document by article ID %s: %w", intent.ArticleID, err)
	}
	if doc == nil || doc.CurrentVersionID == nil {
		return nil, ErrArticleNotIndexed
	}

	version, err := s.docRepo.GetVersionByID(ctx, *doc.CurrentVersionID)
	if err != nil {
		return nil, fmt.Errorf("get version %s: %w", doc.CurrentVersionID.String(), err)
	}
	if version == nil {
		return nil, ErrArticleNotIndexed
	}

	chunks, err := s.chunkRepo.GetChunksByVersionID(ctx, *doc.CurrentVersionID)
	if err != nil {
		return nil, fmt.Errorf("get chunks for version %s: %w", doc.CurrentVersionID.String(), err)
	}
	if len(chunks) == 0 {
		return nil, ErrArticleNotIndexed
	}

	s.logger.Info("article_scoped_retrieval",
		slog.String("article_id", intent.ArticleID),
		slog.Int("chunks", len(chunks)),
		slog.String("version_id", doc.CurrentVersionID.String()))

	contexts := make([]ContextItem, len(chunks))
	for i, chunk := range chunks {
		contexts[i] = ContextItem{
			ChunkID:         chunk.ID,
			ChunkText:       chunk.Content,
			URL:             version.URL,
			Title:           version.Title,
			PublishedAt:     version.CreatedAt.Format("2006-01-02T15:04:05Z"),
			Score:           1.0,
			DocumentVersion: version.VersionNumber,
		}
	}

	// Follow-up turns: rerank chunks by BM25 relevance to the current query.
	// First turn (no history) keeps original ordinal order with uniform score.
	if len(input.ConversationHistory) > 0 && intent.UserQuestion != "" {
		rerankQuery := s.translateQueryForBM25(ctx, intent.UserQuestion, input.ConversationHistory)
		bm25RerankContexts(contexts, rerankQuery)
		s.logger.Info("article_scoped_reranked",
			slog.String("article_id", intent.ArticleID),
			slog.String("query", intent.UserQuestion),
			slog.String("rerank_query", rerankQuery),
			slog.Int("chunks", len(contexts)))
	}

	return &RetrieveContextOutput{Contexts: contexts}, nil
}

// translateQueryForBM25 converts a non-English query to English for BM25 matching
// against English article chunks. Uses QueryExpander with conversation history
// to produce an English standalone query.
func (s *articleScopedStrategy) translateQueryForBM25(ctx context.Context, query string, history []domain.Message) string {
	// If query is already primarily English, use as-is
	if isPrimarilyEnglish(query) {
		return query
	}

	// No expander available — use raw query
	if s.queryExpander == nil {
		return query
	}

	// Use ExpandQueryWithHistory: resolves coreferences + translates to English
	// japaneseCount=0, englishCount=1 → single English translation
	expanded, err := s.queryExpander.ExpandQueryWithHistory(ctx, query, history, 0, 1)
	if err != nil {
		s.logger.Warn("article_scoped_query_translation_failed",
			slog.String("query", query),
			slog.String("error", err.Error()))
		return query // fallback to original
	}

	if len(expanded) == 0 {
		return query
	}

	// Combine original + translated for broader BM25 coverage
	return query + " " + strings.Join(expanded, " ")
}

// isPrimarilyEnglish returns true if the text is predominantly ASCII/Latin.
func isPrimarilyEnglish(text string) bool {
	var asciiLetters, cjkChars int
	for _, r := range text {
		if unicode.IsLetter(r) {
			if r < 0x3000 {
				asciiLetters++
			} else {
				cjkChars++
			}
		}
	}
	total := asciiLetters + cjkChars
	if total == 0 {
		return true
	}
	return float64(asciiLetters)/float64(total) > 0.7
}

// bm25RerankContexts scores and sorts contexts by BM25 relevance to the query.
// This is a lightweight in-memory BM25 implementation that avoids external
// service calls (no embedder latency). Suitable for reranking within a single
// article's chunks (typically < 100 chunks).
func bm25RerankContexts(contexts []ContextItem, query string) {
	terms := tokenize(query)
	if len(terms) == 0 {
		return
	}

	n := len(contexts)
	// Calculate document frequency for each term
	df := make(map[string]int, len(terms))
	for _, term := range terms {
		for _, ctx := range contexts {
			if strings.Contains(strings.ToLower(ctx.ChunkText), term) {
				df[term]++
			}
		}
	}

	// BM25 parameters
	const k1 = 1.2
	const b = 0.75

	// Average document length
	var totalLen float64
	for _, ctx := range contexts {
		totalLen += float64(len(ctx.ChunkText))
	}
	avgDL := totalLen / float64(n)

	// Score each context
	for i := range contexts {
		docLen := float64(len(contexts[i].ChunkText))
		var score float64
		lowerText := strings.ToLower(contexts[i].ChunkText)

		for _, term := range terms {
			docFreq := df[term]
			if docFreq == 0 {
				continue
			}
			// Term frequency in this document
			tf := float64(strings.Count(lowerText, term))
			// IDF: log((N - df + 0.5) / (df + 0.5) + 1)
			idf := math.Log((float64(n)-float64(docFreq)+0.5)/(float64(docFreq)+0.5) + 1.0)
			// BM25 TF normalization
			tfNorm := (tf * (k1 + 1)) / (tf + k1*(1-b+b*docLen/avgDL))
			score += idf * tfNorm
		}
		contexts[i].Score = float32(score)
	}

	// If no term matched at all, restore original scores
	var maxScore float32
	for _, ctx := range contexts {
		if ctx.Score > maxScore {
			maxScore = ctx.Score
		}
	}
	if maxScore == 0 {
		for i := range contexts {
			contexts[i].Score = 1.0
		}
		return
	}

	sort.Slice(contexts, func(i, j int) bool {
		return contexts[i].Score > contexts[j].Score
	})

	// Normalize scores to 0..1 range so quality gate thresholds work correctly.
	for i := range contexts {
		contexts[i].Score /= maxScore
	}
}

// tokenize splits text into lowercase terms for BM25 scoring.
// Handles both ASCII word boundaries and CJK character-level tokens.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	seen := make(map[string]bool)
	var terms []string

	// Extract ASCII words
	var word strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) && r < 0x3000 { // ASCII/Latin letters
			word.WriteRune(r)
		} else {
			if word.Len() > 1 {
				w := word.String()
				if !seen[w] {
					terms = append(terms, w)
					seen[w] = true
				}
			}
			word.Reset()
			// CJK characters: each character is a token
			if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
				s := string(r)
				if !seen[s] {
					terms = append(terms, s)
					seen[s] = true
				}
			}
		}
	}
	if word.Len() > 1 {
		w := word.String()
		if !seen[w] {
			terms = append(terms, w)
		}
	}
	return terms
}

// selectStrategy returns the strategy for the given intent type.
func (u *answerWithRAGUsecase) selectStrategy(intentType IntentType) RetrievalStrategy {
	if s, ok := u.strategies[intentType]; ok {
		return s
	}
	return u.generalStrategy
}
