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
	seen := make(map[string]bool)
	allCandidates := make([]ContextItem, 0, len(hitsOriginal)+len(hitsExpanded))

	for _, res := range hitsOriginal {
		key := hitIdentity(res.Chunk.ID, res.ArticleID)
		if key != "" && seen[key] {
			continue
		}
		item := ContextItem{
			ChunkText:       res.Chunk.Content,
			URL:             res.URL,
			Title:           res.Title,
			PublishedAt:     res.Chunk.CreatedAt.Format(time.RFC3339),
			Score:           res.Score,
			DocumentVersion: res.DocumentVersion,
			ChunkID:         res.Chunk.ID,
			ArticleID:       res.ArticleID,
		}
		if rerankApplied {
			item.RerankScore = res.Score
			item.RerankApplied = true
		}
		allCandidates = append(allCandidates, item)
		seen[key] = true
	}

	for _, item := range hitsExpanded {
		key := hitIdentity(item.ChunkID, item.ArticleID)
		if key != "" && seen[key] {
			continue
		}
		allCandidates = append(allCandidates, item)
		seen[key] = true
	}

	sort.Slice(allCandidates, func(i, j int) bool {
		return allCandidates[i].Score > allCandidates[j].Score
	})

	if len(allCandidates) > totalQuota {
		allCandidates = allCandidates[:totalQuota]
	}

	return allCandidates
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
