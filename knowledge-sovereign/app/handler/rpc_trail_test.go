package handler

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
)

// The trail read path returns collapsed contacts (D24) and never surfaces the
// raw ML tag vocabulary (D25): junk tags are dropped and variants merged
// before they leave the service.
func TestGetTrailFootprints_MapsCollapseAndCleansTags(t *testing.T) {
	first := time.Date(2026, 6, 27, 18, 37, 0, 0, time.UTC)
	latest := time.Date(2026, 7, 7, 22, 20, 0, 0, time.UTC)
	repo := &mockRepo{trailFootprints: []sovereign_db.TrailFootprint{{
		FootprintKey:    "open:article:1",
		Verb:            "read",
		ItemKey:         "article:1",
		Title:           "US military courts in the UK",
		Tags:            []string{"Military", "also", "5", "agents", "agent", "military"},
		OccurredAt:      latest,
		FirstOccurredAt: first,
		ContactCount:    2,
		Wear:            "worn",
	}}}
	h := NewSovereignHandler(repo)

	resp, err := h.GetTrailFootprints(context.Background(), connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId: "22222222-2222-2222-2222-222222222222",
		Limit:  20,
	}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.Footprints, 1)

	fp := resp.Msg.Footprints[0]
	assert.Equal(t, int32(2), fp.ContactCount)
	require.NotNil(t, fp.FirstOccurredAt, "the earliest collapsed contact must be mapped")
	assert.Equal(t, first, fp.FirstOccurredAt.AsTime())
	assert.Equal(t, latest, fp.OccurredAt.AsTime())
	assert.Equal(t, []string{"military", "agent"}, fp.Tags,
		"tags must be cleaned (stopwords/digits dropped, case and plural variants merged)")
}
