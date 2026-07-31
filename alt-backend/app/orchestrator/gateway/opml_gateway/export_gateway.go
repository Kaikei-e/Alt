package opml_gateway

import (
	"alt/domain"
	"context"
	"net/url"
)

// FeedLinkExportStore is the one capability OPML export needs.
//
// Its predecessor built the query here and issued it through
// AltDBRepository.GetPool() — the last gateway in the module reaching past the
// driver layer for a raw pool (capability catalog §4-7). With alt-data-hub
// owning alt_db there is no pool to reach for, so the layering violation is
// not merely fixed but unreachable.
type FeedLinkExportStore interface {
	FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error)
}

// ExportGateway implements opml_port.ExportOPMLPort.
type ExportGateway struct {
	store FeedLinkExportStore
}

func NewExportGateway(store FeedLinkExportStore) *ExportGateway {
	if store == nil {
		panic("opml_gateway: FeedLinkExportStore is required (see .claude/rules/di-wiring.md)")
	}
	return &ExportGateway{store: store}
}

func (g *ExportGateway) FetchFeedLinksForExport(ctx context.Context) ([]*domain.FeedLinkForExport, error) {
	links, err := g.store.FetchFeedLinksForExport(ctx)
	if err != nil {
		return nil, err
	}

	// The hostname fallback stays here. An empty title means no feed has ever
	// been collected for the link, and what to show instead is a rendering
	// decision — the data plane reports the fact and nothing more.
	for _, link := range links {
		if link.Title != "" {
			continue
		}
		if parsed, parseErr := url.Parse(link.URL); parseErr == nil && parsed.Host != "" {
			link.Title = parsed.Host
		}
	}
	return links, nil
}
