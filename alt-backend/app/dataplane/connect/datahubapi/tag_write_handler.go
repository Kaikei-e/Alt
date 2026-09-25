package datahubapi

import (
	"context"
	"encoding/json"
	"errors"

	"alt/dataplane/port/internal_tag_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ── Tag operations (tag-generator) ──

func (h *Handler) UpsertArticleTags(ctx context.Context, req *connect.Request[datahubv1.UpsertArticleTagsRequest]) (*connect.Response[datahubv1.UpsertArticleTagsResponse], error) {
	if h.upsertArticleTags == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}
	if req.Msg.ArticleId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("article_id is required"))
	}

	tags := tagItemsFromProto(req.Msg.Tags)

	count, err := h.upsertArticleTags.UpsertArticleTags(ctx, req.Msg.ArticleId, req.Msg.FeedId, tags)
	if err != nil {
		h.logger.Error("UpsertArticleTags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to upsert article tags"))
	}

	// Also create tag set version + knowledge event for Knowledge Home
	if h.createTagSetVersionUsecase != nil && len(tags) > 0 {
		articleUUID, parseErr := uuid.Parse(req.Msg.ArticleId)
		if parseErr == nil {
			tagsJSON, _ := json.Marshal(tags)
			tsv := domain.TagSetVersion{
				ArticleID: articleUUID,
				Generator: "tag-generator",
				TagsJSON:  tagsJSON,
			}
			// Resolve UserID from article
			if h.getArticleByID != nil {
				article, artErr := h.getArticleByID.GetArticleByID(ctx, req.Msg.ArticleId)
				if artErr == nil && article != nil {
					userUUID, uErr := uuid.Parse(article.UserID)
					if uErr == nil {
						tsv.UserID = userUUID
					}
				} else {
					h.logger.Warn("could not resolve user for tag set version", "article_id", req.Msg.ArticleId, "error", artErr)
				}
			}
			if tsvErr := h.createTagSetVersionUsecase.Execute(ctx, tsv); tsvErr != nil {
				h.logger.Error("failed to create tag set version", "error", tsvErr, "article_id", req.Msg.ArticleId)
			}
		}
	}

	return connect.NewResponse(&datahubv1.UpsertArticleTagsResponse{
		Success:       true,
		UpsertedCount: count,
	}), nil
}

func (h *Handler) BatchUpsertArticleTags(ctx context.Context, req *connect.Request[datahubv1.BatchUpsertArticleTagsRequest]) (*connect.Response[datahubv1.BatchUpsertArticleTagsResponse], error) {
	if h.batchUpsertArticleTags == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("not yet implemented"))
	}

	items := make([]internal_tag_port.BatchUpsertItem, len(req.Msg.Items))
	for i, item := range req.Msg.Items {
		items[i] = internal_tag_port.BatchUpsertItem{
			ArticleID: item.ArticleId,
			FeedID:    item.FeedId,
			Tags:      tagItemsFromProto(item.Tags),
		}
	}

	total, err := h.batchUpsertArticleTags.BatchUpsertArticleTags(ctx, items)
	if err != nil {
		h.logger.Error("BatchUpsertArticleTags failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to batch upsert article tags"))
	}

	// Create tag set version + knowledge event for each article
	if h.createTagSetVersionUsecase != nil {
		for _, item := range req.Msg.Items {
			if len(item.Tags) == 0 {
				continue
			}
			if item.FeedId == "" {
				h.logger.Warn("skipping tag set version creation for article without feed_id", "article_id", item.ArticleId)
				continue
			}
			articleUUID, parseErr := uuid.Parse(item.ArticleId)
			if parseErr != nil {
				continue
			}
			batchTags := tagItemsFromProto(item.Tags)
			tagsJSON, _ := json.Marshal(batchTags)
			tsv := domain.TagSetVersion{
				ArticleID: articleUUID,
				Generator: "tag-generator",
				TagsJSON:  tagsJSON,
			}
			// Resolve UserID from article
			if h.getArticleByID != nil {
				article, artErr := h.getArticleByID.GetArticleByID(ctx, item.ArticleId)
				if artErr == nil && article != nil {
					userUUID, uErr := uuid.Parse(article.UserID)
					if uErr == nil {
						tsv.UserID = userUUID
					}
				}
			}
			if tsvErr := h.createTagSetVersionUsecase.Execute(ctx, tsv); tsvErr != nil {
				h.logger.Error("failed to create tag set version in batch", "error", tsvErr, "article_id", item.ArticleId)
			}
		}
	}

	return connect.NewResponse(&datahubv1.BatchUpsertArticleTagsResponse{
		Success:       true,
		TotalUpserted: total,
	}), nil
}
