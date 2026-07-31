package datahub_gateway

import (
	"encoding/json"
	"testing"
	"time"

	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The §2.K mapping is where the append-first invariants become bytes, so these
// tests are about the three places a value can lose its meaning in transit:
// an optional that becomes a zero, a jsonb payload that gets re-encoded, and a
// malformed id that becomes uuid.Nil.

func TestSummaryVersionRoundTrip(t *testing.T) {
	score := 0.75
	supersededBy := uuid.New()
	original := domain.SummaryVersion{
		SummaryVersionID: uuid.New(),
		ArticleID:        uuid.New(),
		UserID:           uuid.New(),
		GeneratedAt:      time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
		Model:            "stream-summarize",
		PromptVersion:    "v3",
		InputHash:        "abc123",
		QualityScore:     &score,
		SummaryText:      "a summary",
		SupersededBy:     &supersededBy,
		// Transport for the knowledge event, never a column. It must not
		// survive the round trip, because a value that came back would mean the
		// provider had started storing it.
		ArticleTitle: "The article",
	}

	back, err := summaryVersionFromProto(summaryVersionToProto(original))
	require.NoError(t, err)

	assert.Equal(t, original.SummaryVersionID, back.SummaryVersionID)
	assert.Equal(t, original.ArticleID, back.ArticleID)
	assert.Equal(t, original.UserID, back.UserID)
	assert.Equal(t, original.GeneratedAt, back.GeneratedAt.UTC())
	assert.Equal(t, original.Model, back.Model)
	assert.Equal(t, original.PromptVersion, back.PromptVersion)
	assert.Equal(t, original.InputHash, back.InputHash)
	assert.Equal(t, original.SummaryText, back.SummaryText)
	require.NotNil(t, back.QualityScore)
	assert.InDelta(t, score, *back.QualityScore, 1e-9)
	require.NotNil(t, back.SupersededBy)
	assert.Equal(t, supersededBy, *back.SupersededBy)
	assert.Empty(t, back.ArticleTitle, "article_title is not on the wire and must not come back")
}

// TestSummaryVersionOptionalsStayAbsent is the one that would go unnoticed.
//
// A nil quality score turned into 0.0 makes every ungraded summary look like
// the worst one in the system, and a nil superseded_by turned into the zero
// UUID makes a current version look replaced by an article that does not exist.
// Both encode as valid values, so only an assertion on absence catches them.
func TestSummaryVersionOptionalsStayAbsent(t *testing.T) {
	msg := summaryVersionToProto(domain.SummaryVersion{
		SummaryVersionID: uuid.New(),
		ArticleID:        uuid.New(),
		UserID:           uuid.New(),
		SummaryText:      "a summary",
	})

	assert.Nil(t, msg.QualityScore)
	assert.Nil(t, msg.SupersededBy)
	// A zero generated_at stays absent rather than becoming 1970, which would
	// sort first in any ordering by it.
	assert.Nil(t, msg.GeneratedAt)

	back, err := summaryVersionFromProto(msg)
	require.NoError(t, err)
	assert.Nil(t, back.QualityScore)
	assert.Nil(t, back.SupersededBy)
	assert.True(t, back.GeneratedAt.IsZero())
}

func TestSummaryVersionFromProtoRejectsMalformedIDs(t *testing.T) {
	valid := uuid.New().String()

	tests := []struct {
		name string
		msg  *datahubv1.SummaryVersion
	}{
		{name: "nil message"},
		{name: "bad summary version id", msg: &datahubv1.SummaryVersion{
			SummaryVersionId: "nope", ArticleId: valid, UserId: valid}},
		{name: "bad article id", msg: &datahubv1.SummaryVersion{
			SummaryVersionId: valid, ArticleId: "nope", UserId: valid}},
		{name: "bad user id", msg: &datahubv1.SummaryVersion{
			SummaryVersionId: valid, ArticleId: valid, UserId: "nope"}},
		{name: "bad superseded by", msg: func() *datahubv1.SummaryVersion {
			bad := "nope"
			return &datahubv1.SummaryVersion{
				SummaryVersionId: valid, ArticleId: valid, UserId: valid, SupersededBy: &bad}
		}()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := summaryVersionFromProto(tt.msg)
			// An error rather than a uuid.Nil: the zero UUID is a
			// valid-looking key that would silently address some other
			// article's versions.
			assert.Error(t, err)
		})
	}
}

