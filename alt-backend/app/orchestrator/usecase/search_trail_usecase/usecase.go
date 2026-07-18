// Package search_trail_usecase implements trail search (Wave 9, D25):
// full-text search over what the user actually read. It reuses the existing
// article search index (search-indexer / Meilisearch) — no new index, no
// sovereign→Meilisearch coupling — then narrows the user's derived episode
// spine to the episodes containing a hit.
package search_trail_usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"alt/domain"
	"alt/orchestrator/port/search_indexer_port"
	"alt/orchestrator/port/search_trail_port"
	"alt/orchestrator/port/trail_thumbnail_port"
	"alt/orchestrator/usecase/trail_thumbnail_enrichment"

	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	// sovereignSearchWindow bounds how many episodes the narrowed
	// GetTrailFootprints call may return. It mirrors knowledge-sovereign's own
	// episodeWindowRows derivation window — generously large so a hit several
	// episodes back isn't silently dropped before the caller's own limit
	// truncates the final page.
	sovereignSearchWindow = 500

	// articleItemKeyPrefix anchors a search hit's article id back to the
	// item_key vocabulary the trail spine uses.
	articleItemKeyPrefix = "article:"
)

// ErrInvalidRequest wraps client-side validation failures so the handler can
// map them to InvalidArgument — mirrors the ErrInvalidRequest convention of
// resolve_trail_branch_usecase / emit_trail_outcome_usecase.
var ErrInvalidRequest = errors.New("search_trail_usecase: invalid request")

// Result is the trail search response: episodes containing at least one
// matching item, plus the subset of searched item_keys that actually appear
// among their members (D25 — anchors the hit inside its episode).
type Result struct {
	Episodes        []domain.TrailEpisode
	MatchedItemKeys []string
}

// SearchTrailUsecase performs full-text search over the user's read history:
// it searches the existing article index, then narrows the user's episode
// spine to episodes containing a hit.
type SearchTrailUsecase struct {
	searchPort    search_indexer_port.SearchIndexerPort
	trailPort     search_trail_port.SearchTrailPort
	thumbnailPort trail_thumbnail_port.GetOgImageURLsPort
}

func NewSearchTrailUsecase(
	searchPort search_indexer_port.SearchIndexerPort,
	trailPort search_trail_port.SearchTrailPort,
	thumbnailPort trail_thumbnail_port.GetOgImageURLsPort,
) *SearchTrailUsecase {
	return &SearchTrailUsecase{searchPort: searchPort, trailPort: trailPort, thumbnailPort: thumbnailPort}
}

// Execute searches article content for query, then returns the episodes of
// the user's trail spine that contain a hit, anchored (D25).
func (u *SearchTrailUsecase) Execute(ctx context.Context, userID uuid.UUID, query string, limit int) (*Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("%w: query must not be empty", ErrInvalidRequest)
	}
	if limit <= 0 || limit > maxLimit {
		limit = defaultLimit
	}

	hits, err := u.searchPort.SearchArticles(ctx, query, userID.String())
	if err != nil {
		return nil, err
	}
	if len(hits) == 0 {
		return &Result{}, nil
	}

	itemKeys := make([]string, len(hits))
	for i, h := range hits {
		itemKeys[i] = articleItemKeyPrefix + h.ID
	}

	episodes, err := u.trailPort.SearchTrailFootprints(ctx, userID, itemKeys, sovereignSearchWindow)
	if err != nil {
		return nil, err
	}

	if len(episodes) > limit {
		episodes = episodes[:limit]
	}
	episodes = trail_thumbnail_enrichment.Enrich(ctx, u.thumbnailPort, episodes)

	return &Result{Episodes: episodes, MatchedItemKeys: matchedItemKeys(itemKeys, episodes)}, nil
}

// matchedItemKeys is the subset of searched itemKeys that actually appear
// among a member of any returned episode — the sovereign filter narrows
// *which* episodes surface, but a searched key with no surfaced episode
// (e.g. it fell outside the derivation window, or the page limit trimmed it)
// must not be reported as matched.
func matchedItemKeys(itemKeys []string, episodes []domain.TrailEpisode) []string {
	present := make(map[string]struct{})
	for _, ep := range episodes {
		for _, fp := range ep.Footprints {
			present[fp.ItemKey] = struct{}{}
		}
	}
	matched := make([]string, 0, len(itemKeys))
	for _, k := range itemKeys {
		if _, ok := present[k]; ok {
			matched = append(matched, k)
		}
	}
	return matched
}
