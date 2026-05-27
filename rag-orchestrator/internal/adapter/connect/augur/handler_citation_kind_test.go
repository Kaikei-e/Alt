package augur

import (
	"testing"

	augurv2 "alt/gen/proto/alt/augur/v2"
	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
)

// Augur RAG citations carry an ArticleID through the retrieval pipeline so the
// proto wire form can populate kind=ARTICLE + ref_id, which the FE rail then
// turns into a /articles/<refId> link. Before this change the wire arrived as
// kind=UNSPECIFIED and the FE rendered every citation as a disabled span —
// the exact "Ask Augur で元記事が参照されずに回答される" symptom users reported.
func TestHandler_ConvertCitationsToProtoCitations_PopulatesArticleKindAndRefID(t *testing.T) {
	h := &Handler{}
	articleID := "0baad820-7d4c-4f9c-9c5e-8c3a8b7a2a91"
	in := []usecase.Citation{
		{
			ChunkID:     "chunk-1",
			URL:         "https://example.test/posts/grounded",
			Title:       "Grounded Source",
			ArticleID:   articleID,
			PublishedAt: "2026-05-26T10:00:00Z",
		},
	}

	out := h.convertCitationsToProtoCitations(in)
	assert.Len(t, out, 1)
	assert.Equal(t, augurv2.CitationKind_CITATION_KIND_ARTICLE, out[0].Kind)
	assert.Equal(t, articleID, out[0].RefId)
	assert.Equal(t, "Grounded Source", out[0].Title)
	assert.Equal(t, "https://example.test/posts/grounded", out[0].Url)
	assert.Equal(t, "2026-05-26T10:00:00Z", out[0].PublishedAt)
}

// When ArticleID is empty the citation is still allowed to surface as a WEB
// citation if URL looks like http(s) — this is the path for tool-fetched
// external links.
func TestHandler_ConvertCitationsToProtoCitations_FallsBackToWebKind(t *testing.T) {
	h := &Handler{}
	in := []usecase.Citation{
		{URL: "https://example.test/article", Title: "External", ArticleID: ""},
	}
	out := h.convertCitationsToProtoCitations(in)
	assert.Len(t, out, 1)
	assert.Equal(t, augurv2.CitationKind_CITATION_KIND_WEB, out[0].Kind)
	assert.Equal(t, "", out[0].RefId)
}

// Garbage in (URL is a UUID, ArticleID empty) must NOT silently become a WEB
// citation — that was the ADR-926 relative-URL bug. UNSPECIFIED is the safe
// signal that tells the FE rail to render a non-clickable span.
func TestHandler_ConvertCitationsToProtoCitations_NeitherArticleNorWeb_StaysUnspecified(t *testing.T) {
	h := &Handler{}
	in := []usecase.Citation{
		{URL: "11111111-1111-4111-8111-111111111111", Title: "Junk", ArticleID: ""},
	}
	out := h.convertCitationsToProtoCitations(in)
	assert.Len(t, out, 1)
	assert.Equal(t, augurv2.CitationKind_CITATION_KIND_UNSPECIFIED, out[0].Kind)
	assert.Equal(t, "", out[0].RefId)
}

// Defence-in-depth: even if the upstream emitter regresses and ships a Title
// that is literally a UUID (the historical label-fallback bug from ADR-926),
// the handler strips it to empty so the FE's domain / "Untitled source"
// fallback takes over and no internal id surfaces in visible text.
func TestHandler_ConvertCitationsToProtoCitations_SanitizesUUIDTitle(t *testing.T) {
	h := &Handler{}
	articleID := "0baad820-7d4c-4f9c-9c5e-8c3a8b7a2a91"
	in := []usecase.Citation{
		{
			URL:       "https://example.test/post",
			Title:     articleID, // regression: title is the bare UUID
			ArticleID: articleID,
		},
	}
	out := h.convertCitationsToProtoCitations(in)
	assert.Len(t, out, 1)
	assert.Equal(t, "", out[0].Title, "UUID-only Title must be stripped so the FE can fall back to the URL domain")
}
