package datahubapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"alt/dataplane/port/datahub_capability_port"
	"alt/dataplane/usecase/outbox_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/alt/datahub/v1"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

type fakeOutboxPort struct {
	claimed    []domain.OutboxEvent
	claimLimit int
	marks      []domain.OutboxEventStatus
	released   []string
	pruneWin   time.Duration
	pruneCount int64
}

func (f *fakeOutboxPort) ClaimBatch(_ context.Context, limit int) ([]domain.OutboxEvent, error) {
	f.claimLimit = limit
	return f.claimed, nil
}

func (f *fakeOutboxPort) MarkProcessed(_ context.Context, _ string, status domain.OutboxEventStatus, _ string) error {
	f.marks = append(f.marks, status)
	return nil
}

func (f *fakeOutboxPort) Release(_ context.Context, id string) error {
	f.released = append(f.released, id)
	return nil
}

func (f *fakeOutboxPort) Prune(_ context.Context, olderThan time.Duration) (int64, error) {
	f.pruneWin = olderThan
	return f.pruneCount, nil
}

type fakeOgImagePort struct {
	head       *domain.ArticleHead
	headErr    error
	urls       map[string]string
	candidates []domain.OgImageBackfillCandidate
	unwarmed   []string
	purgeTTL   time.Duration
	purged     int64

	// SaveArticleHead is catalog §2.B W3-B2, added in Wave 3 batch 2. It
	// writes the table the reads above read, so it lives on the same port.
	savedHeadArticleID string
	savedHeadHTML      string
	savedHeadOgImage   string
}

func (f *fakeOgImagePort) SaveArticleHead(_ context.Context, articleID, headHTML, ogImageURL string) error {
	f.savedHeadArticleID, f.savedHeadHTML, f.savedHeadOgImage = articleID, headHTML, ogImageURL
	return nil
}

func (f *fakeOgImagePort) GetArticleHead(_ context.Context, _ string) (*domain.ArticleHead, error) {
	return f.head, f.headErr
}

func (f *fakeOgImagePort) BatchGetOgImageURLs(_ context.Context, _ []string) (map[string]string, error) {
	return f.urls, nil
}

func (f *fakeOgImagePort) ListFeedsMissingOgImage(_ context.Context, _ int) ([]domain.OgImageBackfillCandidate, error) {
	return f.candidates, nil
}

func (f *fakeOgImagePort) ListUnwarmedOgImageURLs(_ context.Context, _ int) ([]string, error) {
	return f.unwarmed, nil
}

func (f *fakeOgImagePort) PurgeExpiredArticleHeads(_ context.Context, ttl time.Duration) (int64, error) {
	f.purgeTTL = ttl
	return f.purged, nil
}

type fakeImageCachePort struct {
	entry    *domain.ImageProxyCacheEntry
	saved    *domain.ImageProxyCacheEntry
	purgeTTL time.Duration
}

func (f *fakeImageCachePort) Get(_ context.Context, _ string) (*domain.ImageProxyCacheEntry, error) {
	return f.entry, nil
}

func (f *fakeImageCachePort) Put(_ context.Context, entry *domain.ImageProxyCacheEntry) error {
	f.saved = entry
	return nil
}

func (f *fakeImageCachePort) EvictExpired(_ context.Context) (int64, error) { return 1, nil }

func (f *fakeImageCachePort) PurgeOlderThan(_ context.Context, ttl time.Duration) (int64, error) {
	f.purgeTTL = ttl
	return 2, nil
}

type fakeScrapingPort struct {
	byDomain *domain.ScrapingDomain
	saved    *domain.ScrapingDomain
	update   *domain.ScrapingPolicyUpdate
	declined bool
}

func (f *fakeScrapingPort) GetByDomain(_ context.Context, _ string) (*domain.ScrapingDomain, error) {
	return f.byDomain, nil
}

func (f *fakeScrapingPort) GetByID(_ context.Context, _ uuid.UUID) (*domain.ScrapingDomain, error) {
	return f.byDomain, nil
}

