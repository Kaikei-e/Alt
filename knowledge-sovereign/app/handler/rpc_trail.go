package handler

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"knowledge-sovereign/driver/sovereign_db"
	sovereignv1 "knowledge-sovereign/gen/proto/services/sovereign/v1"
	"knowledge-sovereign/usecase/tagclean"
	"knowledge-sovereign/usecase/trail_episodes"
)

// episodeWindowRows bounds how many collapsed footprint rows are fetched
// from the read model to derive episodes from (Wave 8). Episodes are paged
// in the handler, over this window, independently of the underlying
// footprint row count.
const episodeWindowRows = 500

// episodeCursorPrefix marks a handler-owned episode-page cursor, distinct
// from the read model's own (occurred_at, footprint_key) footprint cursor.
const episodeCursorPrefix = "ep:"

// GetTrailFootprints returns the user's trail spine as derived episodes
// (D24/D30, Wave 8). The legacy flat `footprints` field is superseded and
// always empty; episodes are the sole display unit.
func (h *SovereignHandler) GetTrailFootprints(
	ctx context.Context,
	req *connect.Request[sovereignv1.GetTrailFootprintsRequest],
) (*connect.Response[sovereignv1.GetTrailFootprintsResponse], error) {
	msg := req.Msg
	userID, err := parseUUIDField("user_id", msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	offset, err := parseEpisodeCursor(msg.Cursor)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Episodes derive over a fixed window of the read model, independent of
	// the client's page cursor/limit — those apply to the derived episode
	// list, not the underlying footprint fetch.
	window, _, _, err := h.readDB.GetTrailFootprints(ctx, userID, "", episodeWindowRows, msg.FilterTags)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetTrailFootprints: %w", err))
	}
	episodes := trail_episodes.Derive(window)
	episodes = filterEpisodesByItemKeys(episodes, msg.FilterItemKeys)

	limit := int(msg.Limit)
	if limit < 0 {
		limit = 0
	}
	if offset > len(episodes) {
		offset = len(episodes)
	}
	end := offset + limit
	if end > len(episodes) {
		end = len(episodes)
	}
	page := episodes[offset:end]
	hasMore := end < len(episodes)
	var nextCursor string
	if hasMore {
		nextCursor = fmt.Sprintf("%s%d", episodeCursorPrefix, end)
	}

	branches, err := h.readDB.GetOpenTrailBranches(ctx, userID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("GetOpenTrailBranches: %w", err))
	}

	pbEpisodes := make([]*sovereignv1.TrailEpisode, len(page))
	for i, ep := range page {
		pbEpisodes[i] = &sovereignv1.TrailEpisode{
			EpisodeKey: ep.EpisodeKey,
			Wear:       ep.Wear,
			Footprints: mapTrailFootprints(ep.Footprints),
		}
	}

	pbBranches := make([]*sovereignv1.TrailBranch, len(branches))
	for i, b := range branches {
		refs := make([]*sovereignv1.TrailEvidenceRef, len(b.EvidenceRefs))
		for j, r := range b.EvidenceRefs {
			refs[j] = &sovereignv1.TrailEvidenceRef{RefId: r.RefID, Label: r.Label, Kind: r.Kind}
		}
		pbBranches[i] = &sovereignv1.TrailBranch{
			BranchKey:     b.BranchKey,
			AnchorItemKey: b.AnchorItemKey,
			RelationKind:  b.RelationKind,
			Why:           b.Why,
			EvidenceRefs:  refs,
			Confidence:    b.Confidence,
			TargetItemKey: b.TargetItemKey,
			TargetTitle:   b.TargetTitle,
		}
	}

	return connect.NewResponse(&sovereignv1.GetTrailFootprintsResponse{
		// Footprints is superseded by episodes (Wave 8) — left empty.
		NextCursor: nextCursor,
		HasMore:    hasMore,
		Branches:   pbBranches,
		Episodes:   pbEpisodes,
	}), nil
}

// mapTrailFootprints maps read-model footprints to their wire form, cleaning
// display tags (D25) so the raw ML tag vocabulary never leaves the service.
func mapTrailFootprints(footprints []sovereign_db.TrailFootprint) []*sovereignv1.TrailFootprint {
	pb := make([]*sovereignv1.TrailFootprint, len(footprints))
	for i, fp := range footprints {
		// The earliest contact defaults to the latest for legacy single-contact
		// rows.
		firstOccurredAt := fp.FirstOccurredAt
		if firstOccurredAt.IsZero() {
			firstOccurredAt = fp.OccurredAt
		}
		contactCount := max(fp.ContactCount, 1)
		pb[i] = &sovereignv1.TrailFootprint{
			UserId:          fp.UserID.String(),
			TenantId:        fp.TenantID.String(),
			FootprintKey:    fp.FootprintKey,
			Verb:            fp.Verb,
			ItemKey:         fp.ItemKey,
			Title:           fp.Title,
			Excerpt:         fp.Excerpt,
			Tags:            tagclean.CleanDisplay(fp.Tags),
			Note:            fp.Note,
			SourceEventType: fp.SourceEventType,
			OccurredAt:      timestamppb.New(fp.OccurredAt),
			Wear:            fp.Wear,
			ContactCount:    int32(contactCount), //nolint:gosec // >= 1, bounded by row count
			FirstOccurredAt: timestamppb.New(firstOccurredAt),
		}
	}
	return pb
}

// filterEpisodesByItemKeys narrows episodes to those containing at least one
// footprint whose ItemKey is in itemKeys (Wave 9 — trail search, D25). A
// matching episode surfaces in full, including member footprints whose
// ItemKey did not itself match — the filter narrows *which* episodes surface,
// not what a surfaced episode contains (episodes are the unit of context).
// An empty itemKeys leaves episodes unchanged.
func filterEpisodesByItemKeys(episodes []trail_episodes.Episode, itemKeys []string) []trail_episodes.Episode {
	if len(itemKeys) == 0 {
		return episodes
	}
	want := make(map[string]struct{}, len(itemKeys))
	for _, k := range itemKeys {
		want[k] = struct{}{}
	}
	filtered := make([]trail_episodes.Episode, 0, len(episodes))
	for _, ep := range episodes {
		for _, fp := range ep.Footprints {
			if _, ok := want[fp.ItemKey]; ok {
				filtered = append(filtered, ep)
				break
			}
		}
	}
	return filtered
}

// parseEpisodeCursor parses the handler-owned "ep:<offset>" episode-page
// cursor. An empty cursor is the first page. Anything else that doesn't
// parse is rejected rather than silently reset to page 1.
func parseEpisodeCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	rest, ok := strings.CutPrefix(cursor, episodeCursorPrefix)
	if !ok {
		return 0, fmt.Errorf("invalid episode cursor %q", cursor)
	}
	offset, err := strconv.Atoi(rest)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid episode cursor %q", cursor)
	}
	return offset, nil
}
