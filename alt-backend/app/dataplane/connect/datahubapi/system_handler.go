package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"time"

	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/orchestrator/usecase/fetch_recent_articles_usecase"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
)

// -----------------------------------------------------------------------------
// Absorbed REST routes (ADR-000954 D6)
// -----------------------------------------------------------------------------

// GetSystemUser replaces GET /v1/internal/system-user.
func (h *Handler) GetSystemUser(ctx context.Context, _ *connect.Request[datahubv1.GetSystemUserRequest]) (*connect.Response[datahubv1.GetSystemUserResponse], error) {
	userID, err := h.systemUser.GetFirstIdentityID(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetSystemUser failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch system user: %w", err))
	}

	return connect.NewResponse(&datahubv1.GetSystemUserResponse{UserId: userID}), nil
}

// ListRecentArticles replaces GET /v1/internal/articles/recent.
//
// The REST route answered 400 for a non-positive within_hours and for a
// negative limit, and treated limit=0 as "time window only". Both behaviours
// are reproduced here rather than deferred to the usecase, which clamps
// silently: a caller that sends nonsense should hear about it during the wave
// where it is switching protocols, not receive a quietly different window.
func (h *Handler) ListRecentArticles(ctx context.Context, req *connect.Request[datahubv1.ListRecentArticlesRequest]) (*connect.Response[datahubv1.ListRecentArticlesResponse], error) {
	withinHours := defaultRecentWithinHours
	if v := req.Msg.WithinHours; v != nil {
		if *v <= 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("within_hours must be positive"))
		}
		withinHours = int(*v)
	}

	limit := defaultRecentLimit
	if v := req.Msg.Limit; v != nil {
		if *v < 0 {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("limit must not be negative"))
		}
		limit = int(*v)
	}

	out, err := h.recentArticles.Execute(ctx, fetch_recent_articles_usecase.FetchRecentArticlesInput{
		WithinHours: withinHours,
		Limit:       limit,
	})
	if err != nil {
		h.logger.ErrorContext(ctx, "ListRecentArticles failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("fetch recent articles: %w", err))
	}

	items := make([]*datahubv1.RecentArticleItem, len(out.Articles))
	for i, a := range out.Articles {
		items[i] = &datahubv1.RecentArticleItem{
			Id:          a.ID.String(),
			Title:       a.Title,
			Url:         a.URL,
			PublishedAt: a.PublishedAt.Format(time.RFC3339),
			FeedId:      a.FeedID.String(),
			Tags:        a.Tags,
		}
	}

	return connect.NewResponse(&datahubv1.ListRecentArticlesResponse{
		Articles: items,
		Since:    out.Since.Format(time.RFC3339),
		Until:    out.Until.Format(time.RFC3339),
		Count:    safeconv.Int32(out.Count),
	}), nil
}