func (f *fakeScrapingPort) Save(_ context.Context, sd *domain.ScrapingDomain) (*domain.ScrapingDomain, error) {
	saved := *sd
	if saved.ID == uuid.Nil {
		saved.ID = uuid.MustParse("2b1c3d4e-5f60-4711-8899-aabbccddeeff")
	}
	saved.CreatedAt = time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)
	saved.UpdatedAt = saved.CreatedAt
	f.saved = &saved
	return &saved, nil
}

func (f *fakeScrapingPort) List(_ context.Context, _, _ int) ([]*domain.ScrapingDomain, error) {
	return []*domain.ScrapingDomain{f.byDomain}, nil
}

func (f *fakeScrapingPort) UpdatePolicy(_ context.Context, _ uuid.UUID, update *domain.ScrapingPolicyUpdate) error {
	f.update = update
	return nil
}

func (f *fakeScrapingPort) SaveDeclinedDomain(_ context.Context, _, _ string) error { return nil }

func (f *fakeScrapingPort) IsDomainDeclined(_ context.Context, _, _ string) (bool, error) {
	return f.declined, nil
}

type fakeAutoFulltextPort struct{}

func (fakeAutoFulltextPort) ListSubscribedUserIDsByFeedLinkID(_ context.Context, _ string) ([]string, error) {
	return []string{"u1"}, nil
}

func (fakeAutoFulltextPort) CheckArticleExistsByURLForUser(_ context.Context, _, _ string) (bool, string, error) {
	return true, "a1", nil
}

type wave3Fakes struct {
	outbox   *fakeOutboxPort
	ogImage  *fakeOgImagePort
	cache    *fakeImageCachePort
	scraping *fakeScrapingPort
}

// newWave3Handler builds a Handler with only the Wave 3 collaborators wired.
// The phase 1-4 ports stay nil: this file covers procedures that do not touch
// them, and leaving them out keeps a failure here attributable.
func newWave3Handler(f wave3Fakes) (*Handler, wave3Fakes) {
	if f.outbox == nil {
		f.outbox = &fakeOutboxPort{}
	}
	if f.ogImage == nil {
		f.ogImage = &fakeOgImagePort{}
	}
	if f.cache == nil {
		f.cache = &fakeImageCachePort{}
	}
	if f.scraping == nil {
		f.scraping = &fakeScrapingPort{}
	}

	h := &Handler{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	WithWave3Capabilities(
		outbox_usecase.NewOutboxUsecase(f.outbox),
		f.ogImage,
		f.cache,
		f.scraping,
		fakeAutoFulltextPort{},
	)(h)
	return h, f
}

// ---------------------------------------------------------------------------
// Wiring
// ---------------------------------------------------------------------------

// TestWithWave3CapabilitiesRejectsNilCollaborators: these are not features
// that can be switched off. A handler built without one would answer
// Unimplemented — the same answer a retired procedure gives — while
// alt-harvester ticked every five seconds and delivered nothing
// (CLAUDE.md rule 8 / ADR-000928).
func TestWithWave3CapabilitiesRejectsNilCollaborators(t *testing.T) {
	uc := outbox_usecase.NewOutboxUsecase(&fakeOutboxPort{})
	og := &fakeOgImagePort{}
	cache := &fakeImageCachePort{}
	scraping := &fakeScrapingPort{}
	auto := fakeAutoFulltextPort{}

	tests := []struct {
		name     string
		outbox   *outbox_usecase.OutboxUsecase
		ogImage  datahub_capability_port.OgImagePort
		cache    datahub_capability_port.ImageProxyCachePort
		scraping datahub_capability_port.ScrapingPolicyPort
		auto     datahub_capability_port.AutoFulltextPort
	}{
		{name: "no outbox", ogImage: og, cache: cache, scraping: scraping, auto: auto},
		{name: "no og image", outbox: uc, cache: cache, scraping: scraping, auto: auto},
		{name: "no image cache", outbox: uc, ogImage: og, scraping: scraping, auto: auto},
		{name: "no scraping policy", outbox: uc, ogImage: og, cache: cache, auto: auto},
		{name: "no auto fulltext", outbox: uc, ogImage: og, cache: cache, scraping: scraping},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() {
				WithWave3Capabilities(tt.outbox, tt.ogImage, tt.cache, tt.scraping, tt.auto)
			})
		})
	}
}

