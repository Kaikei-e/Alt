package retrieval

import (
	"log/slog"
	"sort"
	"time"

	"rag-orchestrator/internal/domain"
)

// AllocateConfig holds allocation stage parameters.
type AllocateConfig struct {
	DynamicLanguageAllocationEnabled bool
}

// Allocate selects final context chunks using language allocation and quota (Stage 5).
func Allocate(
	sc *StageContext,
	cfg AllocateConfig,
	logger *slog.Logger,
) []ContextItem {
	quotaOriginal := sc.QuotaOriginal
	quotaExpanded := sc.QuotaExpanded

	var contexts []ContextItem

	if cfg.DynamicLanguageAllocationEnabled {
		contexts = SelectContextsDynamic(sc.HitsOriginal, sc.HitsExpanded, quotaOriginal+quotaExpanded, sc.RerankApplied)

		jaCount, enCount := 0, 0
		for _, ctx := range contexts {
			if IsJapanese(ctx.Title) {
				jaCount++
			} else {
				enCount++
			}
		}
		logger.Info("dynamic_language_allocation_completed",
			slog.String("retrieval_id", sc.RetrievalID),
			slog.Int("japanese_count", jaCount),
			slog.Int("english_count", enCount),
			slog.Int("total_contexts", len(contexts)))
	} else {
		contexts = allocateLegacy(sc.HitsOriginal, sc.HitsExpanded, quotaOriginal, quotaExpanded)
	}

	return contexts
}

// SelectContextsDynamic merges and selects top N contexts from all sources by score.
func SelectContextsDynamic(hitsOriginal []domain.SearchResult, hitsExpanded []ContextItem, totalQuota int, rerankApplied bool) []ContextItem {
	if !rerankApplied {
		// Rank-interleave original and expanded hits when not reranked so incompatible RRF scales are not compared.
		return interleaveHits(hitsOriginal, hitsExpanded, totalQuota, false)
	}

	allCandidates := interleaveHits(hitsOriginal, hitsExpanded, -1, true)

	// Stable: a BM25-only degraded result set carries no score at all (the
	// search-indexer response exposes none), and an unstable sort over equal
	// scores discards the index's ranking.
	sort.SliceStable(allCandidates, func(i, j int) bool {
		return allCandidates[i].Score > allCandidates[j].Score
	})

	if len(allCandidates) > totalQuota {
		allCandidates = allCandidates[:totalQuota]
	}

	return allCandidates
}

// interleaveHits merges original and expanded hits in rank-interleaved order (original first at each rank), deduplicating by hitIdentity.
func interleaveHits(hitsOriginal []domain.SearchResult, hitsExpanded []ContextItem, limit int, rerankApplied bool) []ContextItem {
	if limit == 0 {
		return []ContextItem{}
	}
	total := len(hitsOriginal) + len(hitsExpanded)
	capHint := total
	if limit > 0 && limit < capHint {
		capHint = limit
	}
	seen := make(map[string]struct{}, total)
	merged := make([]ContextItem, 0, capHint)

	add := func(key string, item ContextItem) {
		if key == "" || (limit > 0 && len(merged) >= limit) {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		merged = append(merged, item)
	}

	for r := 0; (limit < 0 || len(merged) < limit) && (r < len(hitsOriginal) || r < len(hitsExpanded)); r++ {
		if r < len(hitsOriginal) {
			res := hitsOriginal[r]
			item := ContextItem{
				ChunkText:       res.Chunk.Content,
				URL:             res.URL,
				Title:           res.Title,
				PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
				Score:           res.Score,
				ScoreKind:       res.ScoreKind,
				DocumentVersion: res.DocumentVersion,
				ChunkID:         res.Chunk.ID,
				ArticleID:       res.ArticleID,
			}
			if rerankApplied {
				item.RerankScore = res.Score
				item.RerankApplied = true
			}
			add(hitIdentity(res.Chunk.ID, res.ArticleID), item)
		}
		if r < len(hitsExpanded) {
			item := hitsExpanded[r]
			add(hitIdentity(item.ChunkID, item.ArticleID), item)
		}
	}

	return merged
}

func allocateLegacy(hitsOriginal []domain.SearchResult, hitsExpanded []ContextItem, quotaOriginal, quotaExpanded int) []ContextItem {
	contexts := make([]ContextItem, 0, quotaOriginal+quotaExpanded)
	seen := make(map[string]bool)

	countOriginal := 0
	for _, res := range hitsOriginal {
		if countOriginal >= quotaOriginal {
			break
		}
		key := hitIdentity(res.Chunk.ID, res.ArticleID)
		if key == "" || !seen[key] {
			contexts = append(contexts, ContextItem{
				ChunkText:       res.Chunk.Content,
				URL:             res.URL,
				Title:           res.Title,
				PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
				Score:           res.Score,
				ScoreKind:       res.ScoreKind,
				DocumentVersion: res.DocumentVersion,
				ChunkID:         res.Chunk.ID,
				ArticleID:       res.ArticleID,
			})
			seen[key] = true
			countOriginal++
		}
	}

	countExpanded := 0

	// Pass 1: Prioritize English/Non-Japanese documents
	for _, item := range hitsExpanded {
		if countExpanded >= quotaExpanded {
			break
		}
		key := hitIdentity(item.ChunkID, item.ArticleID)
		if key != "" && seen[key] {
			continue
		}
		if !IsJapanese(item.Title) {
			contexts = append(contexts, item)
			seen[key] = true
			countExpanded++
		}
	}

	// Pass 2: Fill remaining quota
	for _, item := range hitsExpanded {
		if countExpanded >= quotaExpanded {
			break
		}
		key := hitIdentity(item.ChunkID, item.ArticleID)
		if key != "" && seen[key] {
			continue
		}
		contexts = append(contexts, item)
		seen[key] = true
		countExpanded++
	}

	return contexts
}

// IsJapanese checks if a string contains Japanese characters.
func IsJapanese(s string) bool {
	for _, r := range s {
		if (r >= '\u3040' && r <= '\u309f') || // Hiragana
			(r >= '\u30a0' && r <= '\u30ff') || // Katakana
			(r >= '\u4e00' && r <= '\u9faf') { // Kanji
			return true
		}
	}
	return false
}
