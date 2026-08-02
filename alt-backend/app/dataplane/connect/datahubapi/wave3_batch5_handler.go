package datahubapi

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
	"alt/utils/safeconv"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// WithWave3Batch5Capabilities wires the versioned artifacts and the dashboard
// statistics ADR-000954 Wave 3 batch 5 moved out of alt-backend (capability
// catalog §2.K / §2.M).
//
// Nil panics, as in every batch before it, and the summary-version port is the
// one where a silent Unimplemented would do the most damage. Every summary
// alt-backend writes is accompanied by a SummaryVersionCreated event appended
// to knowledge-sovereign by the caller; if the version write behind it
// answered Unimplemented, the events would keep flowing and sovereign would
// accumulate references to versions that were never persisted. Nothing would
// look broken until somebody replayed the log (CLAUDE.md rule 8, ADR-000928).
func WithWave3Batch5Capabilities(
	summaryVersion datahub_capability_port.SummaryVersionPort,
	tagSetVersion datahub_capability_port.TagSetVersionPort,
	stats datahub_capability_port.StatsPort,
) HandlerOption {
	switch {
	case summaryVersion == nil:
		panic("datahubapi: SummaryVersionPort is required — summary_versions has no other route, and the knowledge events describing those versions are appended regardless")
	case tagSetVersion == nil:
		panic("datahubapi: TagSetVersionPort is required — tag_set_versions has no other route, and TagSetVersionCreated is appended regardless")
	case stats == nil:
		panic("datahubapi: StatsPort is required — every dashboard count and the trend chart read through it")
	}

	return func(h *Handler) {
		h.summaryVersion = summaryVersion
		h.tagSetVersion = tagSetVersion
		h.stats = stats
	}
}

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

// MarkSummaryVersionSuperseded holds the batch's heaviest invariant, and holds
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

// ---------------------------------------------------------------------------
// §2.M Statistics / dashboard
// ---------------------------------------------------------------------------

