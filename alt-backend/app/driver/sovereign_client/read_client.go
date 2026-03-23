package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// === Projection reads ===

func (c *Client) GetKnowledgeHomeItems(ctx context.Context, userID uuid.UUID, cursor string, limit int, filter *domain.KnowledgeHomeLensFilter) ([]domain.KnowledgeHomeItem, string, bool, error) {
	if !c.enabled {
		return nil, "", false, nil
	}

	req := &sovereignv1.GetKnowledgeHomeItemsRequest{
		UserId: userID.String(),
		Cursor: cursor,
		Limit:  int32(limit),
	}
	if filter != nil {
		sourceIDs := make([]string, len(filter.SourceIDs))
		for i, id := range filter.SourceIDs {
			sourceIDs[i] = id.String()
		}
		req.Filter = &sovereignv1.LensFilter{
			QueryText:  filter.QueryText,
			TagIds:     filter.TagNames,
			SourceIds:  sourceIDs,
			TimeWindow: filter.TimeWindow,
		}
	}

	resp, err := c.client.GetKnowledgeHomeItems(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, "", false, fmt.Errorf("sovereign GetKnowledgeHomeItems: %w", err)
	}

	items := make([]domain.KnowledgeHomeItem, len(resp.Msg.Items))
	for i, pb := range resp.Msg.Items {
		items[i] = protoToHomeItem(pb)
	}
	return items, resp.Msg.NextCursor, resp.Msg.HasMore, nil
}

func (c *Client) GetTodayDigest(ctx context.Context, userID uuid.UUID, date time.Time) (domain.TodayDigest, error) {
	if !c.enabled {
		return domain.TodayDigest{UserID: userID, DigestDate: date}, nil
	}

	resp, err := c.client.GetTodayDigest(ctx, connect.NewRequest(&sovereignv1.GetTodayDigestRequest{
		UserId: userID.String(),
		Date:   date.Format("2006-01-02"),
	}))
	if err != nil {
		return domain.TodayDigest{}, fmt.Errorf("sovereign GetTodayDigest: %w", err)
	}

	d := resp.Msg.Digest
	if d == nil {
		return domain.TodayDigest{UserID: userID, DigestDate: date, UpdatedAt: time.Now()}, nil
	}
	digestDate, _ := time.Parse("2006-01-02", d.DigestDate)
	result := domain.TodayDigest{
		UserID:                parseUUID(d.UserId),
		DigestDate:            digestDate,
		NewArticles:           int(d.NewArticles),
		SummarizedArticles:    int(d.SummarizedArticles),
		UnsummarizedArticles:  int(d.UnsummarizedArticles),
		TopTags:               d.TopTags,
		WeeklyRecapAvailable:  d.WeeklyRecapAvailable,
		EveningPulseAvailable: d.EveningPulseAvailable,
		NeedToKnowCount:       int(d.NeedToKnowCount),
		DigestFreshness:       d.DigestFreshness,
	}
	if d.UpdatedAt != nil {
		result.UpdatedAt = d.UpdatedAt.AsTime()
	}
	if d.LastProjectedAt != nil {
		t := d.LastProjectedAt.AsTime()
		result.LastProjectedAt = &t
	}
	return result, nil
}