// ---------------------------------------------------------------------------
// §2.A Outbox
// ---------------------------------------------------------------------------

// TestClaimOutboxBatchReportsProcessing: the response must describe the rows
// as claimed. Reporting PENDING would describe a state no other caller can
// observe, and a consumer that trusted it would re-claim its own batch.
func TestClaimOutboxBatchReportsProcessing(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{outbox: &fakeOutboxPort{claimed: []domain.OutboxEvent{
		{ID: "e1", EventType: "ARTICLE_UPSERT", Payload: []byte(`{"a":1}`), Status: domain.OutboxProcessing, CreatedAt: time.Now()},
	}}})

	resp, err := h.ClaimOutboxBatch(context.Background(),
		connect.NewRequest(&datahubv1.ClaimOutboxBatchRequest{Limit: 10}))
	require.NoError(t, err)
	require.Len(t, resp.Msg.GetEvents(), 1)

	got := resp.Msg.GetEvents()[0]
	assert.Equal(t, datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSING, got.GetStatus())
	assert.Equal(t, []byte(`{"a":1}`), got.GetPayload())
	assert.Equal(t, 10, f.outbox.claimLimit)
}

func TestClaimOutboxBatchDefaultsUnsetLimit(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})

	_, err := h.ClaimOutboxBatch(context.Background(),
		connect.NewRequest(&datahubv1.ClaimOutboxBatchRequest{}))
	require.NoError(t, err)
	assert.Equal(t, outbox_usecase.DefaultClaimLimit, f.outbox.claimLimit)
}

// TestMarkOutboxProcessedRejectsNonTerminalStatus: each transition has exactly
// one procedure, and answering OK to the wrong one would leave the row where
// it was while the caller believed it had moved.
func TestMarkOutboxProcessedRejectsNonTerminalStatus(t *testing.T) {
	tests := []struct {
		name   string
		status datahubv1.OutboxEventStatus
	}{
		{name: "unspecified", status: datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_UNSPECIFIED},
		{name: "pending belongs to ReleaseOutboxEvent", status: datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PENDING},
		{name: "processing belongs to ClaimOutboxBatch", status: datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSING},
		{name: "an enum value this build does not know", status: datahubv1.OutboxEventStatus(99)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, f := newWave3Handler(wave3Fakes{})

			_, err := h.MarkOutboxProcessed(context.Background(),
				connect.NewRequest(&datahubv1.MarkOutboxProcessedRequest{Id: "e1", Status: tt.status}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			assert.Empty(t, f.outbox.marks, "the write must not reach the port")
		})
	}
}

func TestMarkOutboxProcessedAcceptsTerminalStatuses(t *testing.T) {
	for _, status := range []datahubv1.OutboxEventStatus{
		datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_PROCESSED,
		datahubv1.OutboxEventStatus_OUTBOX_EVENT_STATUS_FAILED,
	} {
		t.Run(status.String(), func(t *testing.T) {
			h, f := newWave3Handler(wave3Fakes{})

			_, err := h.MarkOutboxProcessed(context.Background(),
				connect.NewRequest(&datahubv1.MarkOutboxProcessedRequest{Id: "e1", Status: status}))
			require.NoError(t, err)
			require.Len(t, f.outbox.marks, 1)
		})
	}
}

func TestReleaseOutboxEventRequiresID(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})

	_, err := h.ReleaseOutboxEvent(context.Background(),
		connect.NewRequest(&datahubv1.ReleaseOutboxEventRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Empty(t, f.outbox.released)
}

func TestReleaseOutboxEventReturnsRowToPending(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})

	_, err := h.ReleaseOutboxEvent(context.Background(),
		connect.NewRequest(&datahubv1.ReleaseOutboxEventRequest{Id: "e1"}))
	require.NoError(t, err)
	assert.Equal(t, []string{"e1"}, f.outbox.released)
}

