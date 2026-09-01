package repository

import (
	"testing"

	"rag-orchestrator/internal/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A normal turn must keep writing a bare JSON array. Every row written before
// the fallback marker existed is one, and so is every query that reads the
// column with jsonb array operators.
func TestMarshalCitationsColumn_NormalTurnStaysAnArray(t *testing.T) {
	payload, err := marshalCitationsColumn([]domain.AugurCitation{{Title: "T", URL: "https://x"}}, nil)
	require.NoError(t, err)
	assert.Equal(t, byte('['), payload[0])
	assert.JSONEq(t, `[{"url":"https://x","title":"T"}]`, string(payload))
}

func TestMarshalCitationsColumn_NilCitationsStaysEmptyArray(t *testing.T) {
	payload, err := marshalCitationsColumn(nil, nil)
	require.NoError(t, err)
	assert.JSONEq(t, `[]`, string(payload))
}

// A degraded turn switches the column to the envelope so it can be selected
// by jsonb_typeof without touching the schema.
func TestMarshalCitationsColumn_FallbackTurnCarriesMarker(t *testing.T) {
	payload, err := marshalCitationsColumn(
		[]domain.AugurCitation{{Title: "T"}},
		&domain.AugurFallback{Code: "short_under_grounded", Reason: "low keyword coverage"},
	)
	require.NoError(t, err)
	assert.Equal(t, byte('{'), payload[0])
	assert.JSONEq(t,
		`{"items":[{"url":"","title":"T"}],"fallback":{"code":"short_under_grounded","reason":"low keyword coverage"}}`,
		string(payload))
}

func TestUnmarshalCitationsColumn_ReadsBothShapes(t *testing.T) {
	t.Run("legacy array", func(t *testing.T) {
		citations, fallback, err := unmarshalCitationsColumn([]byte(`[{"url":"https://x","title":"T"}]`))
		require.NoError(t, err)
		assert.Nil(t, fallback)
		require.Len(t, citations, 1)
		assert.Equal(t, "T", citations[0].Title)
	})

	t.Run("envelope", func(t *testing.T) {
		citations, fallback, err := unmarshalCitationsColumn(
			[]byte(`  {"items":[{"url":"https://x","title":"T"}],"fallback":{"code":"llm_fallback","reason":"declined"}}`))
		require.NoError(t, err)
		require.NotNil(t, fallback)
		assert.Equal(t, "llm_fallback", fallback.Code)
		assert.Equal(t, "declined", fallback.Reason)
		require.Len(t, citations, 1)
		assert.Equal(t, "T", citations[0].Title)
	})

	t.Run("empty array", func(t *testing.T) {
		citations, fallback, err := unmarshalCitationsColumn([]byte(`[]`))
		require.NoError(t, err)
		assert.Nil(t, fallback)
		assert.Empty(t, citations)
	})
}

// Round-tripping is what a reader of history depends on.
func TestCitationsColumn_RoundTrip(t *testing.T) {
	in := []domain.AugurCitation{{URL: "https://x", Title: "T", Kind: domain.CitationKindArticle, RefID: "id"}}
	marker := &domain.AugurFallback{Code: "retrieval_empty"}

	payload, err := marshalCitationsColumn(in, marker)
	require.NoError(t, err)

	out, fallback, err := unmarshalCitationsColumn(payload)
	require.NoError(t, err)
	assert.Equal(t, in, out)
	assert.Equal(t, marker, fallback)
}
