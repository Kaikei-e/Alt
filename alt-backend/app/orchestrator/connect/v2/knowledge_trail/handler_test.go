package knowledge_trail

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/domain"
	knowledgetrailv1 "alt/gen/proto/alt/knowledge_trail/v1"
	"alt/orchestrator/usecase/get_knowledge_trail_usecase"
)

type fakeTrailPort struct {
	footprints []domain.TrailFootprint
}

func (f *fakeTrailPort) GetTrailFootprints(_ context.Context, _ uuid.UUID, _ string, _ int, _ []string) ([]domain.TrailFootprint, []domain.TrailBranch, string, bool, error) {
	return f.footprints, nil, "", false, nil
}

func userCtx() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.MustParse("22222222-2222-2222-2222-222222222222"),
		TenantID:  uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		Email:     "trail@example.com",
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

// A collapsed footprint's contact count and first contact time must survive
// the BFF mapping — dropping either silently regresses the spine back to the
// one-row-per-day duplicate display (D24).
func TestGetTrail_MapsCollapsedContactFields(t *testing.T) {
	port := &fakeTrailPort{footprints: []domain.TrailFootprint{{
		FootprintKey:    "open:article:1",
		Verb:            "read",
		ItemKey:         "article:1",
		Title:           "US military courts in the UK",
		OccurredAt:      time.Date(2026, 7, 7, 22, 20, 0, 0, time.UTC),
		FirstOccurredAt: time.Date(2026, 6, 27, 18, 37, 0, 0, time.UTC),
		ContactCount:    2,
		Wear:            "worn",
	}}}
	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port), nil, nil, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Footprints, 1)

	fp := resp.Msg.Footprints[0]
	assert.Equal(t, int32(2), fp.ContactCount)
	assert.Equal(t, "2026-06-27T18:37:00Z", fp.FirstOccurredAt)
	assert.Equal(t, "2026-07-07T22:20:00Z", fp.OccurredAt)
}

// A single-contact footprint still reports itself honestly: count 1 and a
// first contact equal to the latest.
func TestGetTrail_SingleContactKeepsCountOne(t *testing.T) {
	at := time.Date(2026, 7, 17, 9, 0, 0, 0, time.UTC)
	port := &fakeTrailPort{footprints: []domain.TrailFootprint{{
		FootprintKey:    "open:article:2",
		Verb:            "read",
		ItemKey:         "article:2",
		OccurredAt:      at,
		FirstOccurredAt: at,
		ContactCount:    1,
	}}}
	h := NewHandler(get_knowledge_trail_usecase.NewGetKnowledgeTrailUsecase(port), nil, nil, slog.Default())

	resp, err := h.GetTrail(userCtx(), connect.NewRequest(&knowledgetrailv1.GetTrailRequest{Limit: 20}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Footprints, 1)
	assert.Equal(t, int32(1), resp.Msg.Footprints[0].ContactCount)
	assert.Equal(t, resp.Msg.Footprints[0].OccurredAt, resp.Msg.Footprints[0].FirstOccurredAt)
}