// GetFeedAmount is the one count with no tenant, because it is the
// deployment's size rather than a user's.
func (h *Handler) GetFeedAmount(ctx context.Context, _ *connect.Request[datahubv1.GetFeedAmountRequest]) (*connect.Response[datahubv1.GetFeedAmountResponse], error) {
	count, err := h.stats.FeedAmount(ctx)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetFeedAmount failed", "error", err)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get feed amount"))
	}
	return connect.NewResponse(&datahubv1.GetFeedAmountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetTotalArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetTotalArticlesCountRequest]) (*connect.Response[datahubv1.GetTotalArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	count, err := h.stats.TotalArticles(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTotalArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get total articles count"))
	}
	return connect.NewResponse(&datahubv1.GetTotalArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetSummarizedArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetSummarizedArticlesCountRequest]) (*connect.Response[datahubv1.GetSummarizedArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	count, err := h.stats.SummarizedArticles(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetSummarizedArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get summarized articles count"))
	}
	return connect.NewResponse(&datahubv1.GetSummarizedArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetUnsummarizedArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetUnsummarizedArticlesCountRequest]) (*connect.Response[datahubv1.GetUnsummarizedArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	count, err := h.stats.UnsummarizedArticles(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetUnsummarizedArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get unsummarized articles count"))
	}
	return connect.NewResponse(&datahubv1.GetUnsummarizedArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

// GetTodayUnreadArticlesCount requires `since` rather than defaulting it.
//
// A server-side midnight would answer a different question convincingly: the
// provider does not know the reader's timezone, so "today" is only meaningful
// where the request came from.
func (h *Handler) GetTodayUnreadArticlesCount(ctx context.Context, req *connect.Request[datahubv1.GetTodayUnreadArticlesCountRequest]) (*connect.Response[datahubv1.GetTodayUnreadArticlesCountResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetSince() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("since is required"))
	}

	count, err := h.stats.TodayUnread(ctx, userID, req.Msg.GetSince().AsTime())
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTodayUnreadArticlesCount failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get today unread articles count"))
	}
	return connect.NewResponse(&datahubv1.GetTodayUnreadArticlesCountResponse{Count: safeconv.Int32(count)}), nil
}

func (h *Handler) GetTrendStats(ctx context.Context, req *connect.Request[datahubv1.GetTrendStatsRequest]) (*connect.Response[datahubv1.GetTrendStatsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	window, err := trendWindowFromProto(req.Msg.GetWindow())
	if err != nil {
		return nil, err
	}

	series, err := h.stats.TrendStats(ctx, userID, window)
	if err != nil {
		h.logger.ErrorContext(ctx, "GetTrendStats failed", "error", err, "user_id", userID, "window", window)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to get trend stats"))
	}

	points := make([]*datahubv1.TrendDataPoint, 0, len(series.Points))
	for _, p := range series.Points {
		points = append(points, &datahubv1.TrendDataPoint{
			Bucket:       timestamppb.New(p.Timestamp),
			Articles:     safeconv.Int32(p.Articles),
			Summarized:   safeconv.Int32(p.Summarized),
			FeedActivity: safeconv.Int32(p.FeedActivity),
		})
	}

	return connect.NewResponse(&datahubv1.GetTrendStatsResponse{
		Points:      points,
		Granularity: trendGranularityToProto(series.Granularity),
	}), nil
}

func (h *Handler) ListUserFeedIDs(ctx context.Context, req *connect.Request[datahubv1.ListUserFeedIDsRequest]) (*connect.Response[datahubv1.ListUserFeedIDsResponse], error) {
	userID, err := requiredUserID(req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}

	ids, err := h.stats.UserFeedIDs(ctx, userID)
	if err != nil {
		h.logger.ErrorContext(ctx, "ListUserFeedIDs failed", "error", err, "user_id", userID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list user feed ids"))
	}
	return connect.NewResponse(&datahubv1.ListUserFeedIDsResponse{FeedIds: uuidStrings(ids)}), nil
}

// ---------------------------------------------------------------------------
// Conversions
// ---------------------------------------------------------------------------

// supersedeIDs refuses an absent or malformed id rather than falling back to
// uuid.Nil, which is a valid-looking key that matches nothing: a supersede
// keyed on it would take the advisory lock for an article that does not exist
// and report success having changed nothing. requiredUUID is batch 2's, for
// the same reason.
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

func summaryVersionFromProto(msg *datahubv1.SummaryVersion) (domain.SummaryVersion, error) {
	if msg == nil {
		return domain.SummaryVersion{}, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}

	versionID, err := requiredUUID(msg.GetSummaryVersionId(), "version.summary_version_id")
	if err != nil {
		return domain.SummaryVersion{}, err
	}
	articleID, err := requiredUUID(msg.GetArticleId(), "version.article_id")
	if err != nil {
		return domain.SummaryVersion{}, err
	}
	userID, err := requiredUUID(msg.GetUserId(), "version.user_id")
	if err != nil {
		return domain.SummaryVersion{}, err
	}
	if msg.GetSummaryText() == "" {
		return domain.SummaryVersion{}, connect.NewError(connect.CodeInvalidArgument, errors.New("version.summary_text is required"))
	}

	sv := domain.SummaryVersion{
		SummaryVersionID: versionID,
		ArticleID:        articleID,
		UserID:           userID,
		GeneratedAt:      msg.GetGeneratedAt().AsTime(),
		Model:            msg.GetModel(),
		PromptVersion:    msg.GetPromptVersion(),
		InputHash:        msg.GetInputHash(),
		SummaryText:      msg.GetSummaryText(),
	}
	if msg.QualityScore != nil {
		score := msg.GetQualityScore()
		sv.QualityScore = &score
	}
	if msg.SupersededBy != nil {
		supersededBy, parseErr := requiredUUID(msg.GetSupersededBy(), "version.superseded_by")
		if parseErr != nil {
			return domain.SummaryVersion{}, parseErr
		}
		sv.SupersededBy = &supersededBy
	}
	return sv, nil
}

func summaryVersionToProto(sv domain.SummaryVersion) *datahubv1.SummaryVersion {
	out := &datahubv1.SummaryVersion{
		SummaryVersionId: sv.SummaryVersionID.String(),
		ArticleId:        sv.ArticleID.String(),
		UserId:           sv.UserID.String(),
		Model:            sv.Model,
		PromptVersion:    sv.PromptVersion,
		InputHash:        sv.InputHash,
		SummaryText:      sv.SummaryText,
	}
	// A zero generated_at stays absent rather than becoming 1970: consumers
	// order versions by it, and an epoch timestamp sorts first.
	if !sv.GeneratedAt.IsZero() {
		out.GeneratedAt = timestamppb.New(sv.GeneratedAt)
	}
	if sv.QualityScore != nil {
		out.QualityScore = sv.QualityScore
	}
	if sv.SupersededBy != nil {
		s := sv.SupersededBy.String()
		out.SupersededBy = &s
	}
	return out
}

func tagSetVersionFromProto(msg *datahubv1.TagSetVersion) (domain.TagSetVersion, error) {
	if msg == nil {
		return domain.TagSetVersion{}, connect.NewError(connect.CodeInvalidArgument, errors.New("version is required"))
	}

	versionID, err := requiredUUID(msg.GetTagSetVersionId(), "version.tag_set_version_id")
	if err != nil {
		return domain.TagSetVersion{}, err
	}
	articleID, err := requiredUUID(msg.GetArticleId(), "version.article_id")
	if err != nil {
		return domain.TagSetVersion{}, err
	}
	userID, err := requiredUUID(msg.GetUserId(), "version.user_id")
	if err != nil {
		return domain.TagSetVersion{}, err
	}

	tsv := domain.TagSetVersion{
		TagSetVersionID: versionID,
		ArticleID:       articleID,
		UserID:          userID,
		GeneratedAt:     msg.GetGeneratedAt().AsTime(),
		Generator:       msg.GetGenerator(),
		InputHash:       msg.GetInputHash(),
		// Stored as the generator wrote it. Not decoded and re-encoded here:
		// the column is jsonb, and a round trip would reorder keys, so a later
		// read would return bytes the generator never produced.
		TagsJSON: msg.GetTagsJson(),
	}
	if msg.SupersededBy != nil {
		supersededBy, parseErr := requiredUUID(msg.GetSupersededBy(), "version.superseded_by")
		if parseErr != nil {
			return domain.TagSetVersion{}, parseErr
		}
		tsv.SupersededBy = &supersededBy
	}
	return tsv, nil
}

func tagSetVersionToProto(tsv domain.TagSetVersion) *datahubv1.TagSetVersion {
	out := &datahubv1.TagSetVersion{
		TagSetVersionId: tsv.TagSetVersionID.String(),
		ArticleId:       tsv.ArticleID.String(),
		UserId:          tsv.UserID.String(),
		Generator:       tsv.Generator,
		InputHash:       tsv.InputHash,
		TagsJson:        tsv.TagsJSON,
	}
	if !tsv.GeneratedAt.IsZero() {
		out.GeneratedAt = timestamppb.New(tsv.GeneratedAt)
	}
	if tsv.SupersededBy != nil {
		s := tsv.SupersededBy.String()
		out.SupersededBy = &s
	}
	return out
}

// trendWindowFromProto rejects the unspecified value instead of picking a
// default window.
//
// Defaulting would answer a question nobody asked and charge for it: the four
// windows differ by two orders of magnitude in rows scanned, and a caller that
// forgot the field would silently get whichever one this function preferred.
func trendWindowFromProto(w datahubv1.TrendWindow) (string, error) {
	switch w {
	case datahubv1.TrendWindow_TREND_WINDOW_4H:
		return "4h", nil
	case datahubv1.TrendWindow_TREND_WINDOW_24H:
		return "24h", nil
	case datahubv1.TrendWindow_TREND_WINDOW_3D:
		return "3d", nil
	case datahubv1.TrendWindow_TREND_WINDOW_7D:
		return "7d", nil
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("window is required"))
	}
}

func trendGranularityToProto(granularity string) datahubv1.TrendGranularity {
	switch granularity {
	case "hourly":
		return datahubv1.TrendGranularity_TREND_GRANULARITY_HOURLY
	case "daily":
		return datahubv1.TrendGranularity_TREND_GRANULARITY_DAILY
	default:
		return datahubv1.TrendGranularity_TREND_GRANULARITY_UNSPECIFIED
	}
}
