package datahubapi

import (
	"context"
	"errors"

	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
)

// ---------------------------------------------------------------------------
// §2.B Article writes
// ---------------------------------------------------------------------------

// ArchiveArticle upserts the article and appends its outbox row in one
// transaction.
//
// user_id is required and must parse. The driver refuses the zero UUID, but
// rejecting it here as well means a caller that omitted the field learns so
// from an InvalidArgument rather than from an Internal that hides a validation
// failure behind a database-shaped error.
func (h *Handler) ArchiveArticle(ctx context.Context, req *connect.Request[datahubv1.ArchiveArticleRequest]) (*connect.Response[datahubv1.ArchiveArticleResponse], error) {
	if req.Msg.GetUrl() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("url is required"))
	}
	if req.Msg.GetContent() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("content is required"))
	}

	userID, err := requiredUUID(req.Msg.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	articleID, created, err := h.articleWrite.Archive(ctx, req.Msg.GetUrl(), req.Msg.GetTitle(), req.Msg.GetContent(), userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ArchiveArticle failed", "error", err, "url", req.Msg.GetUrl())
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to archive article"))
	}

	return connect.NewResponse(&datahubv1.ArchiveArticleResponse{
		ArticleId: articleID,
		Created:   created,
	}), nil
}