// TestPruneOutboxEventsRejectsNonPositiveWindow: an omitted protobuf field and
// an explicit zero are the same bytes, so accepting zero would let a caller
// that forgot the field delete every PROCESSED row.
func TestPruneOutboxEventsRejectsNonPositiveWindow(t *testing.T) {
	for _, seconds := range []int64{0, -1} {
		h, f := newWave3Handler(wave3Fakes{})

		_, err := h.PruneOutboxEvents(context.Background(),
			connect.NewRequest(&datahubv1.PruneOutboxEventsRequest{OlderThanSeconds: seconds}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		assert.Zero(t, f.outbox.pruneWin)
	}
}

func TestPruneOutboxEventsPassesCallerWindow(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{outbox: &fakeOutboxPort{pruneCount: 12}})

	resp, err := h.PruneOutboxEvents(context.Background(),
		connect.NewRequest(&datahubv1.PruneOutboxEventsRequest{OlderThanSeconds: 604800}))
	require.NoError(t, err)
	assert.Equal(t, int64(12), resp.Msg.GetPrunedCount())
	assert.Equal(t, 7*24*time.Hour, f.outbox.pruneWin)
}

// ---------------------------------------------------------------------------
// §2.D OG image
// ---------------------------------------------------------------------------

// TestGetArticleHeadLeavesFieldUnsetOnMiss: the caller re-scrapes on the
// absence. An empty ArticleHead here would read as "scraped, nothing found"
// and stop it permanently.
func TestGetArticleHeadLeavesFieldUnsetOnMiss(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{ogImage: &fakeOgImagePort{head: nil}})

	resp, err := h.GetArticleHead(context.Background(),
		connect.NewRequest(&datahubv1.GetArticleHeadRequest{ArticleId: "a1"}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.Head)
}

func TestGetArticleHeadReturnsStoredRow(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{ogImage: &fakeOgImagePort{
		head: &domain.ArticleHead{ID: "h1", ArticleID: "a1", HeadHTML: "<head></head>", OgImageURL: "https://cdn/og.png"},
	}})

	resp, err := h.GetArticleHead(context.Background(),
		connect.NewRequest(&datahubv1.GetArticleHeadRequest{ArticleId: "a1"}))
	require.NoError(t, err)
	require.NotNil(t, resp.Msg.GetHead())
	assert.Equal(t, "https://cdn/og.png", resp.Msg.GetHead().GetOgImageUrl())
}

