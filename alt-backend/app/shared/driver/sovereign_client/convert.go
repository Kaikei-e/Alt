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
		FootprintKey:    pb.FootprintKey,
		Verb:            pb.Verb,
		ItemKey:         pb.ItemKey,
		Title:           pb.Title,
		Excerpt:         pb.Excerpt,
		Tags:            pb.Tags,
		Note:            pb.Note,
		SourceEventType: pb.SourceEventType,
		Wear:            pb.Wear,
		ContactCount:    int(pb.ContactCount),
	}
	if pb.OccurredAt != nil {
		fp.OccurredAt = pb.OccurredAt.AsTime()
	}
	if pb.FirstOccurredAt != nil {
		fp.FirstOccurredAt = pb.FirstOccurredAt.AsTime()
	}
	return fp
}

// protoToTrailBranches maps wire branches to their domain form. Shared by
// GetTrailFootprints and GetTrailBranchesForAnchor so the two branch surfaces
// (Trail episode header vs. patch-exit) can never diverge on mapping.
func protoToTrailBranches(pbs []*sovereignv1.TrailBranch) []domain.TrailBranch {
	branches := make([]domain.TrailBranch, len(pbs))
	for i, pb := range pbs {
		refs := make([]domain.TrailEvidenceRef, len(pb.EvidenceRefs))
		for j, r := range pb.EvidenceRefs {
			refs[j] = domain.TrailEvidenceRef{RefID: r.RefId, Label: r.Label, Kind: r.Kind}
		}
		branches[i] = domain.TrailBranch{
			BranchKey:     pb.BranchKey,
			AnchorItemKey: pb.AnchorItemKey,
			RelationKind:  pb.RelationKind,
			Why:           pb.Why,
			EvidenceRefs:  refs,
			Confidence:    pb.Confidence,
			TargetItemKey: pb.TargetItemKey,
			TargetTitle:   pb.TargetTitle,
		}
	}
	return branches
}

func protoToTrailEpisode(pb *sovereignv1.TrailEpisode) domain.TrailEpisode {
	epFootprints := make([]domain.TrailFootprint, len(pb.Footprints))
	for j, fpb := range pb.Footprints {
		epFootprints[j] = protoToTrailFootprint(fpb)
	}
	return domain.TrailEpisode{
		EpisodeKey: pb.EpisodeKey,
		Wear:       pb.Wear,
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
	return result
}

func protoToRecallCandidate(pb *sovereignv1.RecallCandidate) domain.RecallCandidate {
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
	return cand
}

// === Knowledge Home Item Converters ===

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
		URL:               pb.Url,
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

// === Event Converters ===

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

// === Projection Version Converters ===

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