// TestTagSetVersionTagsJSONIsForwardedVerbatim pins the byte-for-byte rule.
//
// The column holds what the generator wrote. Decoding and re-encoding would
// reorder keys and drop insignificant whitespace, so a reprojection would
// return bytes the generator never produced — and a test comparing parsed JSON
// would still pass.
func TestTagSetVersionTagsJSONIsForwardedVerbatim(t *testing.T) {
	raw := json.RawMessage(`[{"name":"AI","confidence":0.9},{"name":"Go"}]`)

	original := domain.TagSetVersion{
		TagSetVersionID: uuid.New(),
		ArticleID:       uuid.New(),
		UserID:          uuid.New(),
		GeneratedAt:     time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
		Generator:       "tag-generator",
		InputHash:       "hash",
		TagsJSON:        raw,
	}

	msg := tagSetVersionToProto(original)
	assert.Equal(t, string(raw), string(msg.GetTagsJson()))

	back, err := tagSetVersionFromProto(msg)
	require.NoError(t, err)
	assert.Equal(t, string(raw), string(back.TagsJSON))
	assert.Equal(t, original.Generator, back.Generator)
	assert.Equal(t, original.GeneratedAt, back.GeneratedAt.UTC())
	assert.Nil(t, back.SupersededBy)
}

func TestTagSetVersionFromProtoRejectsMalformedIDs(t *testing.T) {
	valid := uuid.New().String()

	tests := []struct {
		name string
		msg  *datahubv1.TagSetVersion
	}{
		{name: "nil message"},
		{name: "bad tag set version id", msg: &datahubv1.TagSetVersion{
			TagSetVersionId: "nope", ArticleId: valid, UserId: valid}},
		{name: "bad article id", msg: &datahubv1.TagSetVersion{
			TagSetVersionId: valid, ArticleId: "nope", UserId: valid}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tagSetVersionFromProto(tt.msg)
			assert.Error(t, err)
		})
	}
}

// TestTrendWindowToProto pins the closed set and the message an unknown window
// gets.
//
// The wording matters: this is the string the dashboard endpoint has always
// answered for an unsupported window, and it now comes from this side without a
// round trip. The four accepted values are the provider's, because each selects
// both a lower bound and the date_trunc unit the query groups by.
func TestTrendWindowToProto(t *testing.T) {
	tests := []struct {
		window  string
		want    datahubv1.TrendWindow
		wantErr bool
	}{
		{window: "4h", want: datahubv1.TrendWindow_TREND_WINDOW_4H},
		{window: "24h", want: datahubv1.TrendWindow_TREND_WINDOW_24H},
		{window: "3d", want: datahubv1.TrendWindow_TREND_WINDOW_3D},
		{window: "7d", want: datahubv1.TrendWindow_TREND_WINDOW_7D},
		{window: "90d", wantErr: true},
		{window: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run("window "+tt.window, func(t *testing.T) {
			got, err := trendWindowToProto(tt.window)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unsupported window")
				assert.Equal(t, datahubv1.TrendWindow_TREND_WINDOW_UNSPECIFIED, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestTrendGranularityFromProto pins that an unspecified granularity becomes ""
// rather than a guess. The renderer reads the empty string as "no bucketing
// stated"; a guess would label hourly points as daily.
func TestTrendGranularityFromProto(t *testing.T) {
	assert.Equal(t, "hourly", trendGranularityFromProto(datahubv1.TrendGranularity_TREND_GRANULARITY_HOURLY))
	assert.Equal(t, "daily", trendGranularityFromProto(datahubv1.TrendGranularity_TREND_GRANULARITY_DAILY))
	assert.Equal(t, "", trendGranularityFromProto(datahubv1.TrendGranularity_TREND_GRANULARITY_UNSPECIFIED))
}