func TestGetArticleHeadRequiresArticleID(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})

	_, err := h.GetArticleHead(context.Background(),
		connect.NewRequest(&datahubv1.GetArticleHeadRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestGetArticleHeadDoesNotLeakDriverErrorAsMiss: a failed read must be an
// error, never the absence that triggers a re-scrape.
func TestGetArticleHeadDoesNotLeakDriverErrorAsMiss(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{ogImage: &fakeOgImagePort{headErr: errors.New("pool exhausted")}})

	_, err := h.GetArticleHead(context.Background(),
		connect.NewRequest(&datahubv1.GetArticleHeadRequest{ArticleId: "a1"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInternal, connect.CodeOf(err))
}

func TestBatchGetOgImageURLsShortCircuitsEmptyInput(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})

	resp, err := h.BatchGetOgImageURLs(context.Background(),
		connect.NewRequest(&datahubv1.BatchGetOgImageURLsRequest{}))
	require.NoError(t, err)
	assert.Empty(t, resp.Msg.GetOgImageUrls())
}

func TestBatchGetOgImageURLsRejectsOversizedBatch(t *testing.T) {
	ids := make([]string, maxLimit+1)
	for i := range ids {
		ids[i] = "a"
	}
	h, _ := newWave3Handler(wave3Fakes{})

	_, err := h.BatchGetOgImageURLs(context.Background(),
		connect.NewRequest(&datahubv1.BatchGetOgImageURLsRequest{ArticleIds: ids}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestPurgeRetentionRejectsZeroTTL covers both retention deletes at once:
// zero is indistinguishable from an omitted field, and for
// PurgeExpiredArticleHeads it would empty the table.
func TestPurgeRetentionRejectsZeroTTL(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})
	ctx := context.Background()

	_, err := h.PurgeExpiredArticleHeads(ctx,
		connect.NewRequest(&datahubv1.PurgeExpiredArticleHeadsRequest{TtlSeconds: 0}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, f.ogImage.purgeTTL)

	_, err = h.PurgeImageProxyCacheOlderThan(ctx,
		connect.NewRequest(&datahubv1.PurgeImageProxyCacheOlderThanRequest{TtlSeconds: 0}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Zero(t, f.cache.purgeTTL)
}

// ---------------------------------------------------------------------------
// §2.E Image proxy cache
// ---------------------------------------------------------------------------

func TestGetImageProxyCacheLeavesFieldUnsetOnMiss(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{cache: &fakeImageCachePort{entry: nil}})

	resp, err := h.GetImageProxyCache(context.Background(),
		connect.NewRequest(&datahubv1.GetImageProxyCacheRequest{UrlHash: "abc"}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.Entry)
}

func TestPutImageProxyCacheRoundTripsBytes(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})

	_, err := h.PutImageProxyCache(context.Background(),
		connect.NewRequest(&datahubv1.PutImageProxyCacheRequest{
			Entry: &datahubv1.ImageProxyCacheEntry{
				UrlHash:     "abc",
				OriginalUrl: "https://cdn/og.png",
				Data:        []byte{0x52, 0x49, 0x46, 0x46},
				ContentType: "image/webp",
				SizeBytes:   4,
				ExpiresAt:   timestamppb.New(time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC)),
			},
		}))
	require.NoError(t, err)
	require.NotNil(t, f.cache.saved)
	assert.Equal(t, []byte{0x52, 0x49, 0x46, 0x46}, f.cache.saved.Data)
	assert.Equal(t, 4, f.cache.saved.SizeBytes)
	assert.False(t, f.cache.saved.ExpiresAt.IsZero(), "the writer's TTL must reach the row, not a provider default")
}

func TestPutImageProxyCacheRequiresURLHash(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})

	_, err := h.PutImageProxyCache(context.Background(),
		connect.NewRequest(&datahubv1.PutImageProxyCacheRequest{Entry: &datahubv1.ImageProxyCacheEntry{}}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Nil(t, f.cache.saved)
}

// ---------------------------------------------------------------------------
// §2.L Scraping policy
// ---------------------------------------------------------------------------

// TestGetScrapingDomainByDomainLeavesFieldUnsetForUnknownHost: the compliance
// check falls back to a live robots.txt fetch on the absence. Synthesising a
// default row here would skip that fallback for every first-time publisher.
func TestGetScrapingDomainByDomainLeavesFieldUnsetForUnknownHost(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{scraping: &fakeScrapingPort{byDomain: nil}})

	resp, err := h.GetScrapingDomainByDomain(context.Background(),
		connect.NewRequest(&datahubv1.GetScrapingDomainByDomainRequest{Domain: "unknown.example"}))
	require.NoError(t, err)
	assert.Nil(t, resp.Msg.ScrapingDomain)
}

// TestSaveScrapingDomainReturnsAssignedIdentity replaces the driver's in-place
// struct mutation, which cannot cross a process boundary.
func TestSaveScrapingDomainReturnsAssignedIdentity(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})

	resp, err := h.SaveScrapingDomain(context.Background(),
		connect.NewRequest(&datahubv1.SaveScrapingDomainRequest{
			ScrapingDomain: &datahubv1.ScrapingDomain{Domain: "example.com", Scheme: "https"},
		}))
	require.NoError(t, err)

	got := resp.Msg.GetScrapingDomain()
	require.NotNil(t, got)
	assert.Equal(t, "2b1c3d4e-5f60-4711-8899-aabbccddeeff", got.GetId())
	assert.NotNil(t, got.GetCreatedAt())
}

