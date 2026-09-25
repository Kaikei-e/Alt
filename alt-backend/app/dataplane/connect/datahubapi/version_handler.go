package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// §2.K Versioned artifacts — summary_versions
// ---------------------------------------------------------------------------

func (h *Handler) CreateSummaryVersion(ctx context.Context, req *connect.Request[datahubv1.CreateSummaryVersionRequest]) (*connect.Response[datahubv1.CreateSummaryVersionResponse], error) {
	sv, err := summaryVersionFromProto(req.Msg.GetVersion())
	if err != nil {
		return nil, err
	}

	if err := h.summaryVersion.CreateSummaryVersion(ctx, sv); err != nil {
		h.logger.ErrorContext(ctx, "CreateSummaryVersion failed", "error", err,
			"summary_version_id", sv.SummaryVersionID, "article_id", sv.ArticleID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create summary version"))
	}
	return connect.NewResponse(&datahubv1.CreateSummaryVersionResponse{}), nil
}

// MarkSummaryVersionSuperseded holds the version capability's heaviest invariant, and holds
// all of it inside this call.
//
// The port method behind it opens a transaction, takes
// `pg_advisory_xact_lock(hashtext(article_id))`, reads the version that is
// current, marks it superseded and commits. Splitting that across two
// procedures would look tidier and would be broken: an advisory *xact* lock
// ends at commit, so the second caller would acquire it the instant the first
// released it — between the two halves of the first caller's work rather than
// after it.
func (h *Handler) MarkSummaryVersionSuperseded(ctx context.Context, req *connect.Request[datahubv1.MarkSummaryVersionSupersededRequest]) (*connect.Response[datahubv1.MarkSummaryVersionSupersededResponse], error) {
	articleID, newVersionID, err := supersedeIDs(req.Msg.GetArticleId(), req.Msg.GetNewVersionId())
	if err != nil {
		return nil, err
	}

	prev, err := h.summaryVersion.MarkSummaryVersionSuperseded(ctx, articleID, newVersionID)
	if err != nil {
		h.logger.ErrorContext(ctx, "MarkSummaryVersionSuperseded failed", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to mark summary versions superseded"))
	}

	resp := &datahubv1.MarkSummaryVersionSupersededResponse{}
	// Absent, not zero. The caller emits SummarySuperseded on the presence of
	// this field; an empty message would announce the replacement of a summary
	// that never existed.
	if prev != nil {
		resp.PreviousVersion = summaryVersionToProto(*prev)
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) GetSummaryVersionByID(ctx context.Context, req *connect.Request[datahubv1.GetSummaryVersionByIDRequest]) (*connect.Response[datahubv1.GetSummaryVersionByIDResponse], error) {
	versionID, err := requiredUUID(req.Msg.GetSummaryVersionId(), "summary_version_id")
	if err != nil {
		return nil, err
	}

	sv, err := h.summaryVersion.GetSummaryVersionByID(ctx, versionID)
	if err != nil {
		return nil, h.versionReadError(ctx, "GetSummaryVersionByID", err, "summary_version_id", versionID.String())
	}
	return connect.NewResponse(&datahubv1.GetSummaryVersionByIDResponse{Version: summaryVersionToProto(sv)}), nil
}

func (h *Handler) GetLatestSummaryVersion(ctx context.Context, req *connect.Request[datahubv1.GetLatestSummaryVersionRequest]) (*connect.Response[datahubv1.GetLatestSummaryVersionResponse], error) {
	articleID, err := requiredUUID(req.Msg.GetArticleId(), "article_id")
	if err != nil {
		return nil, err
	}

	sv, err := h.summaryVersion.GetLatestSummaryVersion(ctx, articleID)
	if err != nil {
		return nil, h.versionReadError(ctx, "GetLatestSummaryVersion", err, "article_id", articleID.String())
	}
	return connect.NewResponse(&datahubv1.GetLatestSummaryVersionResponse{Version: summaryVersionToProto(sv)}), nil
}

// ---------------------------------------------------------------------------
// §2.K Versioned artifacts — tag_set_versions
// ---------------------------------------------------------------------------

func (h *Handler) CreateTagSetVersion(ctx context.Context, req *connect.Request[datahubv1.CreateTagSetVersionRequest]) (*connect.Response[datahubv1.CreateTagSetVersionResponse], error) {
	tsv, err := tagSetVersionFromProto(req.Msg.GetVersion())
	if err != nil {
		return nil, err
	}

	if err := h.tagSetVersion.CreateTagSetVersion(ctx, tsv); err != nil {
		h.logger.ErrorContext(ctx, "CreateTagSetVersion failed", "error", err,
			"tag_set_version_id", tsv.TagSetVersionID, "article_id", tsv.ArticleID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to create tag set version"))
	}
	return connect.NewResponse(&datahubv1.CreateTagSetVersionResponse{}), nil
}

// MarkTagSetVersionSuperseded is the tag-set twin, and the note on
// MarkSummaryVersionSuperseded applies unchanged. Tag sets are regenerated more
// often than summaries, so the concurrent case this lock covers is the more
// likely of the two.
func (h *Handler) MarkTagSetVersionSuperseded(ctx context.Context, req *connect.Request[datahubv1.MarkTagSetVersionSupersededRequest]) (*connect.Response[datahubv1.MarkTagSetVersionSupersededResponse], error) {
	articleID, newVersionID, err := supersedeIDs(req.Msg.GetArticleId(), req.Msg.GetNewVersionId())
	if err != nil {
		return nil, err
	}

	prev, err := h.tagSetVersion.MarkTagSetVersionSuperseded(ctx, articleID, newVersionID)
	if err != nil {
		h.logger.ErrorContext(ctx, "MarkTagSetVersionSuperseded failed", "error", err, "article_id", articleID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to mark tag set versions superseded"))
	}

	resp := &datahubv1.MarkTagSetVersionSupersededResponse{}
	if prev != nil {
		resp.PreviousVersion = tagSetVersionToProto(*prev)
	}
	return connect.NewResponse(resp), nil
}

func (h *Handler) GetTagSetVersionByID(ctx context.Context, req *connect.Request[datahubv1.GetTagSetVersionByIDRequest]) (*connect.Response[datahubv1.GetTagSetVersionByIDResponse], error) {
	versionID, err := requiredUUID(req.Msg.GetTagSetVersionId(), "tag_set_version_id")
	if err != nil {
		return nil, err
	}

	tsv, err := h.tagSetVersion.GetTagSetVersionByID(ctx, versionID)
	if err != nil {
		return nil, h.versionReadError(ctx, "GetTagSetVersionByID", err, "tag_set_version_id", versionID.String())
	}
	return connect.NewResponse(&datahubv1.GetTagSetVersionByIDResponse{Version: tagSetVersionToProto(tsv)}), nil
}

// versionReadError maps "no such version" to NotFound and everything else to
// Internal.
//
// The driver reports absence as an error string rather than a sentinel, so the
// match is on the message. That is worth naming as the compromise it is: a
// consumer branching on NotFound would get Internal instead if that wording
// changed. It is still better than the alternative — reporting every absence as
// Internal, which would make a projector treat a version it will never find as
// a transient fault and retry it forever.
func (h *Handler) versionReadError(ctx context.Context, procedure string, err error, key, value string) error {
	if strings.Contains(err.Error(), "no rows in result set") ||
		strings.Contains(err.Error(), "no summary version found") ||
		strings.Contains(err.Error(), "no tag set version found") {
		return connect.NewError(connect.CodeNotFound, errors.New("version not found"))
	}
	h.logger.ErrorContext(ctx, procedure+" failed", "error", err, key, value)
	return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to %s", procedure))
}

// supersedeIDs refuses an absent or malformed id rather than falling back to
// uuid.Nil, which is a valid-looking key that matches nothing: a supersede
// keyed on it would take the advisory lock for an article that does not exist
// and report success having changed nothing. requiredUUID was introduced for
// the article capability, for the same reason.
func supersedeIDs(rawArticleID, rawNewVersionID string) (uuid.UUID, uuid.UUID, error) {
	articleID, err := requiredUUID(rawArticleID, "article_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	newVersionID, err := requiredUUID(rawNewVersionID, "new_version_id")
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	return articleID, newVersionID, nil
}
