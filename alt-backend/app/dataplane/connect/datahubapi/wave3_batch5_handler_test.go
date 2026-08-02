package datahubapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

type fakeSummaryVersionPort struct {
	created    []domain.SummaryVersion
	prev       *domain.SummaryVersion
	byID       domain.SummaryVersion
	latest     domain.SummaryVersion
	err        error
	notFound   bool
	supersedes [][2]uuid.UUID
}

func (f *fakeSummaryVersionPort) CreateSummaryVersion(_ context.Context, sv domain.SummaryVersion) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, sv)
	return nil
}

func (f *fakeSummaryVersionPort) MarkSummaryVersionSuperseded(_ context.Context, articleID, newVersionID uuid.UUID) (*domain.SummaryVersion, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.supersedes = append(f.supersedes, [2]uuid.UUID{articleID, newVersionID})
	return f.prev, nil
}

func (f *fakeSummaryVersionPort) GetSummaryVersionByID(_ context.Context, _ uuid.UUID) (domain.SummaryVersion, error) {
	if f.notFound {
		return domain.SummaryVersion{}, errors.New("no summary version found for id x")
	}
	if f.err != nil {
		return domain.SummaryVersion{}, f.err
	}
	return f.byID, nil
}

func (f *fakeSummaryVersionPort) GetLatestSummaryVersion(_ context.Context, _ uuid.UUID) (domain.SummaryVersion, error) {
	if f.notFound {
		return domain.SummaryVersion{}, errors.New("no summary version found for article x")
	}
	if f.err != nil {
		return domain.SummaryVersion{}, f.err
	}
	return f.latest, nil
}

type fakeTagSetVersionPort struct {
	created []domain.TagSetVersion
	prev    *domain.TagSetVersion
	byID    domain.TagSetVersion
	err     error
}

func (f *fakeTagSetVersionPort) CreateTagSetVersion(_ context.Context, tsv domain.TagSetVersion) error {
	if f.err != nil {
		return f.err
	}
	f.created = append(f.created, tsv)
	return nil
}

func (f *fakeTagSetVersionPort) MarkTagSetVersionSuperseded(_ context.Context, _, _ uuid.UUID) (*domain.TagSetVersion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.prev, nil
}

func (f *fakeTagSetVersionPort) GetTagSetVersionByID(_ context.Context, _ uuid.UUID) (domain.TagSetVersion, error) {
	if f.err != nil {
		return domain.TagSetVersion{}, f.err
	}
	return f.byID, nil
}

type fakeStatsPort struct {
	feedAmount   int
	total        int
	summarized   int
	unsummarized int
	todayUnread  int
	series       *datahub_capability_port.TrendSeries
	feedIDs      []uuid.UUID
	err          error

	sawUser   uuid.UUID
	sawSince  time.Time
	sawWindow string
}

func (f *fakeStatsPort) FeedAmount(context.Context) (int, error) { return f.feedAmount, f.err }

func (f *fakeStatsPort) TotalArticles(_ context.Context, userID uuid.UUID) (int, error) {
	f.sawUser = userID
	return f.total, f.err
}

func (f *fakeStatsPort) SummarizedArticles(_ context.Context, userID uuid.UUID) (int, error) {
	f.sawUser = userID
	return f.summarized, f.err
}

func (f *fakeStatsPort) UnsummarizedArticles(_ context.Context, userID uuid.UUID) (int, error) {
	f.sawUser = userID
	return f.unsummarized, f.err
}

func (f *fakeStatsPort) TodayUnread(_ context.Context, userID uuid.UUID, since time.Time) (int, error) {
	f.sawUser = userID
	f.sawSince = since
	return f.todayUnread, f.err
}

func (f *fakeStatsPort) TrendStats(_ context.Context, userID uuid.UUID, window string) (*datahub_capability_port.TrendSeries, error) {
	f.sawUser = userID
	f.sawWindow = window
	return f.series, f.err
}

func (f *fakeStatsPort) UserFeedIDs(_ context.Context, userID uuid.UUID) ([]uuid.UUID, error) {
	f.sawUser = userID
	return f.feedIDs, f.err
}

