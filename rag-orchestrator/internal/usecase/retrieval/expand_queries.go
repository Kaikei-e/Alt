package retrieval

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"rag-orchestrator/internal/domain"

	"golang.org/x/sync/errgroup"
)

// RewriteQueryWithHistory rewrites a user query using conversation history
// so it becomes self-contained for retrieval. E.g., "Tell me more about that"
// becomes "Tell me more about Russia oil sanctions" based on prior context.
func RewriteQueryWithHistory(ctx context.Context, query string, history []domain.Message, llmClient domain.LLMClient, logger *slog.Logger) string {
	if len(history) == 0 {
		return query
	}

	// Build a compact conversation summary (last 3 turns max)
	maxTurns := 6 // 3 user + 3 assistant
	start := 0
	if len(history) > maxTurns {
		start = len(history) - maxTurns
	}
	var historyLines strings.Builder
	for _, msg := range history[start:] {
		// Truncate long messages
		content := msg.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		historyLines.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, content))
	}

	prompt := fmt.Sprintf(`Rewrite the latest user query as a self-contained search query using the conversation context.
Output ONLY the rewritten query on a single line. No explanation.

Conversation:
%s
Latest query: %s

Rewritten query:`, historyLines.String(), query)

	resp, err := llmClient.Generate(ctx, prompt, 60)
	if err != nil {
		logger.Warn("query_rewrite_failed", slog.String("error", err.Error()))
		return query // Fallback to original
	}

	rewritten := strings.TrimSpace(resp.Text)
	if rewritten == "" {
		return query
	}

	logger.Info("query_rewritten",
		slog.String("original", query),
		slog.String("rewritten", rewritten))
	return rewritten
}

// ExpandQueries runs query expansion, tag search, and original embedding in parallel (Stage 1).
func ExpandQueries(
	ctx context.Context,
	sc *StageContext,
	queryExpander domain.QueryExpander,
	llmClient domain.LLMClient,
	searchClient domain.SearchClient,
	encoder domain.VectorEncoder,
	logger *slog.Logger,
) error {
	// Multi-turn: conversation history is passed to the expansion step directly,
	// so the fast 4B model handles both coreference resolution and query expansion
	// in a single call. This replaces the separate RewriteQueryWithHistory (12B model, 27s)
	// with an integrated approach (~3s via news-creator).

	g, gctx := errgroup.WithContext(ctx)

	// goroutine A: Query Expansion (with optional conversation history)
	g.Go(func() error {
		expanded, err := expandQuery(gctx, sc.Query, sc.ConversationHistory, queryExpander, llmClient, logger)
		if err != nil {
			logger.Warn("expansion_failed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("query", sc.Query),
				slog.String("error", err.Error()))
			return nil // non-fatal
		}
		if len(expanded) > 0 {
			preFilterCount := len(expanded)
			expanded = filterExpandedQueries(expanded)
			sc.ExpandedQueries = expanded
			logger.Info("query_expanded",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("original", sc.Query),
				slog.Int("pre_filter_count", preFilterCount),
				slog.Int("post_filter_count", len(expanded)),
				slog.Any("expanded", expanded))
			if len(expanded) == 0 {
				logger.Warn("expansion_all_filtered",
					slog.String("retrieval_id", sc.RetrievalID),
					slog.String("query", sc.Query),
					slog.Int("pre_filter_count", preFilterCount),
					slog.String("reason", "all_queries_rejected_by_filter"))
			}
		}
		return nil
	})

	// goroutine B: Tag Search
	g.Go(func() error {
		if searchClient == nil {
			return nil
		}
		tagSearchStart := time.Now()
		hits, err := searchClient.Search(gctx, sc.Query)
		tagSearchDuration := time.Since(tagSearchStart)

		if err != nil {
			logger.Warn("tag_search_failed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("error", err.Error()))
			return nil // non-fatal
		}

		limit := 3
		if len(hits) < limit {
			limit = len(hits)
		}
		tagSet := make(map[string]bool)
		for i := 0; i < limit; i++ {
			for _, tag := range hits[i].Tags {
				if tag != "" {
					tagSet[tag] = true
				}
			}
		}
		for tag := range tagSet {
			if tag != sc.Query {
				sc.TagQueries = append(sc.TagQueries, tag)
			}
		}

		logger.Info("tag_search_completed",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.Int("hits_found", len(hits)),
			slog.Int("tags_extracted", len(sc.TagQueries)),
			slog.Int64("duration_ms", tagSearchDuration.Milliseconds()))
		return nil
	})

	// goroutine C: Original Query Embedding (non-fatal: degrades to BM25-only retrieval)
	g.Go(func() error {
		embeddings, err := encoder.Encode(gctx, []string{sc.Query})
		if err != nil {
			logger.Warn("original_embedding_failed",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("error", err.Error()),
				slog.String("degraded_mode", "bm25_only"))
			return nil // non-fatal: downstream stages check sc.OriginalEmbedding == nil
		}
		if len(embeddings) == 0 {
			logger.Warn("original_embedding_empty",
				slog.String("retrieval_id", sc.RetrievalID),
				slog.String("degraded_mode", "bm25_only"))
			return nil
		}
		sc.OriginalEmbedding = embeddings[0]
		return nil
	})

	return g.Wait()
}

func expandQuery(ctx context.Context, query string, history []domain.Message, queryExpander domain.QueryExpander, llmClient domain.LLMClient, logger *slog.Logger) ([]string, error) {
	if queryExpander == nil {
		return expandQueryLegacy(ctx, query, history, llmClient)
	}

	// Single-path expansion via news-creator (semaphore-managed, HIGH PRIORITY).
	// The previous race design (news-creator vs ollama-legacy in parallel) was removed
	// because both paths now route through the same HybridPrioritySemaphore after
	// ADR-567 unified AUGUR_EXTERNAL. Running two goroutines doubled semaphore slot
	// consumption without improving latency.
	var expansions []string
	var err error
	if len(history) > 0 {
		expansions, err = queryExpander.ExpandQueryWithHistory(ctx, query, history, 1, 3)
	} else {
		expansions, err = queryExpander.ExpandQuery(ctx, query, 1, 3)
	}
	if err != nil {
		logger.Warn("query_expansion_source_failed",
			slog.String("source", "news-creator"),
			slog.String("error", err.Error()))
		return nil, fmt.Errorf("query expansion failed: %w", err)
	}
	if len(expansions) > 0 {
		logger.Info("query_expansion_completed",
			slog.String("source", "news-creator"),
			slog.Int("count", len(expansions)))
	}
	return expansions, nil
}

// maxExpandedQueries caps the number of expanded queries to limit embedding + vector search cost.
const maxExpandedQueries = 8

// minQueryRuneLen is the minimum rune length for a useful search query.
const minQueryRuneLen = 3

// maxQueryRuneLen is the maximum rune length before a query is considered prompt leakage.
const maxQueryRuneLen = 200

// filterExpandedQueries removes useless queries and caps the result count.
// Filters applied in order: romanized Japanese, instruction leaks, length, dedup.
func filterExpandedQueries(queries []string) []string {
	if len(queries) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(queries))
	filtered := make([]string, 0, len(queries))
	for _, q := range queries {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		// Length filter
		runeLen := len([]rune(q))
		if runeLen < minQueryRuneLen || runeLen > maxQueryRuneLen {
			continue
		}
		if isGarbagePattern(q) {
			continue
		}
		if isRomanizedJapanese(q) {
			continue
		}
		if isDateOnly(q) {
			continue
		}
		if isInstructionLeak(q) {
			continue
		}
		if isXMLTagLeak(q) {
			continue
		}
		if isConversationMessageLeak(q) {
			continue
		}
		// Order-preserving dedup (case-insensitive)
		key := strings.ToLower(q)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		filtered = append(filtered, q)
		if len(filtered) >= maxExpandedQueries {
			break
		}
	}
	return filtered
}

