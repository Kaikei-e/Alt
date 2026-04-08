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

func TestCausalStrategy_Retrieve_UsesPlannerSearchQueries(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	baseQuery := "物流危機の原因"
	plannerQuery1 := "物流危機 サプライチェーン 原因"
	plannerQuery2 := "global logistics crisis root cause"
	stub := &causalStubRetrieve{
		outputs: map[string]*RetrieveContextOutput{
			baseQuery: &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "base", Title: "Base", Score: 0.20, RerankScore: 0.20}},
			},
			plannerQuery1: &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "supply", Title: "Supply", Score: 0.25, RerankScore: 0.25}},
			},
			plannerQuery2: &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "geo", Title: "Geo", Score: 0.85, RerankScore: 0.85}},
			},
		},
	}
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 1)
	s := NewCausalStrategy(stub, assessor, logger)

	output, err := s.Retrieve(context.Background(), RetrieveContextInput{Query: baseQuery}, QueryIntent{
		IntentType:    IntentCausalExplanation,
		UserQuestion:  baseQuery,
		SearchQueries: []string{plannerQuery1, plannerQuery2},
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Len(t, output.Contexts, 1)
	assert.Equal(t, "Geo", output.Contexts[0].Title)
	// Should use planner queries + keyword-augmented base query
	assert.Equal(t, []string{baseQuery, plannerQuery1, plannerQuery2}, stub.queries[:3])
}

func TestCausalStrategy_Retrieve_FallbackWithoutPlannerQueries(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	baseQuery := "物流危機の原因"
	stub := &causalStubRetrieve{
		outputs: map[string]*RetrieveContextOutput{
			baseQuery: &RetrieveContextOutput{
				Contexts: []ContextItem{{ChunkText: "base", Title: "Base", Score: 0.85, RerankScore: 0.85}},
			},
		},
	}
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 1)
	s := NewCausalStrategy(stub, assessor, logger)

	output, err := s.Retrieve(context.Background(), RetrieveContextInput{Query: baseQuery}, QueryIntent{
		IntentType:   IntentCausalExplanation,
		UserQuestion: baseQuery,
		// No SearchQueries — uses base query + keyword augmentation
	})
	require.NoError(t, err)
	require.NotNil(t, output)
	assert.Equal(t, baseQuery, stub.queries[0])
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
				Contexts: []ContextItem{{ChunkText: "base", Title: "Base", Score: 0.20, RerankScore: 0.20}},
			},
		},
	}
	assessor := NewRetrievalQualityAssessor(0.5, 0.25, 1)
	s := NewCausalStrategy(stub, assessor, logger)

	// Without planner queries: returns best available (graceful degradation)
	output, err := s.Retrieve(context.Background(), RetrieveContextInput{Query: baseQuery}, QueryIntent{
		IntentType:   IntentCausalExplanation,
		UserQuestion: baseQuery,
	})
	require.NoError(t, err)
	// Graceful degradation: returns best result even if not Good quality
	require.NotNil(t, output, "should return best available result for low-confidence generation")
	assert.Equal(t, baseQuery, stub.queries[0])
}

func TestIntentCausalExplanation_IsValidConstant(t *testing.T) {
	assert.Equal(t, IntentType("causal_explanation"), IntentCausalExplanation)
}