func TestSaveScrapingDomainRejectsMalformedID(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})

	_, err := h.SaveScrapingDomain(context.Background(),
		connect.NewRequest(&datahubv1.SaveScrapingDomainRequest{
			ScrapingDomain: &datahubv1.ScrapingDomain{Id: "not-a-uuid", Domain: "example.com"},
		}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// TestUpdateScrapingDomainPolicyKeepsAbsentFieldsAbsent: an omitted field must
// stay nil all the way to the COALESCE. Materialising it as false/0 would
// reset three policy flags the caller never mentioned.
func TestUpdateScrapingDomainPolicyKeepsAbsentFieldsAbsent(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})
	allow := false

	_, err := h.UpdateScrapingDomainPolicy(context.Background(),
		connect.NewRequest(&datahubv1.UpdateScrapingDomainPolicyRequest{
			Id:     "2b1c3d4e-5f60-4711-8899-aabbccddeeff",
			Update: &datahubv1.ScrapingPolicyUpdate{AllowFetchBody: &allow},
		}))
	require.NoError(t, err)

	require.NotNil(t, f.scraping.update)
	require.NotNil(t, f.scraping.update.AllowFetchBody)
	assert.False(t, *f.scraping.update.AllowFetchBody)
	assert.Nil(t, f.scraping.update.AllowMLTraining)
	assert.Nil(t, f.scraping.update.AllowCacheDays)
	assert.Nil(t, f.scraping.update.ForceRespectRobots)
}

func TestUpdateScrapingDomainPolicyRejectsMalformedID(t *testing.T) {
	h, f := newWave3Handler(wave3Fakes{})

	_, err := h.UpdateScrapingDomainPolicy(context.Background(),
		connect.NewRequest(&datahubv1.UpdateScrapingDomainPolicyRequest{Id: "nope"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Nil(t, f.scraping.update)
}

func TestDeclinedDomainProceduresRequireBothArguments(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})
	ctx := context.Background()

	_, err := h.SaveDeclinedDomain(ctx,
		connect.NewRequest(&datahubv1.SaveDeclinedDomainRequest{UserId: "u1"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = h.IsDomainDeclined(ctx,
		connect.NewRequest(&datahubv1.IsDomainDeclinedRequest{Domain: "example.com"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

// ---------------------------------------------------------------------------
// §2.O Automatic full-text fetch groundwork
// ---------------------------------------------------------------------------

func TestAutoFulltextProceduresValidateArguments(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})
	ctx := context.Background()

	_, err := h.ListSubscribedUserIDsByFeedLinkID(ctx,
		connect.NewRequest(&datahubv1.ListSubscribedUserIDsByFeedLinkIDRequest{}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = h.CheckArticleExistsByURLForUser(ctx,
		connect.NewRequest(&datahubv1.CheckArticleExistsByURLForUserRequest{Url: "https://example.com"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestCheckArticleExistsByURLForUserReturnsArticleID(t *testing.T) {
	h, _ := newWave3Handler(wave3Fakes{})

	resp, err := h.CheckArticleExistsByURLForUser(context.Background(),
		connect.NewRequest(&datahubv1.CheckArticleExistsByURLForUserRequest{
			Url: "https://example.com/post", UserId: "u1",
		}))
	require.NoError(t, err)
	assert.True(t, resp.Msg.GetExists())
	assert.Equal(t, "a1", resp.Msg.GetArticleId())
}
