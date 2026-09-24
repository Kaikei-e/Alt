package trail_planner

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

func TestWhyPhraseForVerb_CoversEveryEngagementVerb(t *testing.T) {
	for _, verb := range sovereign_db.EngagementVerbs {
		phrase, ok := whyPhraseForVerb(verb)
		assert.True(t, ok, "verb %q must have an anchored why phrase", verb)
		assert.NotEmpty(t, phrase)
	}

	_, ok := whyPhraseForVerb("dismissed")
	assert.False(t, ok, "dismissal is not an engagement act")
}

func TestBuildClusterBranch_WhyNamesTheActThatHappened(t *testing.T) {
	user := uuid.New()
	c := sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:target",
		TargetTitle:   "Target Article",
		SharedTags:    []string{"rust"},
	}

	tests := []struct {
		verb       string
		wantPhrase string
	}{
		{"read", "Because you read \"Anchor Title\" — joins rust"},
		{"asked", "Because you asked about \"Anchor Title\" — joins rust"},
		{"listened", "Because you listened to \"Anchor Title\" — joins rust"},
	}

	for _, tt := range tests {
		t.Run(tt.verb, func(t *testing.T) {
			phrase, ok := whyPhraseForVerb(tt.verb)
			require.True(t, ok)
			ref := anchorRef{itemKey: "article:anchor", title: "Anchor Title", whyPhrase: phrase}
			payload := buildClusterBranch(user, ref, c)
			assert.Equal(t, tt.wantPhrase, payload.Why)
		})
	}
}

func TestBuildContinuationBranch_WhyNamesTheActThatHappened(t *testing.T) {
	user := uuid.New()
	c := sovereign_db.TrailContinuationCandidate{
		TargetItemKey: "article:c",
		TargetTitle:   "Quiet Thread",
		Verb:          "asked",
	}
	phrase, ok := whyPhraseForVerb(c.Verb)
	require.True(t, ok)
	payload := buildContinuationBranch(user, phrase, c)
	assert.Equal(t, "Because you asked about \"Quiet Thread\" and the thread went quiet — pick it back up.", payload.Why)
	assert.Equal(t, "article:c", payload.AnchorItemKey)
	assert.Equal(t, "article:c", payload.TargetItemKey)
}

func TestBuildClusterBranch_AlwaysPopulatesFourTuple(t *testing.T) {
	user := uuid.New()
	ref := anchorRef{itemKey: "article:a", title: "A", whyPhrase: "you read"}
	c := sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:b",
		TargetTitle:   "B",
		SharedTags:    []string{"go"},
	}
	payload := buildClusterBranch(user, ref, c)
	assert.True(t, payload.Valid())
	assert.Equal(t, "cluster", payload.RelationKind)
	assert.NotEmpty(t, payload.Why)
	assert.NotEmpty(t, payload.EvidenceRefs)
	assert.NotEmpty(t, payload.Confidence)
}

func TestBuildClusterBranch_SingleTagIsPlausible(t *testing.T) {
	user := uuid.New()
	ref := anchorRef{itemKey: "article:a", title: "A", whyPhrase: "you read"}
	c := sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:b",
		TargetTitle:   "B",
		SharedTags:    []string{"go"},
	}
	payload := buildClusterBranch(user, ref, c)
	assert.Equal(t, "plausible", payload.Confidence)

	c.SharedTags = []string{"go", "cli"}
	payload = buildClusterBranch(user, ref, c)
	assert.Equal(t, "corroborated", payload.Confidence)
}

func TestBuildClusterBranch_WhyReferencesAnchorTitleInQuotes(t *testing.T) {
	user := uuid.New()
	ref := anchorRef{itemKey: "article:a", title: "Specific Anchor Title", whyPhrase: "you read"}
	c := sovereign_db.TrailClusterCandidate{
		TargetItemKey: "article:b",
		TargetTitle:   "Target",
		SharedTags:    []string{"topic"},
	}
	payload := buildClusterBranch(user, ref, c)
	assert.Contains(t, payload.Why, "\"Specific Anchor Title\"")
}

func TestValidResolution(t *testing.T) {
	tests := []struct {
		resolution string
		want       bool
	}{
		{"taken", true},
		{"dismissed", true},
		{"unknown", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.resolution, func(t *testing.T) {
			assert.Equal(t, tt.want, ValidResolution(tt.resolution))
		})
	}
}

func TestBranchProposedPayload_Valid(t *testing.T) {
	base := BranchProposedPayload{
		RelationKind: "cluster",
		Why:          "Because reason",
		EvidenceRefs: []EvidenceRef{{RefID: "t1", Label: "t1", Kind: "tag"}},
		Confidence:   "plausible",
	}
	assert.True(t, base.Valid())

	missingKind := base
	missingKind.RelationKind = ""
	assert.False(t, missingKind.Valid())

	missingWhy := base
	missingWhy.Why = ""
	assert.False(t, missingWhy.Valid())

	missingRefs := base
	missingRefs.EvidenceRefs = nil
	assert.False(t, missingRefs.Valid())

	missingConfidence := base
	missingConfidence.Confidence = ""
	assert.False(t, missingConfidence.Valid())
}

func TestBuildBranchProposedEvent(t *testing.T) {
	userID := uuid.New()
	tenantID := uuid.New()
	fixedTime := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	payload := BranchProposedPayload{
		BranchKey:     "cluster:" + userID.String() + ":article:x",
		AnchorItemKey: "article:a",
		RelationKind:  "cluster",
		Why:           "Because why",
		EvidenceRefs:  []EvidenceRef{{RefID: "r1", Label: "r1", Kind: "tag"}},
		Confidence:    "plausible",
		TargetItemKey: "article:x",
		TargetTitle:   "Article X",
	}

	evt, err := buildBranchProposedEvent(userID, tenantID, payload, fixedTime)
	require.NoError(t, err)

	assert.Equal(t, EventTrailBranchProposed, evt.EventType)
	assert.Equal(t, "trail_branch", evt.AggregateType)
	assert.Equal(t, payload.BranchKey, evt.AggregateID)
	assert.Equal(t, EventTrailBranchProposed+":"+payload.BranchKey, evt.DedupeKey)
	assert.Equal(t, fixedTime, evt.OccurredAt)
	assert.Equal(t, tenantID, evt.TenantID)
	require.NotNil(t, evt.UserID)
	assert.Equal(t, userID, *evt.UserID)
	assert.Equal(t, "system", evt.ActorType)
	assert.Equal(t, "trail-planner", evt.ActorID)

	var parsed BranchProposedPayload
	require.NoError(t, json.Unmarshal(evt.Payload, &parsed))
	assert.Equal(t, payload.BranchKey, parsed.BranchKey)
	assert.Equal(t, payload.Why, parsed.Why)
}
