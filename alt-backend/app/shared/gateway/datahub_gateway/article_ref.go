package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"

	"connectrpc.com/connect"
)

// ArticleRefGateway satisfies recall_candidate_port.ArticleFallbackPort — the
// recall rail's read for candidates the knowledge_home_items projection has
// not caught up with (catalog §2.C, ADR-000954 Wave 3 batch 6).
//
// It is a gateway of its own rather than a method on ArticleStoreGateway or
// ArticleBatchGateway because of who holds it. Those are held by every surface
// that renders articles; this one is held by a single usecase and answers a
// question only that usecase asks — "the projection is behind, what did this
// id used to be". Folding it in would hand the whole article surface a method
// that only makes sense during projection lag.
//
// This was the last read alt-backend performed against its own database pool.
type ArticleRefGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewArticleRefGateway(client datahubv1connect.DataHubServiceClient) *ArticleRefGateway {
	if client == nil {
		panic("datahub_gateway: ArticleRefGateway requires a DataHubService client — " +
			"a nil client would make every recall candidate whose projection is " +
			"behind vanish from the rail without an error anyone could see " +
			"(see .claude/rules/di-wiring.md)")
	}
	return &ArticleRefGateway{client: client}
}

// GetArticleTitleAndLink returns the article's title, url and published_at.
//
// A missing article is an empty title and a nil error, which is what the
// caller has always read — it skips candidates whose title comes back empty.
// The provider says so explicitly with `found`, and this method collapses that
// back into the caller's shape rather than inventing an error the recall rail
// has no branch for: an article deleted since sovereign proposed it is an
// ordinary outcome, not a fault.
func (g *ArticleRefGateway) GetArticleTitleAndLink(ctx context.Context, articleID string) (string, string, *time.Time, error) {
	resp, err := g.client.GetArticleTitleAndLink(ctx, connect.NewRequest(&datahubv1.GetArticleTitleAndLinkRequest{
		ArticleId: articleID,
	}))
	if err != nil {
		return "", "", nil, fmt.Errorf("get article title and link %s: %w", articleID, err)
	}
	if !resp.Msg.GetFound() {
		return "", "", nil, nil
	}

	var publishedAt *time.Time
	if ts := resp.Msg.GetPublishedAt(); ts != nil && ts.IsValid() {
		t := ts.AsTime()
		publishedAt = &t
	}
	return resp.Msg.GetTitle(), resp.Msg.GetUrl(), publishedAt, nil
}
