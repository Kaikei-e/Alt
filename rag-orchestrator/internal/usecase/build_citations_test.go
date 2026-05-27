package usecase

import (
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// buildCitations must carry the ArticleID from the retrieval ContextItem into
// the usecase Citation so the handler can later infer kind=ARTICLE and emit a
// proper ref_id. Without this propagation the Augur RAG path was silently
// shipping kind=UNSPECIFIED, which the FE renders as a disabled span — the
// "Ask Augur で元記事が参照されずに回答される" regression this change fixes.
func TestBuildCitations_PropagatesArticleID(t *testing.T) {
	u := &answerWithRAGUsecase{logger: slog.Default()}

	chunkID := uuid.New()
	articleID := uuid.New().String()
	contexts := []ContextItem{
		{
			ChunkID:     chunkID,
			ChunkText:   "Some excerpt.",
			URL:         "https://example.test/post",
			Title:       "Title A",
			ArticleID:   articleID,
			PublishedAt: "2026-05-26T10:00:00Z",
		},
	}

	raw := []LLMCitation{{ChunkID: chunkID.String()}}
	out := u.buildCitations(contexts, raw)
	assert.Len(t, out, 1)
	assert.Equal(t, articleID, out[0].ArticleID)
	assert.Equal(t, "2026-05-26T10:00:00Z", out[0].PublishedAt)
}

// 1-based index lookups must also carry the ArticleID through. The LLM may
// emit "1", "2", "3" instead of full UUIDs to save tokens, and the citation
// path falls through to a slice index lookup — that path was the original
// drop point so the test pins it down explicitly.
func TestBuildCitations_PropagatesArticleID_ViaIndexLookup(t *testing.T) {
	u := &answerWithRAGUsecase{logger: slog.Default()}

	articleID := uuid.New().String()
	contexts := []ContextItem{
		{
			ChunkID:   uuid.New(),
			ChunkText: "x",
			Title:     "Indexed",
			ArticleID: articleID,
		},
	}
	raw := []LLMCitation{{ChunkID: "1"}}
	out := u.buildCitations(contexts, raw)
	assert.Len(t, out, 1)
	assert.Equal(t, articleID, out[0].ArticleID)
}
