package usecase

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type causalStubRetrieve struct {
	outputs map[string]*RetrieveContextOutput
	queries []string
}

func (s *causalStubRetrieve) Execute(ctx context.Context, input RetrieveContextInput) (*RetrieveContextOutput, error) {
	s.queries = append(s.queries, input.Query)
	if out, ok := s.outputs[input.Query]; ok {
		return out, nil
	}
	return nil, nil
}

func TestCausalStrategy_Name(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	s := NewCausalStrategy(nil, nil, logger)
	assert.Equal(t, "causal", s.Name())
}

func TestCausalStrategy_Retrieve_DecomposesSubqueries(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	baseQuery := "物流危機の原因"
	stub := &causalStubRetrieve{
		outputs: map[string]*RetrieveContextOutput{
			baseQuery: &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "base", Title: "Base", Score: 0.3, RerankScore: 0.3}},
			},
			baseQuery + " 供給 制裁 sanctions supply": &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "supply", Title: "Supply", Score: 0.35, RerankScore: 0.35}},
			},
			baseQuery + " 地政学 geopolitical conflict": &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "geo", Title: "Geo", Score: 0.85, RerankScore: 0.85}},
			},
		},
	}
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 1)
	s := NewCausalStrategy(stub, assessor, logger)

	output, err := s.Retrieve(context.Background(), RetrieveContextInput{Query: baseQuery}, QueryIntent{
		IntentType:   IntentCausalExplanation,
		UserQuestion: baseQuery,
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Len(t, output.Contexts, 1)
	assert.Equal(t, "Geo", output.Contexts[0].Title)
	assert.Equal(t, []string{
		baseQuery,
		baseQuery + " 供給 制裁 sanctions supply",
		baseQuery + " 地政学 geopolitical conflict",
	}, stub.queries)
}

func TestCausalStrategy_Retrieve_EarlyExitOnGoodBaseResult(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	baseQuery := "物流危機の原因"
	stub := &causalStubRetrieve{
		outputs: map[string]*RetrieveContextOutput{
			baseQuery: &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "base", Title: "Base", Score: 0.9, RerankScore: 0.9}},
			},
		},
	}
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 1)
	s := NewCausalStrategy(stub, assessor, logger)

	output, err := s.Retrieve(context.Background(), RetrieveContextInput{Query: baseQuery}, QueryIntent{
		IntentType:   IntentCausalExplanation,
		UserQuestion: baseQuery,
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Len(t, output.Contexts, 1)
	assert.Equal(t, "Base", output.Contexts[0].Title)
	assert.Equal(t, []string{baseQuery}, stub.queries)
}

func TestCausalStrategy_Retrieve_DetailedQueryRequiresGoodVerdict(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	baseQuery := "物流危機の背景を詳しく教えて"
	stub := &causalStubRetrieve{
		outputs: map[string]*RetrieveContextOutput{
			baseQuery: {
				Contexts: []ContextItem{{ChunkText: "base", Title: "Base", Score: 0.31, RerankScore: 0.31}},
			},
			baseQuery + " 供給 制裁 sanctions supply": {
				Contexts: []ContextItem{{ChunkText: "supply", Title: "Supply", Score: 0.34, RerankScore: 0.34}},
			},
			baseQuery + " 地政学 geopolitical conflict": {
				Contexts: []ContextItem{{ChunkText: "geo", Title: "Geo", Score: 0.33, RerankScore: 0.33}},
			},
			baseQuery + " 経済 market price impact": {
				Contexts: []ContextItem{{ChunkText: "market", Title: "Market", Score: 0.32, RerankScore: 0.32}},
			},
		},
	}
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 1)
	s := NewCausalStrategy(stub, assessor, logger)

	output, err := s.Retrieve(context.Background(), RetrieveContextInput{Query: baseQuery}, QueryIntent{
		IntentType:   IntentCausalExplanation,
		UserQuestion: baseQuery,
	})
	require.NoError(t, err)
	assert.Nil(t, output)
	assert.Equal(t, []string{
		baseQuery,
		baseQuery + " 供給 制裁 sanctions supply",
		baseQuery + " 地政学 geopolitical conflict",
		baseQuery + " 経済 market price impact",
	}, stub.queries)
}

func TestIntentCausalExplanation_IsValidConstant(t *testing.T) {
	assert.Equal(t, IntentType("causal_explanation"), IntentCausalExplanation)
}