func batch5Handler(t *testing.T, sv *fakeSummaryVersionPort, tsv *fakeTagSetVersionPort, stats *fakeStatsPort) *Handler {
	t.Helper()
	if sv == nil {
		sv = &fakeSummaryVersionPort{}
	}
	if tsv == nil {
		tsv = &fakeTagSetVersionPort{}
	}
	if stats == nil {
		stats = &fakeStatsPort{}
	}
	return NewHandler(nil, nil, nil, nil, nil, &fakeSystemUser{}, &fakeRecentArticles{}, slog.Default(),
		WithWave3Batch5Capabilities(sv, tsv, stats))
}

const (
	testArticleID    = "7a2b3c4d-5e6f-4a1b-8c2d-3e4f5a6b7c8d"
	testVersionID    = "11111111-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	testPrevID       = "22222222-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	testStatsUser    = "9f8e7d6c-5b4a-4392-8281-706f5e4d3c2b"
	testTagSetVerID  = "33333333-cccc-4ccc-8ccc-cccccccccccc"
	testStatsFeedIDs = "5c6d7e8f-9a0b-4c1d-8e2f-3a4b5c6d7e8f"
)

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------

// TestWithWave3Batch5CapabilitiesRefusesNil is the rule-8 test for this batch.
//
// A nil version port would make the procedure answer Unimplemented, which is
// indistinguishable from a retired one — and the damage is silent rather than
// loud: the caller appends SummaryVersionCreated to knowledge-sovereign
// regardless, so sovereign would fill with references to versions that were
// never written and nothing would surface until a replay.
func TestWithWave3Batch5CapabilitiesRefusesNil(t *testing.T) {
	tests := []struct {
		name  string
		sv    datahub_capability_port.SummaryVersionPort
		tsv   datahub_capability_port.TagSetVersionPort
		stats datahub_capability_port.StatsPort
	}{
		{name: "nil summary version port", tsv: &fakeTagSetVersionPort{}, stats: &fakeStatsPort{}},
		{name: "nil tag set version port", sv: &fakeSummaryVersionPort{}, stats: &fakeStatsPort{}},
		{name: "nil stats port", sv: &fakeSummaryVersionPort{}, tsv: &fakeTagSetVersionPort{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				WithWave3Batch5Capabilities(tt.sv, tt.tsv, tt.stats)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// §2.K Versioned artifacts
// ---------------------------------------------------------------------------

func TestCreateSummaryVersion(t *testing.T) {
	sv := &fakeSummaryVersionPort{}
	h := batch5Handler(t, sv, nil, nil)

	score := 0.8
	generatedAt := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)

	resp, err := h.CreateSummaryVersion(context.Background(), connect.NewRequest(&datahubv1.CreateSummaryVersionRequest{
		Version: &datahubv1.SummaryVersion{
			SummaryVersionId: testVersionID,
			ArticleId:        testArticleID,
			UserId:           testStatsUser,
			GeneratedAt:      timestamppb.New(generatedAt),
			Model:            "stream-summarize",
			SummaryText:      "a summary",
			QualityScore:     &score,
		},
	}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	require.Len(t, sv.created, 1)
	assert.Equal(t, testVersionID, sv.created[0].SummaryVersionID.String())
	assert.Equal(t, generatedAt, sv.created[0].GeneratedAt.UTC())
	require.NotNil(t, sv.created[0].QualityScore)
	assert.InDelta(t, 0.8, *sv.created[0].QualityScore, 1e-9)
}

// TestCreateSummaryVersionQualityScoreAbsent pins that an absent score stays
// absent rather than becoming 0.0.
//
// The two are different facts — a summary that scored nothing versus one that
// was never graded — and the column is nullable for that reason. A pointer that
// arrived as &0 would make every ungraded summary look like the worst one in
// the system.
func TestCreateSummaryVersionQualityScoreAbsent(t *testing.T) {
	sv := &fakeSummaryVersionPort{}
	h := batch5Handler(t, sv, nil, nil)

	_, err := h.CreateSummaryVersion(context.Background(), connect.NewRequest(&datahubv1.CreateSummaryVersionRequest{
		Version: &datahubv1.SummaryVersion{
			SummaryVersionId: testVersionID,
			ArticleId:        testArticleID,
			UserId:           testStatsUser,
			SummaryText:      "a summary",
		},
	}))
	require.NoError(t, err)
	require.Len(t, sv.created, 1)
	assert.Nil(t, sv.created[0].QualityScore)
}

func TestCreateSummaryVersionRejectsBadInput(t *testing.T) {
	tests := []struct {
		name    string
		version *datahubv1.SummaryVersion
	}{
		{name: "no version at all"},
		{name: "no summary version id", version: &datahubv1.SummaryVersion{
			ArticleId: testArticleID, UserId: testStatsUser, SummaryText: "x"}},
		{name: "non-uuid article id", version: &datahubv1.SummaryVersion{
			SummaryVersionId: testVersionID, ArticleId: "not-a-uuid", UserId: testStatsUser, SummaryText: "x"}},
		{name: "no user id", version: &datahubv1.SummaryVersion{
			SummaryVersionId: testVersionID, ArticleId: testArticleID, SummaryText: "x"}},
		// An empty summary is refused rather than stored: summary_versions is
		// append-only, so an empty row cannot be corrected, only superseded.
		{name: "empty summary text", version: &datahubv1.SummaryVersion{
			SummaryVersionId: testVersionID, ArticleId: testArticleID, UserId: testStatsUser}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sv := &fakeSummaryVersionPort{}
			h := batch5Handler(t, sv, nil, nil)

			_, err := h.CreateSummaryVersion(context.Background(),
				connect.NewRequest(&datahubv1.CreateSummaryVersionRequest{Version: tt.version}))
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Empty(t, sv.created, "nothing may be written for a request that was refused")
		})
	}
}

// TestMarkSummaryVersionSupersededReportsPrevious and its FirstVersion twin are
// the pair the append-first story depends on.
//
// The caller emits SummarySuperseded only when a previous version comes back,
// and excerpts its text into the payload. Returning a zero-valued message for
// "there was none" would announce the replacement of a summary that never
// existed, with an empty excerpt — and both branches would still compile,
// still return 200, and still look right in a log.
func TestMarkSummaryVersionSupersededReportsPrevious(t *testing.T) {
	prevID := uuid.MustParse(testPrevID)
	articleID := uuid.MustParse(testArticleID)
	sv := &fakeSummaryVersionPort{prev: &domain.SummaryVersion{
		SummaryVersionID: prevID,
		ArticleID:        articleID,
		UserID:           uuid.MustParse(testStatsUser),
		GeneratedAt:      time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
		Model:            "pre-processor",
		SummaryText:      "the older summary",
	}}
	h := batch5Handler(t, sv, nil, nil)

	resp, err := h.MarkSummaryVersionSuperseded(context.Background(),
		connect.NewRequest(&datahubv1.MarkSummaryVersionSupersededRequest{
			ArticleId:    testArticleID,
			NewVersionId: testVersionID,
		}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.PreviousVersion)
	assert.Equal(t, testPrevID, resp.Msg.GetPreviousVersion().GetSummaryVersionId())
	assert.Equal(t, "the older summary", resp.Msg.GetPreviousVersion().GetSummaryText())

	// The article and the new version travel together into one port call, which
	// is what keeps the advisory lock and the update inside one transaction.
	require.Len(t, sv.supersedes, 1)
	assert.Equal(t, articleID, sv.supersedes[0][0])
	assert.Equal(t, uuid.MustParse(testVersionID), sv.supersedes[0][1])
}

func TestMarkSummaryVersionSupersededFirstVersion(t *testing.T) {
	h := batch5Handler(t, &fakeSummaryVersionPort{prev: nil}, nil, nil)

	resp, err := h.MarkSummaryVersionSuperseded(context.Background(),
		connect.NewRequest(&datahubv1.MarkSummaryVersionSupersededRequest{
			ArticleId:    testArticleID,
			NewVersionId: testVersionID,
		}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.PreviousVersion,
		"an article's first version supersedes nothing, and that must not arrive as a zero value")
}

func TestMarkSummaryVersionSupersededRejectsBadIDs(t *testing.T) {
	h := batch5Handler(t, nil, nil, nil)

	tests := []struct {
		name         string
		articleID    string
		newVersionID string
	}{
		{name: "no article id", newVersionID: testVersionID},
		{name: "no new version id", articleID: testArticleID},
		{name: "non-uuid article id", articleID: "nope", newVersionID: testVersionID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := h.MarkSummaryVersionSuperseded(context.Background(),
				connect.NewRequest(&datahubv1.MarkSummaryVersionSupersededRequest{
					ArticleId:    tt.articleID,
					NewVersionId: tt.newVersionID,
				}))
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

// TestGetSummaryVersionByIDReturnsSupersededVersion is the reproject-safe read
// stated as a test: a version that has been replaced is still returned, with
// supersededBy set. A provider that filtered those out would make replaying an
// old event impossible.
func TestGetSummaryVersionByIDReturnsSupersededVersion(t *testing.T) {
	supersededBy := uuid.MustParse(testVersionID)
	h := batch5Handler(t, &fakeSummaryVersionPort{byID: domain.SummaryVersion{
		SummaryVersionID: uuid.MustParse(testPrevID),
		ArticleID:        uuid.MustParse(testArticleID),
		UserID:           uuid.MustParse(testStatsUser),
		SummaryText:      "the older summary",
		SupersededBy:     &supersededBy,
	}}, nil, nil)

	resp, err := h.GetSummaryVersionByID(context.Background(),
		connect.NewRequest(&datahubv1.GetSummaryVersionByIDRequest{SummaryVersionId: testPrevID}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GetVersion().SupersededBy)
	assert.Equal(t, testVersionID, resp.Msg.GetVersion().GetSupersededBy())
}

// TestGetSummaryVersionNotFound pins that an absent version is NotFound rather
// than Internal.
//
// A projector treats Internal as transient and retries; a version that will
// never exist would be retried forever.
func TestGetSummaryVersionNotFound(t *testing.T) {
	h := batch5Handler(t, &fakeSummaryVersionPort{notFound: true}, nil, nil)

	_, err := h.GetSummaryVersionByID(context.Background(),
		connect.NewRequest(&datahubv1.GetSummaryVersionByIDRequest{SummaryVersionId: testPrevID}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))

	_, err = h.GetLatestSummaryVersion(context.Background(),
		connect.NewRequest(&datahubv1.GetLatestSummaryVersionRequest{ArticleId: testArticleID}))
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestGetLatestSummaryVersionCarriesNoSupersededBy(t *testing.T) {
	h := batch5Handler(t, &fakeSummaryVersionPort{latest: domain.SummaryVersion{
		SummaryVersionID: uuid.MustParse(testVersionID),
		ArticleID:        uuid.MustParse(testArticleID),
		UserID:           uuid.MustParse(testStatsUser),
		SummaryText:      "a summary",
	}}, nil, nil)

	resp, err := h.GetLatestSummaryVersion(context.Background(),
		connect.NewRequest(&datahubv1.GetLatestSummaryVersionRequest{ArticleId: testArticleID}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.GetVersion().SupersededBy,
		"'latest' means nothing has replaced it, so a supersededBy here is a contradiction")
}

// TestCreateTagSetVersionPassesTagsJSONThrough pins that the jsonb payload is
// forwarded byte for byte.
//
// The column holds what the generator wrote. Decoding and re-encoding it here
// would reorder keys, so a later reproject would return bytes the generator
// never produced — a difference no test that compared parsed JSON would catch.
func TestCreateTagSetVersionPassesTagsJSONThrough(t *testing.T) {
	tsv := &fakeTagSetVersionPort{}
	h := batch5Handler(t, nil, tsv, nil)

	raw := []byte(`[{"name":"AI","confidence":0.9},{"name":"Go"}]`)

	_, err := h.CreateTagSetVersion(context.Background(), connect.NewRequest(&datahubv1.CreateTagSetVersionRequest{
		Version: &datahubv1.TagSetVersion{
			TagSetVersionId: testTagSetVerID,
			ArticleId:       testArticleID,
			UserId:          testStatsUser,
			Generator:       "tag-generator",
			TagsJson:        raw,
		},
	}))
	require.NoError(t, err)
	require.Len(t, tsv.created, 1)
	assert.Equal(t, string(raw), string(tsv.created[0].TagsJSON))
	assert.JSONEq(t, string(raw), string(tsv.created[0].TagsJSON))
}

func TestMarkTagSetVersionSupersededFirstVersion(t *testing.T) {
	h := batch5Handler(t, nil, &fakeTagSetVersionPort{prev: nil}, nil)

	resp, err := h.MarkTagSetVersionSuperseded(context.Background(),
		connect.NewRequest(&datahubv1.MarkTagSetVersionSupersededRequest{
			ArticleId:    testArticleID,
			NewVersionId: testTagSetVerID,
		}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.PreviousVersion)
}

func TestGetTagSetVersionByID(t *testing.T) {
	h := batch5Handler(t, nil, &fakeTagSetVersionPort{byID: domain.TagSetVersion{
		TagSetVersionID: uuid.MustParse(testTagSetVerID),
		ArticleID:       uuid.MustParse(testArticleID),
		UserID:          uuid.MustParse(testStatsUser),
		Generator:       "tag-generator",
		TagsJSON:        json.RawMessage(`[{"name":"AI"}]`),
	}}, nil)

	resp, err := h.GetTagSetVersionByID(context.Background(),
		connect.NewRequest(&datahubv1.GetTagSetVersionByIDRequest{TagSetVersionId: testTagSetVerID}))
	require.NoError(t, err)
	assert.Equal(t, testTagSetVerID, resp.Msg.GetVersion().GetTagSetVersionId())
	assert.JSONEq(t, `[{"name":"AI"}]`, string(resp.Msg.GetVersion().GetTagsJson()))
}

// ---------------------------------------------------------------------------
// §2.M Statistics / dashboard
// ---------------------------------------------------------------------------

// TestStatsProceduresRequireTheTenant is the batch's tenancy test.
//
// These counts read the owner from a request field because the context on this
// side describes a peer certificate naming alt-backend. Defaulting a missing
// one to uuid.Nil would answer 0 for every question and read as an empty
// account rather than as a bug.
func TestStatsProceduresRequireTheTenant(t *testing.T) {
	h := batch5Handler(t, nil, nil, &fakeStatsPort{})

	tests := []struct {
		name string
		call func() error
	}{
		{"GetTotalArticlesCount", func() error {
			_, err := h.GetTotalArticlesCount(context.Background(),
				connect.NewRequest(&datahubv1.GetTotalArticlesCountRequest{}))
			return err
		}},
		{"GetSummarizedArticlesCount", func() error {
			_, err := h.GetSummarizedArticlesCount(context.Background(),
				connect.NewRequest(&datahubv1.GetSummarizedArticlesCountRequest{}))
			return err
		}},
		{"GetUnsummarizedArticlesCount", func() error {
			_, err := h.GetUnsummarizedArticlesCount(context.Background(),
				connect.NewRequest(&datahubv1.GetUnsummarizedArticlesCountRequest{}))
			return err
		}},
		{"GetTodayUnreadArticlesCount", func() error {
			_, err := h.GetTodayUnreadArticlesCount(context.Background(),
				connect.NewRequest(&datahubv1.GetTodayUnreadArticlesCountRequest{Since: timestamppb.Now()}))
			return err
		}},
		{"GetTrendStats", func() error {
			_, err := h.GetTrendStats(context.Background(),
				connect.NewRequest(&datahubv1.GetTrendStatsRequest{Window: datahubv1.TrendWindow_TREND_WINDOW_7D}))
			return err
		}},
		{"ListUserFeedIDs", func() error {
			_, err := h.ListUserFeedIDs(context.Background(),
				connect.NewRequest(&datahubv1.ListUserFeedIDsRequest{}))
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(tt.call()))
		})
	}
}

// TestGetFeedAmountTakesNoTenant is the deliberate exception: this count is the
// deployment's size, and adding a user to it would silently change what the
// number means.
func TestGetFeedAmountTakesNoTenant(t *testing.T) {
	h := batch5Handler(t, nil, nil, &fakeStatsPort{feedAmount: 42})

	resp, err := h.GetFeedAmount(context.Background(), connect.NewRequest(&datahubv1.GetFeedAmountRequest{}))
	require.NoError(t, err)
	assert.Equal(t, int32(42), resp.Msg.GetCount())
}

// TestGetTodayUnreadArticlesCountRequiresSince pins that the bound is not
// defaulted. "Today" needs the reader's timezone, which this side does not
// have; a server-side midnight would answer a different question convincingly.
func TestGetTodayUnreadArticlesCountRequiresSince(t *testing.T) {
	stats := &fakeStatsPort{}
	h := batch5Handler(t, nil, nil, stats)

	_, err := h.GetTodayUnreadArticlesCount(context.Background(),
		connect.NewRequest(&datahubv1.GetTodayUnreadArticlesCountRequest{UserId: testStatsUser}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.True(t, stats.sawSince.IsZero(), "no query may run for a request that was refused")
}

func TestGetTodayUnreadArticlesCountForwardsSince(t *testing.T) {
	stats := &fakeStatsPort{todayUnread: 7}
	h := batch5Handler(t, nil, nil, stats)

	since := time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	resp, err := h.GetTodayUnreadArticlesCount(context.Background(),
		connect.NewRequest(&datahubv1.GetTodayUnreadArticlesCountRequest{
			UserId: testStatsUser,
			Since:  timestamppb.New(since),
		}))
	require.NoError(t, err)
	assert.Equal(t, int32(7), resp.Msg.GetCount())
	assert.Equal(t, since, stats.sawSince.UTC())
	assert.Equal(t, testStatsUser, stats.sawUser.String())
}

// TestGetTrendStatsRejectsUnspecifiedWindow pins that the enum's zero value is
// refused rather than defaulted.
//
// The four windows differ by two orders of magnitude in rows scanned, so a
// caller that forgot the field would silently get whichever one the provider
// preferred — and would get it fast enough not to notice on 4h, slow enough to
// matter on 7d.
func TestGetTrendStatsRejectsUnspecifiedWindow(t *testing.T) {
	stats := &fakeStatsPort{}
	h := batch5Handler(t, nil, nil, stats)

	_, err := h.GetTrendStats(context.Background(),
		connect.NewRequest(&datahubv1.GetTrendStatsRequest{UserId: testStatsUser}))
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Empty(t, stats.sawWindow)
}

func TestGetTrendStatsMapsWindowAndGranularity(t *testing.T) {
	tests := []struct {
		name       string
		window     datahubv1.TrendWindow
		wantWindow string
	}{
		{"4h", datahubv1.TrendWindow_TREND_WINDOW_4H, "4h"},
		{"24h", datahubv1.TrendWindow_TREND_WINDOW_24H, "24h"},
		{"3d", datahubv1.TrendWindow_TREND_WINDOW_3D, "3d"},
		{"7d", datahubv1.TrendWindow_TREND_WINDOW_7D, "7d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bucket := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
			stats := &fakeStatsPort{series: &datahub_capability_port.TrendSeries{
				Points: []datahub_capability_port.TrendPoint{
					{Timestamp: bucket, Articles: 12, Summarized: 9, FeedActivity: 3},
				},
				Granularity: "daily",
			}}
			h := batch5Handler(t, nil, nil, stats)

			resp, err := h.GetTrendStats(context.Background(),
				connect.NewRequest(&datahubv1.GetTrendStatsRequest{UserId: testStatsUser, Window: tt.window}))
			require.NoError(t, err)
			assert.Equal(t, tt.wantWindow, stats.sawWindow)

			require.Len(t, resp.Msg.GetPoints(), 1)
			assert.Equal(t, bucket, resp.Msg.GetPoints()[0].GetBucket().AsTime())
			assert.Equal(t, int32(12), resp.Msg.GetPoints()[0].GetArticles())
			// Reported by the provider, not echoed from the request: the
			// date_trunc unit is a property of the query.
			assert.Equal(t, datahubv1.TrendGranularity_TREND_GRANULARITY_DAILY, resp.Msg.GetGranularity())
		})
	}
}

func TestListUserFeedIDs(t *testing.T) {
	h := batch5Handler(t, nil, nil, &fakeStatsPort{
		feedIDs: []uuid.UUID{uuid.MustParse(testStatsFeedIDs)},
	})

	resp, err := h.ListUserFeedIDs(context.Background(),
		connect.NewRequest(&datahubv1.ListUserFeedIDsRequest{UserId: testStatsUser}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetFeedIds(), 1)
	assert.Equal(t, testStatsFeedIDs, resp.Msg.GetFeedIds()[0])
}

// TestStatsErrorsAreInternal pins that a failed count is Internal and carries
// no detail. The caller is another service; the query text would travel to a
// place that cannot act on it, and the log on this side already has it.
func TestStatsErrorsAreInternal(t *testing.T) {
	h := batch5Handler(t, nil, nil, &fakeStatsPort{err: errors.New("connection refused")})

	_, err := h.GetFeedAmount(context.Background(), connect.NewRequest(&datahubv1.GetFeedAmountRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
	assert.NotContains(t, err.Error(), "connection refused")
}