func (c *Client) GetRecallCandidates(ctx context.Context, userID uuid.UUID, limit int) ([]domain.RecallCandidate, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetRecallCandidates(ctx, connect.NewRequest(&sovereignv1.GetRecallCandidatesRequest{
		UserId: userID.String(), Limit: int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetRecallCandidates: %w", err)
	}
	candidates := make([]domain.RecallCandidate, len(resp.Msg.Candidates))
	for i, pb := range resp.Msg.Candidates {
		cand := domain.RecallCandidate{
			UserID:            parseUUID(pb.UserId),
			ItemKey:           pb.ItemKey,
			RecallScore:       pb.RecallScore,
			ProjectionVersion: int(pb.ProjectionVersion),
		}
		if pb.UpdatedAt != nil {
			cand.UpdatedAt = pb.UpdatedAt.AsTime()
		}
		if pb.NextSuggestAt != nil {
			t := pb.NextSuggestAt.AsTime()
			cand.NextSuggestAt = &t
		}
		if pb.FirstEligibleAt != nil {
			t := pb.FirstEligibleAt.AsTime()
			cand.FirstEligibleAt = &t
		}
		for _, r := range pb.Reasons {
			cand.Reasons = append(cand.Reasons, domain.RecallReason{
				Type: r.Type, Description: r.Description, SourceItemKey: r.SourceItemKey,
			})
		}
		if pb.Item != nil {
			item := protoToHomeItem(pb.Item)
			cand.Item = &item
		}
		candidates[i] = cand
	}
	return candidates, nil
}

func (c *Client) ListDistinctUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListDistinctUserIDs(ctx, connect.NewRequest(&sovereignv1.ListDistinctUserIDsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListDistinctUserIDs: %w", err)
	}
	ids := make([]uuid.UUID, len(resp.Msg.UserIds))
	for i, s := range resp.Msg.UserIds {
		ids[i] = parseUUID(s)
	}
	return ids, nil
}

func (c *Client) CountNeedToKnowItems(ctx context.Context, userID uuid.UUID, date time.Time) (int, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.CountNeedToKnowItems(ctx, connect.NewRequest(&sovereignv1.CountNeedToKnowItemsRequest{
		UserId: userID.String(), Date: date.Format("2006-01-02"),
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign CountNeedToKnowItems: %w", err)
	}
	return int(resp.Msg.Count), nil
}

func (c *Client) GetProjectionFreshness(ctx context.Context, projectorName string) (*time.Time, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetProjectionFreshness(ctx, connect.NewRequest(&sovereignv1.GetProjectionFreshnessRequest{
		ProjectorName: projectorName,
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetProjectionFreshness: %w", err)
	}
	if !resp.Msg.Found || resp.Msg.UpdatedAt == nil {
		return nil, nil
	}
	t := resp.Msg.UpdatedAt.AsTime()
	return &t, nil
}

// === Events ===

func (c *Client) ListKnowledgeEventsSince(ctx context.Context, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListKnowledgeEvents(ctx, connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
		AfterSeq: afterSeq, Limit: int32(limit),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListKnowledgeEventsSince: %w", err)
	}
	return protoToEvents(resp.Msg.Events), nil
}

func (c *Client) ListKnowledgeEventsSinceForUser(ctx context.Context, userID uuid.UUID, afterSeq int64, limit int) ([]domain.KnowledgeEvent, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListKnowledgeEvents(ctx, connect.NewRequest(&sovereignv1.ListKnowledgeEventsRequest{
		AfterSeq: afterSeq, Limit: int32(limit), UserId: userID.String(),
	}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListKnowledgeEventsSinceForUser: %w", err)
	}
	return protoToEvents(resp.Msg.Events), nil
}

func (c *Client) GetLatestKnowledgeEventSeqForUser(ctx context.Context, userID uuid.UUID) (int64, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetLatestEventSeq(ctx, connect.NewRequest(&sovereignv1.GetLatestEventSeqRequest{
		UserId: userID.String(),
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetLatestEventSeqForUser: %w", err)
	}
	return resp.Msg.EventSeq, nil
}

func (c *Client) AppendKnowledgeEvent(ctx context.Context, event domain.KnowledgeEvent) error {
	if !c.enabled {
		return nil
	}
	pbEvent := domainEventToProto(event)
	_, err := c.client.AppendKnowledgeEvent(ctx, connect.NewRequest(&sovereignv1.AppendKnowledgeEventRequest{
		Event: pbEvent,
	}))
	if err != nil {
		return fmt.Errorf("sovereign AppendKnowledgeEvent: %w", err)
	}
	return nil
}

// === Projection infra ===

func (c *Client) GetActiveProjectionVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.GetActiveProjectionVersion(ctx, connect.NewRequest(&sovereignv1.GetActiveProjectionVersionRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign GetActiveProjectionVersion: %w", err)
	}
	if resp.Msg.Version == nil {
		return nil, nil
	}
	return protoToProjectionVersion(resp.Msg.Version), nil
}

func (c *Client) ListProjectionVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error) {
	if !c.enabled {
		return nil, nil
	}
	resp, err := c.client.ListProjectionVersions(ctx, connect.NewRequest(&sovereignv1.ListProjectionVersionsRequest{}))
	if err != nil {
		return nil, fmt.Errorf("sovereign ListProjectionVersions: %w", err)
	}
	versions := make([]domain.KnowledgeProjectionVersion, len(resp.Msg.Versions))
	for i, v := range resp.Msg.Versions {
		versions[i] = *protoToProjectionVersion(v)
	}
	return versions, nil
}

func (c *Client) CreateProjectionVersion(ctx context.Context, v domain.KnowledgeProjectionVersion) error {
	if !c.enabled {
		return nil
	}
	_, err := c.client.CreateProjectionVersion(ctx, connect.NewRequest(&sovereignv1.CreateProjectionVersionRequest{
		Version: domainToProtoVersion(v),
	}))
	if err != nil {
		return fmt.Errorf("sovereign CreateProjectionVersion: %w", err)
	}
	return nil
}

func (c *Client) ActivateProjectionVersion(ctx context.Context, version int) error {
	if !c.enabled {
		return nil
	}
	_, err := c.client.ActivateProjectionVersion(ctx, connect.NewRequest(&sovereignv1.ActivateProjectionVersionRequest{
		Version: int32(version),
	}))
	if err != nil {
		return fmt.Errorf("sovereign ActivateProjectionVersion: %w", err)
	}
	return nil
}

func (c *Client) GetProjectionCheckpoint(ctx context.Context, projectorName string) (int64, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetProjectionCheckpoint(ctx, connect.NewRequest(&sovereignv1.GetProjectionCheckpointRequest{
		ProjectorName: projectorName,
	}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetProjectionCheckpoint: %w", err)
	}
	return resp.Msg.LastEventSeq, nil
}

func (c *Client) UpdateProjectionCheckpoint(ctx context.Context, projectorName string, lastSeq int64) error {
	if !c.enabled {
		return nil
	}
	_, err := c.client.UpdateProjectionCheckpoint(ctx, connect.NewRequest(&sovereignv1.UpdateProjectionCheckpointRequest{
		ProjectorName: projectorName, LastEventSeq: lastSeq,
	}))
	if err != nil {
		return fmt.Errorf("sovereign UpdateProjectionCheckpoint: %w", err)
	}
	return nil
}

func (c *Client) GetProjectionLag(ctx context.Context) (time.Duration, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetProjectionLag(ctx, connect.NewRequest(&sovereignv1.GetProjectionLagRequest{}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetProjectionLag: %w", err)
	}
	if resp.Msg.LagSeconds < 0 {
		return time.Duration(-1), nil
	}
	return time.Duration(resp.Msg.LagSeconds * float64(time.Second)), nil
}

func (c *Client) GetProjectionAge(ctx context.Context) (time.Duration, error) {
	if !c.enabled {
		return 0, nil
	}
	resp, err := c.client.GetProjectionLag(ctx, connect.NewRequest(&sovereignv1.GetProjectionLagRequest{}))
	if err != nil {
		return 0, fmt.Errorf("sovereign GetProjectionAge: %w", err)
	}
	if resp.Msg.AgeSeconds < 0 {
		return time.Duration(-1), nil
	}
	return time.Duration(resp.Msg.AgeSeconds * float64(time.Second)), nil
}

// === Port interface aliases ===
// These methods alias the longer names to satisfy the shorter port interfaces.

func (c *Client) GetActiveVersion(ctx context.Context) (*domain.KnowledgeProjectionVersion, error) {
	return c.GetActiveProjectionVersion(ctx)
}

func (c *Client) ListVersions(ctx context.Context) ([]domain.KnowledgeProjectionVersion, error) {
	return c.ListProjectionVersions(ctx)
}

func (c *Client) CreateVersion(ctx context.Context, v domain.KnowledgeProjectionVersion) error {
	return c.CreateProjectionVersion(ctx, v)
}

func (c *Client) ActivateVersion(ctx context.Context, version int) error {
	return c.ActivateProjectionVersion(ctx, version)
}

// === helpers ===

func parseUUID(s string) uuid.UUID {
	id, _ := uuid.Parse(s)
	return id
}

func parseUUIDPtr(s string) *uuid.UUID {
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}

func protoToHomeItem(pb *sovereignv1.KnowledgeHomeItem) domain.KnowledgeHomeItem {
	item := domain.KnowledgeHomeItem{
		UserID:            parseUUID(pb.UserId),
		TenantID:          parseUUID(pb.TenantId),
		ItemKey:           pb.ItemKey,
		ItemType:          pb.ItemType,
		PrimaryRefID:      parseUUIDPtr(pb.PrimaryRefId),
		Title:             pb.Title,
		SummaryExcerpt:    pb.SummaryExcerpt,
		Tags:              pb.Tags,
		Score:             pb.Score,
		ProjectionVersion: int(pb.ProjectionVersion),
		SummaryState:      pb.SummaryState,
		SupersedeState:    pb.SupersedeState,
		PreviousRefJSON:   pb.PreviousRefJson,
		Link:              pb.Link,
	}
	if pb.GeneratedAt != nil {
		item.GeneratedAt = pb.GeneratedAt.AsTime()
	}
	if pb.UpdatedAt != nil {
		item.UpdatedAt = pb.UpdatedAt.AsTime()
	}
	if pb.FreshnessAt != nil {
		t := pb.FreshnessAt.AsTime()
		item.FreshnessAt = &t
	}
	if pb.PublishedAt != nil {
		t := pb.PublishedAt.AsTime()
		item.PublishedAt = &t
	}
	if pb.LastInteractedAt != nil {
		t := pb.LastInteractedAt.AsTime()
		item.LastInteractedAt = &t
	}
	if pb.DismissedAt != nil {
		t := pb.DismissedAt.AsTime()
		item.DismissedAt = &t
	}
	if pb.SupersededAt != nil {
		t := pb.SupersededAt.AsTime()
		item.SupersededAt = &t
	}
	for _, r := range pb.WhyReasons {
		item.WhyReasons = append(item.WhyReasons, domain.WhyReason{
			Code: r.Code, RefID: r.RefId, Tag: r.Tag,
		})
	}
	return item
}

func protoToEvents(pbs []*sovereignv1.KnowledgeEvent) []domain.KnowledgeEvent {
	events := make([]domain.KnowledgeEvent, len(pbs))
	for i, pb := range pbs {
		events[i] = domain.KnowledgeEvent{
			EventID:       parseUUID(pb.EventId),
			EventSeq:      pb.EventSeq,
			TenantID:      parseUUID(pb.TenantId),
			UserID:        parseUUIDPtr(pb.UserId),
			ActorType:     pb.ActorType,
			ActorID:       pb.ActorId,
			EventType:     pb.EventType,
			AggregateType: pb.AggregateType,
			AggregateID:   pb.AggregateId,
			CorrelationID: parseUUIDPtr(pb.CorrelationId),
			CausationID:   parseUUIDPtr(pb.CausationId),
			DedupeKey:     pb.DedupeKey,
			Payload:       json.RawMessage(pb.Payload),
		}
		if pb.OccurredAt != nil {
			events[i].OccurredAt = pb.OccurredAt.AsTime()
		}
	}
	return events
}

func domainEventToProto(e domain.KnowledgeEvent) *sovereignv1.KnowledgeEvent {
	pb := &sovereignv1.KnowledgeEvent{
		EventId:       e.EventID.String(),
		EventSeq:      e.EventSeq,
		TenantId:      e.TenantID.String(),
		ActorType:     e.ActorType,
		ActorId:       e.ActorID,
		EventType:     e.EventType,
		AggregateType: e.AggregateType,
		AggregateId:   e.AggregateID,
		DedupeKey:     e.DedupeKey,
		Payload:       e.Payload,
	}
	if !e.OccurredAt.IsZero() {
		pb.OccurredAt = timeToProto(e.OccurredAt)
	}
	if e.UserID != nil {
		pb.UserId = e.UserID.String()
	}
	if e.CorrelationID != nil {
		pb.CorrelationId = e.CorrelationID.String()
	}
	if e.CausationID != nil {
		pb.CausationId = e.CausationID.String()
	}
	return pb
}

func protoToProjectionVersion(pb *sovereignv1.ProjectionVersion) *domain.KnowledgeProjectionVersion {
	v := &domain.KnowledgeProjectionVersion{
		Version:     int(pb.Version),
		Description: pb.Description,
		Status:      pb.Status,
	}
	if pb.CreatedAt != nil {
		v.CreatedAt = pb.CreatedAt.AsTime()
	}
	if pb.ActivatedAt != nil {
		t := pb.ActivatedAt.AsTime()
		v.ActivatedAt = &t
	}
	return v
}

func domainToProtoVersion(v domain.KnowledgeProjectionVersion) *sovereignv1.ProjectionVersion {
	pb := &sovereignv1.ProjectionVersion{
		Version:     int32(v.Version),
		Description: v.Description,
		Status:      v.Status,
		CreatedAt:   timeToProto(v.CreatedAt),
	}
	if v.ActivatedAt != nil {
		pb.ActivatedAt = timeToProto(*v.ActivatedAt)
	}
	return pb
}

func timeToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}
