package sovereign_client

import (
	"encoding/json"
	"testing"
	"time"

	"alt/domain"
	sovereignv1 "alt/gen/proto/services/sovereign/v1"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestParseUUID(t *testing.T) {
	validID := uuid.New()
	tests := []struct {
		name     string
		input    string
		expected uuid.UUID
	}{
		{
			name:     "valid uuid",
			input:    validID.String(),
			expected: validID,
		},
		{
			name:     "invalid uuid falls back to nil uuid",
			input:    "not-a-uuid",
			expected: uuid.Nil,
		},
		{
			name:     "empty string falls back to nil uuid",
			input:    "",
			expected: uuid.Nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUUID(tt.input)
			if got != tt.expected {
				t.Fatalf("expected %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestParseUUIDPtr(t *testing.T) {
	validID := uuid.New()
	tests := []struct {
		name     string
		input    string
		expected *uuid.UUID
	}{
		{
			name:     "valid uuid returns pointer",
			input:    validID.String(),
			expected: &validID,
		},
		{
			name:     "invalid uuid returns nil",
			input:    "invalid-uuid",
			expected: nil,
		},
		{
			name:     "empty string returns nil",
			input:    "",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseUUIDPtr(tt.input)
			if tt.expected == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
			} else {
				if got == nil || *got != *tt.expected {
					t.Fatalf("expected %v, got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestTimeToProto(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	tests := []struct {
		name     string
		input    time.Time
		expected *timestamppb.Timestamp
	}{
		{
			name:     "zero time returns nil",
			input:    time.Time{},
			expected: nil,
		},
		{
			name:     "valid time returns timestamp",
			input:    now,
			expected: timestamppb.New(now),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := timeToProto(tt.input)
			if tt.expected == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
			} else {
				if got == nil || got.AsTime().Unix() != tt.expected.AsTime().Unix() {
					t.Fatalf("expected %v, got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestProtoToTrailFootprint(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	pb := &sovereignv1.TrailFootprint{
		FootprintKey:    "fp-1",
		Verb:            "read",
		ItemKey:         "item-1",
		Title:           "Article 1",
		Excerpt:         "Excerpt 1",
		Tags:            []string{"go", "solid"},
		Note:            "A note",
		SourceEventType: "article_read",
		Wear:            "0.85",
		ContactCount:    3,
		OccurredAt:      timestamppb.New(now),
		FirstOccurredAt: timestamppb.New(now.Add(-time.Hour)),
	}

	fp := protoToTrailFootprint(pb)
	if fp.FootprintKey != pb.FootprintKey || fp.Verb != pb.Verb || fp.ItemKey != pb.ItemKey {
		t.Errorf("unexpected footprint fields: %+v", fp)
	}
	if fp.ContactCount != 3 || fp.Wear != "0.85" {
		t.Errorf("unexpected numeric fields: %+v", fp)
	}
	if fp.OccurredAt.Unix() != now.Unix() || fp.FirstOccurredAt.Unix() != now.Add(-time.Hour).Unix() {
		t.Errorf("unexpected timestamps: %+v", fp)
	}
}

func TestProtoToTrailBranches(t *testing.T) {
	pbs := []*sovereignv1.TrailBranch{
		{
			BranchKey:     "br-1",
			AnchorItemKey: "item-1",
			RelationKind:  "continuation",
			Why:           "next part in series",
			Confidence:    "0.9",
			TargetItemKey: "item-2",
			TargetTitle:   "Article 2",
			EvidenceRefs: []*sovereignv1.TrailEvidenceRef{
				{
					RefId: "ref-1",
					Label: "Label 1",
					Kind:  "kind-1",
				},
			},
		},
	}

	branches := protoToTrailBranches(pbs)
	if len(branches) != 1 {
		t.Fatalf("expected 1 branch, got %d", len(branches))
	}
	b := branches[0]
	if b.BranchKey != "br-1" || b.TargetTitle != "Article 2" || len(b.EvidenceRefs) != 1 {
		t.Fatalf("unexpected branch content: %+v", b)
	}
	if b.EvidenceRefs[0].RefID != "ref-1" || b.EvidenceRefs[0].Label != "Label 1" {
		t.Fatalf("unexpected evidence ref: %+v", b.EvidenceRefs[0])
	}
}

func TestProtoToTrailEpisode(t *testing.T) {
	pb := &sovereignv1.TrailEpisode{
		EpisodeKey: "ep-1",
		Wear:       "0.75",
		Footprints: []*sovereignv1.TrailFootprint{
			{FootprintKey: "fp-1", ItemKey: "item-1"},
			{FootprintKey: "fp-2", ItemKey: "item-2"},
		},
	}

	ep := protoToTrailEpisode(pb)
	if ep.EpisodeKey != "ep-1" || ep.Wear != "0.75" || len(ep.Footprints) != 2 {
		t.Fatalf("unexpected episode: %+v", ep)
	}

	eps := protoToTrailEpisodes([]*sovereignv1.TrailEpisode{pb})
	if len(eps) != 1 || eps[0].EpisodeKey != "ep-1" {
		t.Fatalf("unexpected episodes slice: %+v", eps)
	}
}

func TestProtoToTodayDigest(t *testing.T) {
	uid := uuid.New()
	fallbackDate := time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC)

	t.Run("nil digest uses fallback", func(t *testing.T) {
		got := protoToTodayDigest(nil, uid, fallbackDate)
		if got.UserID != uid || got.DigestDate != fallbackDate || got.UpdatedAt.IsZero() {
			t.Fatalf("unexpected fallback digest: %+v", got)
		}
	})

	t.Run("valid proto digest", func(t *testing.T) {
		now := time.Now().Truncate(time.Second)
		pb := &sovereignv1.TodayDigest{
			UserId:                uid.String(),
			DigestDate:            "2026-09-25",
			NewArticles:           10,
			SummarizedArticles:    8,
			UnsummarizedArticles:  2,
			TopTags:               []string{"go", "grpc"},
			WeeklyRecapAvailable:  true,
			EveningPulseAvailable: false,
			NeedToKnowCount:       3,
			DigestFreshness:       "fresh",
			UpdatedAt:             timestamppb.New(now),
			LastProjectedAt:       timestamppb.New(now),
		}

		got := protoToTodayDigest(pb, uid, fallbackDate)
		if got.UserID != uid || got.NewArticles != 10 || got.SummarizedArticles != 8 || !got.WeeklyRecapAvailable {
			t.Fatalf("unexpected parsed digest: %+v", got)
		}
		if got.UpdatedAt.Unix() != now.Unix() || got.LastProjectedAt == nil || got.LastProjectedAt.Unix() != now.Unix() {
			t.Fatalf("unexpected timestamps: %+v", got)
		}
	})
}

func TestProtoToRecallCandidate(t *testing.T) {
	uid := uuid.New()
	now := time.Now().Truncate(time.Second)

	pb := &sovereignv1.RecallCandidate{
		UserId:            uid.String(),
		ItemKey:           "item-key-1",
		RecallScore:       0.95,
		ProjectionVersion: 2,
		UpdatedAt:         timestamppb.New(now),
		NextSuggestAt:     timestamppb.New(now.Add(time.Hour)),
		FirstEligibleAt:   timestamppb.New(now.Add(-time.Hour)),
		Reasons: []*sovereignv1.RecallReason{
			{Type: "spaced_rep", Description: "review interval", SourceItemKey: "source-1"},
		},
		Item: &sovereignv1.KnowledgeHomeItem{
			UserId:   uid.String(),
			TenantId: uid.String(),
			ItemKey:  "item-key-1",
			Title:    "Candidate item",
		},
	}

	cand := protoToRecallCandidate(pb)
	if cand.UserID != uid || cand.ItemKey != "item-key-1" || cand.RecallScore != 0.95 {
		t.Fatalf("unexpected candidate: %+v", cand)
	}
	if len(cand.Reasons) != 1 || cand.Reasons[0].Type != "spaced_rep" {
		t.Fatalf("unexpected reasons: %+v", cand.Reasons)
	}
	if cand.Item == nil || cand.Item.Title != "Candidate item" {
		t.Fatalf("unexpected embedded item: %+v", cand.Item)
	}
}

func TestProtoToHomeItem(t *testing.T) {
	uid := uuid.New()
	tid := uuid.New()
	refID := uuid.New()
	now := time.Now().Truncate(time.Second)

	pb := &sovereignv1.KnowledgeHomeItem{
		UserId:            uid.String(),
		TenantId:          tid.String(),
		ItemKey:           "item-key-1",
		ItemType:          "article",
		PrimaryRefId:      refID.String(),
		Title:             "Home item",
		SummaryExcerpt:    "Summary...",
		Tags:              []string{"tech"},
		Score:             0.88,
		ProjectionVersion: 1,
		SummaryState:      "done",
		SupersedeState:    "active",
		PreviousRefJson:   `{"prev":"ref"}`,
		Url:               "https://example.com/item",
		GeneratedAt:       timestamppb.New(now),
		UpdatedAt:         timestamppb.New(now),
		FreshnessAt:       timestamppb.New(now),
		PublishedAt:       timestamppb.New(now),
		LastInteractedAt:  timestamppb.New(now),
		DismissedAt:       timestamppb.New(now),
		SupersededAt:      timestamppb.New(now),
		WhyReasons: []*sovereignv1.WhyReason{
			{Code: "TOPIC_INTEREST", RefId: "ref-why", Tag: "tech"},
		},
	}

	item := protoToHomeItem(pb)
	if item.UserID != uid || item.TenantID != tid || item.PrimaryRefID == nil || *item.PrimaryRefID != refID {
		t.Fatalf("unexpected IDs: %+v", item)
	}
	if item.Title != "Home item" || item.URL != "https://example.com/item" {
		t.Fatalf("unexpected text fields: %+v", item)
	}
	if len(item.WhyReasons) != 1 || item.WhyReasons[0].Code != "TOPIC_INTEREST" {
		t.Fatalf("unexpected why reasons: %+v", item.WhyReasons)
	}
}

func TestEventsConversion(t *testing.T) {
	eventID := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	corrID := uuid.New()
	causID := uuid.New()
	now := time.Now().Truncate(time.Second)

	evt := domain.KnowledgeEvent{
		EventID:       eventID,
		EventSeq:      42,
		TenantID:      tenantID,
		UserID:        &userID,
		ActorType:     "user",
		ActorID:       "actor-1",
		EventType:     "knowledge.article.read",
		AggregateType: "article",
		AggregateID:   "agg-1",
		CorrelationID: &corrID,
		CausationID:   &causID,
		DedupeKey:     "dedupe-1",
		Payload:       json.RawMessage(`{"key":"value"}`),
		OccurredAt:    now,
	}

	pb := domainEventToProto(evt)
	if pb.EventId != eventID.String() || pb.EventSeq != 42 || pb.TenantId != tenantID.String() {
		t.Fatalf("unexpected proto event: %+v", pb)
	}

	events := protoToEvents([]*sovereignv1.KnowledgeEvent{pb})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	got := events[0]
	if got.EventID != eventID || got.EventSeq != 42 || got.TenantID != tenantID {
		t.Fatalf("unexpected parsed event: %+v", got)
	}
	if got.UserID == nil || *got.UserID != userID {
		t.Fatalf("unexpected user ID in parsed event: %+v", got)
	}
}

func TestProjectionVersionConversion(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	v := domain.KnowledgeProjectionVersion{
		Version:     3,
		Description: "v3 schema",
		Status:      "active",
		CreatedAt:   now,
		ActivatedAt: &now,
	}

	pb := domainToProtoVersion(v)
	if pb.Version != 3 || pb.Description != "v3 schema" || pb.Status != "active" {
		t.Fatalf("unexpected proto version: %+v", pb)
	}

	got := protoToProjectionVersion(pb)
	if got.Version != 3 || got.Description != "v3 schema" || got.Status != "active" {
		t.Fatalf("unexpected domain version: %+v", got)
	}
	if got.CreatedAt.Unix() != now.Unix() || got.ActivatedAt == nil || got.ActivatedAt.Unix() != now.Unix() {
		t.Fatalf("unexpected dates: %+v", got)
	}
}
