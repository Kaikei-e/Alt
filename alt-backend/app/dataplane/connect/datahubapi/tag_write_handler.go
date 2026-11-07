package datahubapi

import (
	"context"
	"encoding/json"
	"errors"

	"alt/dataplane/port/internal_tag_port"
	"alt/dataplane/usecase/create_tag_set_version_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	if len(tags) > 0 {
		if h.createTagSetVersionUsecase == nil {
			panic("datahubapi: CreateTagSetVersionUsecase is nil — " +
				"wire it via WithKnowledgeVersionUsecases (see .claude/rules/di-wiring.md)")
		}
		articleUUID, parseErr := uuid.Parse(req.Msg.ArticleId)
		if parseErr == nil {
			if err := h.recordTagSetVersion(ctx, req.Msg.ArticleId, articleUUID, tags); err != nil {
				return nil, err
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
	if h.createTagSetVersionUsecase == nil {
		panic("datahubapi: CreateTagSetVersionUsecase is nil — " +
			"wire it via WithKnowledgeVersionUsecases (see .claude/rules/di-wiring.md)")
	}
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
		if err := h.recordTagSetVersion(ctx, item.ArticleId, articleUUID, batchTags); err != nil {
			return nil, err
		}
	}

	return connect.NewResponse(&datahubv1.BatchUpsertArticleTagsResponse{
		Success:       true,
		TotalUpserted: total,
	}), nil
}

func (h *Handler) recordTagSetVersion(ctx context.Context, articleID string, articleUUID uuid.UUID, tags []internal_tag_port.TagItem) error {
	if h.createTagSetVersionUsecase == nil {
		panic("datahubapi: CreateTagSetVersionUsecase is nil — " +
			"wire it via WithKnowledgeVersionUsecases (see .claude/rules/di-wiring.md)")
	}
	if h.getArticleByID == nil {
		panic("datahubapi: internal_article_port.GetArticleByIDPort is nil — " +
			"tag set versioning requires GetArticleByIDPort to resolve the article owner " +
			"(see .claude/rules/di-wiring.md)")
	}

	article, artErr := h.getArticleByID.GetArticleByID(ctx, articleID)
	if artErr != nil {
		if errors.Is(artErr, pgx.ErrNoRows) || errors.Is(artErr, domain.ErrArticleNotFound) {
			h.logger.Warn("skipping tag set version for missing or deleted article",
				"article_id", articleID, "error", artErr)
			return nil
		}
		h.logger.Error("failed to resolve article owner for tag set version; refusing to acknowledge the write",
			"article_id", articleID, "error", artErr)
		return connect.NewError(connect.CodeUnavailable,
			errors.New("tags written but article owner could not be resolved; retry"))
	}
	if article == nil {
		h.logger.Warn("skipping tag set version for missing or deleted article",
			"article_id", articleID)
		return nil
	}

	userUUID, uErr := uuid.Parse(article.UserID)
	if uErr != nil || userUUID == uuid.Nil {
		h.logger.Error("could not resolve user for tag set version",
			"article_id", articleID, "error", uErr)
		return nil
	}

	tagsJSON, _ := json.Marshal(tags)
	tsv := domain.TagSetVersion{
		ArticleID: articleUUID,
		UserID:    userUUID,
		Generator: "tag-generator",
		TagsJSON:  tagsJSON,
	}
	if tsvErr := h.createTagSetVersionUsecase.Execute(ctx, tsv); tsvErr != nil {
		if isPermanentTagSetVersionError(tsvErr) {
			h.logger.Error("invalid tag set version; refusing to acknowledge the write",
				"error", tsvErr, "article_id", articleID)
			return connect.NewError(connect.CodeInvalidArgument, tsvErr)
		}
		h.logger.Error("failed to create tag set version; refusing to acknowledge the write",
			"error", tsvErr, "article_id", articleID)
		return connect.NewError(connect.CodeUnavailable,
			errors.New("tags written but their version could not be recorded; retry"))
	}
	return nil
}

func isPermanentTagSetVersionError(err error) bool {
	return errors.Is(err, create_tag_set_version_usecase.ErrMissingArticleID) ||
		errors.Is(err, create_tag_set_version_usecase.ErrMissingUserID) ||
		errors.Is(err, create_tag_set_version_usecase.ErrEmptyTagsJSON)
}
