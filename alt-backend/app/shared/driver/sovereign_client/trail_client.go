package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// GetTrailFootprints fetches the user's derived episode spine (D24/D30) and
// open branches. The legacy footprints return is superseded and always empty
// once the provider ships episodes.
func (c *Client) GetTrailFootprints(ctx context.Context, userID uuid.UUID, cursor string, limit int, filterTags []string) ([]domain.TrailFootprint, []domain.TrailBranch, []domain.TrailEpisode, string, bool, error) {
	if !c.enabled {
		return nil, nil, nil, "", false, nil
	}

	resp, err := c.client.GetTrailFootprints(ctx, connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId:     userID.String(),
		Cursor:     cursor,
		Limit:      safeconv.Int32(limit),
		FilterTags: filterTags,
	}))
	if err != nil {
		return nil, nil, nil, "", false, fmt.Errorf("sovereign GetTrailFootprints: %w", err)
	}

	footprints := make([]domain.TrailFootprint, len(resp.Msg.Footprints))
	for i, pb := range resp.Msg.Footprints {
		footprints[i] = protoToTrailFootprint(pb)
	}

	branches := protoToTrailBranches(resp.Msg.Branches)
	episodes := protoToTrailEpisodes(resp.Msg.Episodes)

	return footprints, branches, episodes, resp.Msg.NextCursor, resp.Msg.HasMore, nil
}

// SearchTrailFootprints narrows the derived episode spine to episodes
// containing at least one footprint whose item_key is in itemKeys (trail
// search capability, D25). It reuses the same GetTrailFootprints RPC as
// GetTrailFootprints above, narrowed server-side via filter_item_keys; a
// single cursor-less call with a generously-sized limit is sufficient since
// the caller pages the search hits, not the sovereign call.
func (c *Client) SearchTrailFootprints(ctx context.Context, userID uuid.UUID, itemKeys []string, limit int) ([]domain.TrailEpisode, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetTrailFootprints(ctx, connect.NewRequest(&sovereignv1.GetTrailFootprintsRequest{
		UserId:         userID.String(),
		Cursor:         "",
		Limit:          safeconv.Int32(limit),
		FilterItemKeys: itemKeys,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetTrailFootprints (search): %w", err)
	}

	return protoToTrailEpisodes(resp.Msg.Episodes), nil
}

// GetTrailBranchesForAnchor fetches the user's open branches anchored on one
// item — the anchor branch (D26) patch-exit surface shown at the article read-end.
func (c *Client) GetTrailBranchesForAnchor(ctx context.Context, userID uuid.UUID, anchorItemKey string, limit int) ([]domain.TrailBranch, error) {
	if !c.enabled {
		return nil, nil
	}

	resp, err := c.client.GetTrailBranchesForAnchor(ctx, connect.NewRequest(&sovereignv1.GetTrailBranchesForAnchorRequest{
		UserId:        userID.String(),
		AnchorItemKey: anchorItemKey,
		Limit:         safeconv.Int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetTrailBranchesForAnchor: %w", err)
	}
	return protoToTrailBranches(resp.Msg.Branches), nil
}
