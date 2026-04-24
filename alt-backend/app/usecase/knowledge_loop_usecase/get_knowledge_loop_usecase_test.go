package knowledge_loop_usecase

import (
	"alt/domain"
	"alt/port/knowledge_loop_port"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeEntriesPort records each query so tests can pin per-bucket fetch behavior.
type fakeEntriesPort struct {
	queries  []knowledge_loop_port.GetEntriesQuery
	byBucket map[domain.SurfaceBucket][]*domain.KnowledgeLoopEntry
}

func (f *fakeEntriesPort) GetKnowledgeLoopEntries(
	_ context.Context,
	q knowledge_loop_port.GetEntriesQuery,
) ([]*domain.KnowledgeLoopEntry, error) {
	f.queries = append(f.queries, q)
	if q.SurfaceBucket == nil {
		return nil, nil
	}
	return f.byBucket[*q.SurfaceBucket], nil
}

type fakeSessionPort struct{}

func (fakeSessionPort) GetKnowledgeLoopSessionState(
	_ context.Context,
	_, _ uuid.UUID,
	_ string,
) (*domain.KnowledgeLoopSessionState, error) {
	return &domain.KnowledgeLoopSessionState{ProjectionSeqHiwater: 10}, nil
}

type fakeSurfacesPort struct{}

func (fakeSurfacesPort) GetKnowledgeLoopSurfaces(
	_ context.Context,
	_, _ uuid.UUID,
	_ string,
) ([]*domain.KnowledgeLoopSurface, error) {
	return nil, nil
}

func TestGetKnowledgeLoop_FetchesAllFourBuckets(t *testing.T) {
	entries := &fakeEntriesPort{
		byBucket: map[domain.SurfaceBucket][]*domain.KnowledgeLoopEntry{
			domain.SurfaceNow: {
				{EntryKey: "article:1", SurfaceBucket: domain.SurfaceNow, ProjectionSeqHiwater: 100},
			},
			domain.SurfaceContinue: {
				{EntryKey: "article:2", SurfaceBucket: domain.SurfaceContinue, ProjectionSeqHiwater: 101},
			},
			domain.SurfaceChanged: {
				{EntryKey: "article:3", SurfaceBucket: domain.SurfaceChanged, ProjectionSeqHiwater: 102},
			},
			domain.SurfaceReview: {
				{EntryKey: "article:4", SurfaceBucket: domain.SurfaceReview, ProjectionSeqHiwater: 103},
			},
		},
	}
	uc := NewGetKnowledgeLoopUsecase(entries, fakeSessionPort{}, fakeSurfacesPort{})

	result, err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "default", 3)
	require.NoError(t, err)

	// The usecase must hit each bucket exactly once.
	seen := map[domain.SurfaceBucket]int{}
	for _, q := range entries.queries {
		require.NotNil(t, q.SurfaceBucket, "every query must be bucket-scoped")
		seen[*q.SurfaceBucket]++
	}
	require.Equal(t, 1, seen[domain.SurfaceNow])
	require.Equal(t, 1, seen[domain.SurfaceContinue])
	require.Equal(t, 1, seen[domain.SurfaceChanged])
	require.Equal(t, 1, seen[domain.SurfaceReview])

	// Foreground carries only Now.
	require.Len(t, result.ForegroundEntries, 1)
	require.Equal(t, domain.SurfaceNow, result.ForegroundEntries[0].SurfaceBucket)

	// BucketEntries carries the other three, partitionable by SurfaceBucket field.
	require.Len(t, result.BucketEntries, 3)
	bucketKeys := map[domain.SurfaceBucket]string{}
	for _, e := range result.BucketEntries {
		bucketKeys[e.SurfaceBucket] = e.EntryKey
	}
	require.Equal(t, "article:2", bucketKeys[domain.SurfaceContinue])
	require.Equal(t, "article:3", bucketKeys[domain.SurfaceChanged])
	require.Equal(t, "article:4", bucketKeys[domain.SurfaceReview])

	// Seq hiwater is the max across everything.
	require.Equal(t, int64(103), result.ProjectionSeqHiwater)
}

func TestGetKnowledgeLoop_PerBucketLimit(t *testing.T) {
	entries := &fakeEntriesPort{byBucket: map[domain.SurfaceBucket][]*domain.KnowledgeLoopEntry{}}
	uc := NewGetKnowledgeLoopUsecase(entries, fakeSessionPort{}, fakeSurfacesPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "default", 3)
	require.NoError(t, err)

	for _, q := range entries.queries {
		if q.SurfaceBucket == nil {
			continue
		}
		if *q.SurfaceBucket == domain.SurfaceNow {
			require.Equal(t, 3, q.Limit, "foreground limit is caller-controlled")
			continue
		}
		require.Equal(t, otherBucketLimitPerBucket, q.Limit,
			"non-NOW buckets must cap at the plan-default otherBucketLimitPerBucket")
	}
}

func TestGetKnowledgeLoop_RejectsMalformedLensModeID(t *testing.T) {
	entries := &fakeEntriesPort{byBucket: map[domain.SurfaceBucket][]*domain.KnowledgeLoopEntry{}}
	uc := NewGetKnowledgeLoopUsecase(entries, fakeSessionPort{}, fakeSurfacesPort{})

	_, err := uc.Execute(context.Background(), uuid.New(), uuid.New(), "has space", 3)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.Empty(t, entries.queries, "validation must short-circuit before hitting the port")
}