// instructionLeakExact contains known instruction echo patterns (lowercased, period-stripped).
var instructionLeakExact = []string{
	"japanese queries and english queries must be translated to each other",
	"japanese queries first, then english queries",
	"output only the generated queries, one per line",
	"do not add numbering, bullets, labels, or explanations",
	"output japanese queries first",
	"one query per line",
}

// instructionMetaWords are words that appear in prompt instructions but rarely in real search queries.
// If 3+ appear in a single line, it's likely an instruction leak.
var instructionMetaWords = map[string]struct{}{
	"queries":      {},
	"generate":     {},
	"variations":   {},
	"translate":    {},
	"numbering":    {},
	"bullets":      {},
	"labels":       {},
	"explanations": {},
	"output":       {},
	"exactly":      {},
	"requirements": {},
}

// FilterSearchQueries validates search queries from the LLM query planner.
// It removes date-only, too-short, and instruction leak queries, then falls back
// to resolvedQuery if all queries were filtered out.
func FilterSearchQueries(queries []string, resolvedQuery string) []string {
	filtered := filterExpandedQueries(queries)
	if len(filtered) == 0 && resolvedQuery != "" {
		return []string{resolvedQuery}
	}
	return filtered
}

// dateOnlyPattern matches strings that are only a date (e.g., "2026-04-07", "2026/03/15", "2026.01.01").
// These are useless as search queries and appear when LLM hallucinates dates from conversation context.
var dateOnlyPattern = regexp.MustCompile(`^\d{4}[-/\.]\d{1,2}[-/\.]\d{1,2}$`)

// isDateOnly returns true if the string is only a date with no other content.
func isDateOnly(q string) bool {
	return dateOnlyPattern.MatchString(strings.TrimSpace(q))
}

// isXMLTagLeak detects leaked XML tags from the prompt structure.
func isXMLTagLeak(q string) bool {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return false
	}
	// XML tags like </example>, <input>..., </task>, <rules>
	if strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">") {
		return true
	}
	return false
}

