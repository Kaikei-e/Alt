package knowledge_home

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"alt/connect/errorhandler"
	"alt/connect/v2/middleware"
	"alt/domain"
	knowledgehomev1 "alt/gen/proto/alt/knowledge_home/v1"
	"alt/utils/safeconv"
)

// GetKnowledgeHome returns the Knowledge Home feed.
func (h *Handler) GetKnowledgeHome(
	ctx context.Context,
	req *connect.Request[knowledgehomev1.GetKnowledgeHomeRequest],
) (resp *connect.Response[knowledgehomev1.GetKnowledgeHomeResponse], err error) {
	start := time.Now()
	defer func() {
		if h.metrics == nil {
			return
		}
		status := "ok"
		if err != nil {
			status = "error"
		}
		h.metrics.RequestsTotal.Add(ctx, 1, metric.WithAttributes(attribute.String("status", status)))
		if h.metrics.Snapshot != nil {
			h.metrics.Snapshot.RecordRequest()
		}
		if err != nil {
			return
		}
		h.metrics.RequestDurationSeconds.Record(ctx, time.Since(start).Seconds())
	}()

	user, err := middleware.GetUserContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, nil)
	}

	// Feature flag guard: deny access if Knowledge Home page is disabled for this user
	if h.featureFlagPort != nil && !h.featureFlagPort.IsEnabled(domain.FlagKnowledgeHomePage, user.UserID) {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("knowledge home is not enabled for this user"))
	}

	limit := int(req.Msg.Limit)
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var cursor string
	if req.Msg.Cursor != nil {
		cursor = *req.Msg.Cursor
	}

	date := time.Now()
	if req.Msg.Date != nil && *req.Msg.Date != "" {
		parsed, err := time.Parse("2006-01-02", *req.Msg.Date)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err))
		}
		date = parsed
	}

	var lensID *uuid.UUID
	if req.Msg.LensId != nil && *req.Msg.LensId != "" {
		parsedLensID, err := parseUUID(*req.Msg.LensId, "lens_id")
		if err != nil {
			return nil, err
		}
		lensID = &parsedLensID
	}

	result, err := h.getHomeUsecase.Execute(ctx, user.UserID, cursor, limit, date, lensID)
	if err != nil {
		return nil, errorhandler.HandleUpstreamError(ctx, h.logger, err, "GetKnowledgeHome")
	}

	// Convert items to proto
	protoItems := make([]*knowledgehomev1.KnowledgeHomeItem, 0, len(result.Items))
	for _, item := range result.Items {
		protoItems = append(protoItems, convertHomeItemToProto(item))
	}

	// Map digest from usecase (needToKnowCount is backend-authoritative, not page-scanned)
	digest := &knowledgehomev1.TodayDigest{
		Date:                  result.Digest.DigestDate.Format("2006-01-02"),
		NewArticles:           safeconv.Int32(result.Digest.NewArticles),
		SummarizedArticles:    safeconv.Int32(result.Digest.SummarizedArticles),
		UnsummarizedArticles:  safeconv.Int32(result.Digest.UnsummarizedArticles),
		TopTags:               result.Digest.TopTags,
		WeeklyRecapAvailable:  result.Digest.WeeklyRecapAvailable,
		EveningPulseAvailable: result.Digest.EveningPulseAvailable,
		NeedToKnowCount:       safeconv.Int32(result.Digest.NeedToKnowCount),
		DigestFreshness:       result.Digest.DigestFreshness,
	}
	if result.Digest.LastProjectedAt != nil {
		digest.LastProjectedAt = result.Digest.LastProjectedAt.Format(time.RFC3339)
	}

	// Build feature flag statuses for the response
	var featureFlags []*knowledgehomev1.FeatureFlagStatus
	if h.featureFlagPort != nil {
		flags := []string{
			domain.FlagKnowledgeHomePage,
			domain.FlagKnowledgeHomeTracking,
			domain.FlagKnowledgeHomeProjectionV2,
			domain.FlagLensV0,
			domain.FlagStreamUpdates,
			domain.FlagSupersedeUX,
		}
		for _, flag := range flags {
			featureFlags = append(featureFlags, &knowledgehomev1.FeatureFlagStatus{
				Name:    flag,
				Enabled: h.featureFlagPort.IsEnabled(flag, user.UserID),
			})
		}
	}

	// Determine 3-tier service quality
	serviceQuality := result.ServiceQuality
	if serviceQuality == "" {
		// Backward compatibility: derive from Degraded flag
		serviceQuality = "full"
		if result.Degraded {
			serviceQuality = "degraded"
		}
	}

	respPayload := &knowledgehomev1.GetKnowledgeHomeResponse{
		TodayDigest:    digest,
		Items:          protoItems,
		NextCursor:     result.NextCursor,
		HasMore:        result.HasMore,
		DegradedMode:   result.Degraded,
		GeneratedAt:    result.GeneratedAt.Format(time.RFC3339),
		FeatureFlags:   featureFlags,
		ServiceQuality: &serviceQuality,
	}

	if h.metrics != nil {
		if serviceQuality == "degraded" || serviceQuality == "fallback" {
			h.metrics.DegradedResponsesTotal.Add(ctx, 1)
			if h.metrics.Snapshot != nil {
				h.metrics.Snapshot.RecordDegradedResponse()
			}
		}
		if len(result.Items) == 0 {
			h.metrics.EmptyResponsesTotal.Add(ctx, 1)
			if h.metrics.Snapshot != nil {
				h.metrics.Snapshot.RecordEmptyResponse()
			}
		}
	}

	// ADR-000913 §D-9: recall candidates are now part of the canonical
	// GetKnowledgeHome payload. The legacy FlagRecallRail gate was retired
	// with PR 13; the rail surfaces unconditionally when the projector has
	// candidates for the user.
	if h.recallRailUsecase != nil {
		candidates, railErr := h.recallRailUsecase.Execute(ctx, user.UserID, 5)
		if railErr == nil {
			for _, c := range candidates {
				respPayload.RecallCandidates = append(respPayload.RecallCandidates, convertRecallCandidateToProto(c))
			}
		}
	}

	return connect.NewResponse(respPayload), nil
}
