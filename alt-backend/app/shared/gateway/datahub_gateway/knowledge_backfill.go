package datahub_gateway

import (
	"context"
	"fmt"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/gen/proto/services/datahub/v1/datahubv1connect"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// KnowledgeBackfillGateway is the alt_db half of alt-backend's knowledge
// backfill jobs (catalog §2.N).
//
// Only the reads cross this boundary. The sovereign append, the progress
// bookkeeping and the reprojection stay in alt-backend's usecases, because
// talking to another service is the caller's business under ADR-000954 D4 —
// routing them through the data hub would make it the hub for everything
// rather than the owner of one database.
type KnowledgeBackfillGateway struct {
	client datahubv1connect.DataHubServiceClient
}

func NewKnowledgeBackfillGateway(client datahubv1connect.DataHubServiceClient) *KnowledgeBackfillGateway {
	if client == nil {
		panic("datahub_gateway: KnowledgeBackfillGateway requires a DataHubService client (see .claude/rules/di-wiring.md)")
	}
	return &KnowledgeBackfillGateway{client: client}
}

func (g *KnowledgeBackfillGateway) CountBackfillArticles(ctx context.Context) (int, error) {
	resp, err := g.client.CountBackfillArticles(ctx, connect.NewRequest(&datahubv1.CountBackfillArticlesRequest{}))
	if err != nil {
		return 0, fmt.Errorf("count backfill articles: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

// ListBackfillArticles walks (created_at, article_id) ascending.
//
// Both cursor parts go on the wire together or not at all. The provider
// rejects half a cursor; sending one would restart the walk and re-emit every
// knowledge event the job had already emitted.
func (g *KnowledgeBackfillGateway) ListBackfillArticles(ctx context.Context, lastCreatedAt *time.Time, lastArticleID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillArticle, error) {
	req := &datahubv1.ListBackfillArticlesRequest{Limit: safeconv.Int32(limit)}
	if lastCreatedAt != nil && lastArticleID != nil {
		req.LastCreatedAt = timePtrToProto(lastCreatedAt)
		req.LastArticleId = lastArticleID.String()
	}

	resp, err := g.client.ListBackfillArticles(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("list backfill articles: %w", err)
	}

	out := make([]domain.KnowledgeBackfillArticle, 0, len(resp.Msg.GetArticles()))
	for _, msg := range resp.Msg.GetArticles() {
		article, convErr := backfillArticleFromProto(msg)
		if convErr != nil {
			return nil, convErr
		}
		out = append(out, article)
	}
	return out, nil
}

func (g *KnowledgeBackfillGateway) CountBackfillSummaryTitles(ctx context.Context) (int, error) {
	resp, err := g.client.CountBackfillSummaryTitles(ctx, connect.NewRequest(&datahubv1.CountBackfillSummaryTitlesRequest{}))
	if err != nil {
		return 0, fmt.Errorf("count backfill summary titles: %w", err)
	}
	return int(resp.Msg.GetCount()), nil
}

// ListBackfillSummaryTitles walks (generated_at, summary_version_id)
// ascending, with the same all-or-nothing cursor rule.
func (g *KnowledgeBackfillGateway) ListBackfillSummaryTitles(ctx context.Context, lastGeneratedAt *time.Time, lastSummaryVersionID *uuid.UUID, limit int) ([]domain.KnowledgeBackfillSummaryTitle, error) {
	req := &datahubv1.ListBackfillSummaryTitlesRequest{Limit: safeconv.Int32(limit)}
	if lastGeneratedAt != nil && lastSummaryVersionID != nil {
		req.LastGeneratedAt = timePtrToProto(lastGeneratedAt)
		req.LastSummaryVersionId = lastSummaryVersionID.String()
	}

	resp, err := g.client.ListBackfillSummaryTitles(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, fmt.Errorf("list backfill summary titles: %w", err)
	}

	out := make([]domain.KnowledgeBackfillSummaryTitle, 0, len(resp.Msg.GetEntries()))
	for _, msg := range resp.Msg.GetEntries() {
		entry, convErr := backfillSummaryTitleFromProto(msg)
		if convErr != nil {
			return nil, convErr
		}
		out = append(out, entry)
	}
	return out, nil
}