// isConversationMessageLeak detects leaked conversation messages (e.g., "assistant: Hello!").
// These appear when the frontend's welcome message or conversation history leaks into expanded queries.
func isConversationMessageLeak(q string) bool {
	lower := strings.ToLower(strings.TrimSpace(q))
	return strings.HasPrefix(lower, "assistant:") || strings.HasPrefix(lower, "user:")
}

// isInstructionLeak detects if a query is an echoed prompt instruction.
// Uses three signals:
//  1. Exact match against known instruction patterns
//  2. High-overlap containment of known instruction patterns
//  3. High density of meta-words (3+ in a single line)
func isInstructionLeak(q string) bool {
	normalized := strings.TrimRight(strings.ToLower(strings.TrimSpace(q)), ".")

	// Check exact or high-overlap match against known patterns
	for _, pattern := range instructionLeakExact {
		if normalized == pattern {
			return true
		}
		// Long patterns: containment is enough
		if len(pattern) > 20 && strings.Contains(normalized, pattern) {
			return true
		}
	}

	// Meta-word density check
	words := strings.Fields(normalized)
	metaCount := 0
	for _, w := range words {
		if _, ok := instructionMetaWords[w]; ok {
			metaCount++
		}
	}
	return metaCount >= 3
}

// isGarbagePattern detects repetitive character sequences like ":):):):)..." or "hahahahaha".
// These appear when the LLM enters a degenerate decoding state during query expansion.
func isGarbagePattern(q string) bool {
	runes := []rune(strings.TrimSpace(q))
	if len(runes) < 6 {
		return false
	}
	for patLen := 1; patLen <= 4; patLen++ {
		if len(runes) < patLen*3 {
			continue
		}
		pat := string(runes[:patLen])
		repetitions := 0
		for i := 0; i+patLen <= len(runes); i += patLen {
			if string(runes[i:i+patLen]) == pat {
				repetitions++
			} else {
				break
			}
		}
		if repetitions >= 3 && repetitions*patLen*3 >= len(runes)*2 {
			return true
		}
	}
	return false
}

// isRomanizedJapanese detects romanized Japanese strings like "Sei-sai naiyō Rosia Amerika".
// Uses two signals: (1) macron diacritics (ō, ū, ā) typical of romaji long vowels,
// (2) multiple hyphenated words typical of syllable-split romanization.
func isRomanizedJapanese(q string) bool {
	if q == "" {
		return false
	}
	hasMacron := false
	for _, r := range q {
		// CJK characters mean real Japanese text, not romanized
		if (r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0x4E00 && r <= 0x9FFF) { // CJK Unified Ideographs
			return false
		}
		// Macron vowels are a strong signal of romanized Japanese
		switch r {
		case 'ō', 'ū', 'ā', 'ē', 'ī', 'Ō', 'Ū', 'Ā', 'Ē', 'Ī':
			hasMacron = true
		}
	}
	if hasMacron {
		return true
	}
	// Multiple hyphenated words (e.g., "sei-sai", "Roshi-a") without CJK → romanized
	words := strings.Fields(q)
	hyphenatedWords := 0
	for _, w := range words {
		w = strings.Trim(w, "-")
		if strings.Contains(w, "-") {
			hyphenatedWords++
		}
	}
	return hyphenatedWords >= 2
}

func expandQueryLegacy(ctx context.Context, query string, history []domain.Message, llmClient domain.LLMClient) ([]string, error) {
	currentDate := time.Now().Format("2006-01-02")

	var prompt string
	if len(history) > 0 {
		// Multi-turn: include conversation context for coreference resolution
		maxTurns := 6
		start := 0
		if len(history) > maxTurns {
			start = len(history) - maxTurns
		}
		var historyLines strings.Builder
		for _, msg := range history[start:] {
			content := msg.Content
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			fmt.Fprintf(&historyLines, "%s: %s\n", msg.Role, content)
		}
		prompt = fmt.Sprintf(`You are a search query generator. Current Date: %s

The user is in a conversation. Resolve coreferences using the context, then generate queries.

Conversation:
%s
Latest query: %s

Generate 3 to 5 diverse search queries based on the resolved meaning.
Include 1-2 Japanese and 2-3 English variations.
Output ONLY queries, one per line. No numbering, bullets, or explanations.`, currentDate, historyLines.String(), query)
	} else {
		prompt = fmt.Sprintf(`You are a search query generator. Current Date: %s

Generate 3 to 5 diverse search queries for the user's input.
If the input is Japanese, include 1-2 Japanese variations and 2-3 English translations.
If the input is English, generate English variations.
Interpret relative dates based on Current Date.
Output ONLY queries, one per line. No numbering, bullets, or explanations.

User Input: %s`, currentDate, query)
	}

	resp, err := llmClient.Generate(ctx, prompt, 100)
	if err != nil {
		return nil, err
	}

	rawLines := strings.Split(resp.Text, "\n")
	var expansions []string
	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			expansions = append(expansions, trimmed)
		}
	}
	return expansions, nil
}
