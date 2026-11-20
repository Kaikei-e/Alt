package sovereign_client

import (
	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"
	"alt/utils/safeconv"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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

func timeToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

// === Trail Converters ===

func protoToTrailFootprint(pb *sovereignv1.TrailFootprint) domain.TrailFootprint {
	fp := domain.TrailFootprint{
		FootprintKey:    pb.GetFootprintKey(),
		Verb:            pb.GetVerb(),
		ItemKey:         pb.GetItemKey(),
		Title:           pb.GetTitle(),
		Excerpt:         pb.GetExcerpt(),
		Tags:            pb.GetTags(),
		Note:            pb.GetNote(),
		SourceEventType: pb.GetSourceEventType(),
		Wear:            pb.GetWear(),
		ContactCount:    int(pb.GetContactCount()),
	}
	if pb.GetOccurredAt() != nil {
		fp.OccurredAt = pb.GetOccurredAt().AsTime()
	}
	if pb.GetFirstOccurredAt() != nil {
		fp.FirstOccurredAt = pb.GetFirstOccurredAt().AsTime()
	}
	return fp
}

// protoToTrailBranches maps wire branches to their domain form. Shared by
// GetTrailFootprints and GetTrailBranchesForAnchor so the two branch surfaces
// (Trail episode header vs. patch-exit) can never diverge on mapping.
func protoToTrailBranches(pbs []*sovereignv1.TrailBranch) []domain.TrailBranch {
	branches := make([]domain.TrailBranch, len(pbs))
	for i, pb := range pbs {
		refs := make([]domain.TrailEvidenceRef, len(pb.GetEvidenceRefs()))
		for j, r := range pb.GetEvidenceRefs() {
			refs[j] = domain.TrailEvidenceRef{RefID: r.GetRefId(), Label: r.GetLabel(), Kind: r.GetKind()}
		}
		branches[i] = domain.TrailBranch{
			BranchKey:     pb.GetBranchKey(),
			AnchorItemKey: pb.GetAnchorItemKey(),
			RelationKind:  pb.GetRelationKind(),
			Why:           pb.GetWhy(),
			EvidenceRefs:  refs,
			Confidence:    pb.GetConfidence(),
			TargetItemKey: pb.GetTargetItemKey(),
			TargetTitle:   pb.GetTargetTitle(),
		}
	}
	return branches
}

func protoToTrailEpisode(pb *sovereignv1.TrailEpisode) domain.TrailEpisode {
	epFootprints := make([]domain.TrailFootprint, len(pb.GetFootprints()))
	for j, fpb := range pb.GetFootprints() {
		epFootprints[j] = protoToTrailFootprint(fpb)
	}
	return domain.TrailEpisode{
		EpisodeKey: pb.GetEpisodeKey(),
		Wear:       pb.GetWear(),
		// ThumbnailURL is left "" here: enrichment happens at the usecase
		// layer, keyed by the representative article id (D29).
		Footprints: epFootprints,
	}
}

func protoToTrailEpisodes(pbs []*sovereignv1.TrailEpisode) []domain.TrailEpisode {
	episodes := make([]domain.TrailEpisode, len(pbs))
	for i, pb := range pbs {
		episodes[i] = protoToTrailEpisode(pb)
	}
	return episodes
}

// === Digest & Recall Converters ===

func protoToTodayDigest(d *sovereignv1.TodayDigest, fallbackUserID uuid.UUID, fallbackDate time.Time) domain.TodayDigest {
	if d == nil {
		return domain.TodayDigest{UserID: fallbackUserID, DigestDate: fallbackDate, UpdatedAt: time.Now()}
	}
	digestDate, _ := time.Parse("2006-01-02", d.GetDigestDate())
	result := domain.TodayDigest{
		UserID:               parseUUID(d.GetUserId()),
		DigestDate:           digestDate,
		NewArticles:          int(d.GetNewArticles()),
		SummarizedArticles:   int(d.GetSummarizedArticles()),
		UnsummarizedArticles: int(d.GetUnsummarizedArticles()),
		TopTags:              d.GetTopTags(),
		WeeklyRecapAvailable: d.GetWeeklyRecapAvailable(),
		NeedToKnowCount:      int(d.GetNeedToKnowCount()),
		DigestFreshness:      d.GetDigestFreshness(),
	}
	if d.GetUpdatedAt() != nil {
		result.UpdatedAt = d.GetUpdatedAt().AsTime()
	}
	if d.GetLastProjectedAt() != nil {
		t := d.GetLastProjectedAt().AsTime()
		result.LastProjectedAt = &t
	}
	return result
}

func protoToRecallCandidate(pb *sovereignv1.RecallCandidate) domain.RecallCandidate {
	cand := domain.RecallCandidate{
		UserID:            parseUUID(pb.GetUserId()),
		ItemKey:           pb.GetItemKey(),
		RecallScore:       pb.GetRecallScore(),
		ProjectionVersion: int(pb.GetProjectionVersion()),
	}
	if pb.GetUpdatedAt() != nil {
		cand.UpdatedAt = pb.GetUpdatedAt().AsTime()
	}
	if pb.GetNextSuggestAt() != nil {
		t := pb.GetNextSuggestAt().AsTime()
		cand.NextSuggestAt = &t
	}
	if pb.GetFirstEligibleAt() != nil {
		t := pb.GetFirstEligibleAt().AsTime()
		cand.FirstEligibleAt = &t
	}
	for _, r := range pb.GetReasons() {
		cand.Reasons = append(cand.Reasons, domain.RecallReason{
			Type: r.GetType(), Description: r.GetDescription(), SourceItemKey: r.GetSourceItemKey(),
		})
	}
	if pb.GetItem() != nil {
		item := protoToHomeItem(pb.GetItem())
		cand.Item = &item
	}
	return cand
}

// === Knowledge Home Item Converters ===

func protoToHomeItem(pb *sovereignv1.KnowledgeHomeItem) domain.KnowledgeHomeItem {
	item := domain.KnowledgeHomeItem{
		UserID:            parseUUID(pb.GetUserId()),
		TenantID:          parseUUID(pb.GetTenantId()),
		ItemKey:           pb.GetItemKey(),
		ItemType:          pb.GetItemType(),
		PrimaryRefID:      parseUUIDPtr(pb.GetPrimaryRefId()),
		Title:             pb.GetTitle(),
		SummaryExcerpt:    pb.GetSummaryExcerpt(),
		Tags:              pb.GetTags(),
		Score:             pb.GetScore(),
		ProjectionVersion: int(pb.GetProjectionVersion()),
		SummaryState:      pb.GetSummaryState(),
		SupersedeState:    pb.GetSupersedeState(),
		PreviousRefJSON:   pb.GetPreviousRefJson(),
		URL:               pb.GetUrl(),
	}
	if pb.GetGeneratedAt() != nil {
		item.GeneratedAt = pb.GetGeneratedAt().AsTime()
	}
	if pb.GetUpdatedAt() != nil {
		item.UpdatedAt = pb.GetUpdatedAt().AsTime()
	}
	if pb.GetFreshnessAt() != nil {
		t := pb.GetFreshnessAt().AsTime()
		item.FreshnessAt = &t
	}
	if pb.GetPublishedAt() != nil {
		t := pb.GetPublishedAt().AsTime()
		item.PublishedAt = &t
	}
	if pb.GetLastInteractedAt() != nil {
		t := pb.GetLastInteractedAt().AsTime()
		item.LastInteractedAt = &t
	}
	if pb.GetDismissedAt() != nil {
		t := pb.GetDismissedAt().AsTime()
		item.DismissedAt = &t
	}
	if pb.GetSupersededAt() != nil {
		t := pb.GetSupersededAt().AsTime()
		item.SupersededAt = &t
	}
	for _, r := range pb.GetWhyReasons() {
		item.WhyReasons = append(item.WhyReasons, domain.WhyReason{
			Code: r.GetCode(), RefID: r.GetRefId(), Tag: r.GetTag(),
		})
	}
	return item
}

// === Event Converters ===

func protoToEvents(pbs []*sovereignv1.KnowledgeEvent) []domain.KnowledgeEvent {
	events := make([]domain.KnowledgeEvent, len(pbs))
	for i, pb := range pbs {
		events[i] = domain.KnowledgeEvent{
			EventID:       parseUUID(pb.GetEventId()),
			EventSeq:      pb.GetEventSeq(),
			TenantID:      parseUUID(pb.GetTenantId()),
			UserID:        parseUUIDPtr(pb.GetUserId()),
			ActorType:     pb.GetActorType(),
			ActorID:       pb.GetActorId(),
			EventType:     pb.GetEventType(),
			AggregateType: pb.GetAggregateType(),
			AggregateID:   pb.GetAggregateId(),
			CorrelationID: parseUUIDPtr(pb.GetCorrelationId()),
			CausationID:   parseUUIDPtr(pb.GetCausationId()),
			DedupeKey:     pb.GetDedupeKey(),
			Payload:       json.RawMessage(pb.GetPayload()),
		}
		if pb.GetOccurredAt() != nil {
			events[i].OccurredAt = pb.GetOccurredAt().AsTime()
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

// === Projection Version Converters ===

func protoToProjectionVersion(pb *sovereignv1.ProjectionVersion) *domain.KnowledgeProjectionVersion {
	v := &domain.KnowledgeProjectionVersion{
		Version:     int(pb.GetVersion()),
		Description: pb.GetDescription(),
		Status:      pb.GetStatus(),
	}
	if pb.GetCreatedAt() != nil {
		v.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	if pb.GetActivatedAt() != nil {
		t := pb.GetActivatedAt().AsTime()
		v.ActivatedAt = &t
	}
	return v
}

func domainToProtoVersion(v domain.KnowledgeProjectionVersion) *sovereignv1.ProjectionVersion {
	pb := &sovereignv1.ProjectionVersion{
		Version:     safeconv.Int32(v.Version),
		Description: v.Description,
		Status:      v.Status,
		CreatedAt:   timeToProto(v.CreatedAt),
	}
	if v.ActivatedAt != nil {
		pb.ActivatedAt = timeToProto(*v.ActivatedAt)
	}
	return pb
}
