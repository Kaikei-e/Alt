package datahubapi

import (
	"context"
	"errors"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WithWave3Batch6Capabilities wires the two capabilities ADR-000954 Wave 3
// batch 6 moved — the Tag Trail's paged reads (catalog §2.J) and the recall
// rail's article fallback (§2.C).
//
// These are the last two, and after them alt-backend has no database pool at
// all. Both refuse nil for the same reason every batch before them did, but
// the failure they prevent is quieter than most: neither of these reads
// returning nothing looks like an error to its caller. An empty Tag Trail page
// renders as "this tag has no articles", and a recall candidate with no
// article behind it is simply skipped. A nil port here would therefore produce
// a working-looking product with two features missing, which is exactly the
// state CLAUDE.md rule 8 exists to make impossible.
func WithWave3Batch6Capabilities(
	tagTrail datahub_capability_port.TagTrailPort,
	articleRef datahub_capability_port.ArticleRefPort,
) HandlerOption {
	switch {
	case tagTrail == nil:
		panic("datahubapi: TagTrailPort is required — an unwired Tag Trail answers every tag with an empty page, which the UI renders as 'no articles' rather than as a fault")
	case articleRef == nil:
		panic("datahubapi: ArticleRefPort is required — an unwired recall fallback silently drops exactly the items the fallback exists to render")
	}

	return func(h *Handler) {
		h.tagTrail = tagTrail
		h.articleRef = articleRef
	}
}

// ---------------------------------------------------------------------------
// §2.J Tag Trail paging
// ---------------------------------------------------------------------------

func (h *Handler) ListArticlesByTagID(ctx context.Context, req *connect.Request[datahubv1.ListArticlesByTagIDRequest]) (*connect.Response[datahubv1.ListArticlesByTagIDResponse], error) {
	tagID := req.Msg.GetTagId()
	if tagID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tag_id is required"))
	}

	articles, err := h.tagTrail.ArticlesByTagID(ctx, tagID, cursorFromProto(req.Msg.GetCursor()), clampTagLimit(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListArticlesByTagID failed", "error", err, "tag_id", tagID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles by tag id"))
	}
	return connect.NewResponse(&datahubv1.ListArticlesByTagIDResponse{
		Articles: tagTrailArticlesToProto(articles),
	}), nil
}

func (h *Handler) ListArticlesByTagName(ctx context.Context, req *connect.Request[datahubv1.ListArticlesByTagNameRequest]) (*connect.Response[datahubv1.ListArticlesByTagNameResponse], error) {
	tagName := req.Msg.GetTagName()
	if tagName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tag_name is required"))
	}

	articles, err := h.tagTrail.ArticlesByTagName(ctx, tagName, cursorFromProto(req.Msg.GetCursor()), clampTagLimit(req.Msg.GetLimit()))
	if err != nil {
		h.logger.ErrorContext(ctx, "ListArticlesByTagName failed", "error", err, "tag_name", tagName)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list articles by tag name"))
	}
	return connect.NewResponse(&datahubv1.ListArticlesByTagNameResponse{
		Articles: tagTrailArticlesToProto(articles),
	}), nil
}

// ---------------------------------------------------------------------------
// §2.C Article reference for the recall rail
// ---------------------------------------------------------------------------

// GetArticleTitleAndLink answers a missing article with found=false and no
// error.
//
// NotFound would be the wrong Connect code here. The rail asks about
// candidates that knowledge-sovereign proposed, and an article deleted since
// then is an ordinary outcome the caller renders around — turning it into an
// error would put a routine miss in every error budget and every log.
func (h *Handler) GetArticleTitleAndLink(ctx context.Context, req *connect.Request[datahubv1.GetArticleTitleAndLinkRequest]) (*connect.Response[datahubv1.GetArticleTitleAndLinkResponse], error) {
	articleID := req.Msg.GetArticleId()
	if articleID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	ref, err := h.articleRef.ArticleRef(ctx, articleID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetArticleTitleAndLink failed", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get article title and link"))
	}
	if ref == nil {
		return connect.NewResponse(&datahubv1.GetArticleTitleAndLinkResponse{Found: false}), nil
	}

	resp := &datahubv1.GetArticleTitleAndLinkResponse{
		Found: true,
		Title: ref.Title,
		Url:   ref.Link,
	}
	if ref.PublishedAt != nil {
		resp.PublishedAt = timestamppb.New(*ref.PublishedAt)
	}
	return connect.NewResponse(resp), nil
}

// ---------------------------------------------------------------------------
// Conversion
// ---------------------------------------------------------------------------

// cursorFromProto turns an unset timestamp into a nil cursor rather than into
// the zero time. The port reads nil as "first page" and builds a query with no
// upper bound; the zero time would build `created_at < '0001-01-01'`, which
// selects nothing and looks like an empty tag.
func cursorFromProto(ts *timestamppb.Timestamp) *time.Time {
	if ts == nil {
		return nil
	}
	t := ts.AsTime()
	return &t
}

func tagTrailArticlesToProto(articles []*domain.TagTrailArticle) []*datahubv1.TagTrailArticle {
	out := make([]*datahubv1.TagTrailArticle, 0, len(articles))
	for _, a := range articles {
		if a == nil {
			continue
		}
		out = append(out, &datahubv1.TagTrailArticle{
			Id:          a.ID,
			Title:       a.Title,
			Url:         a.Link,
			PublishedAt: timestamppb.New(a.PublishedAt),
			FeedId:      a.FeedID,
			FeedTitle:   a.FeedTitle,
		})
	}
	return out
}
