package articles

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	articlesv2 "alt/gen/proto/alt/articles/v2"

	"alt/config"
	"alt/domain"
	"alt/utils/logger"
)

// ---------------------------------------------------------------------------
// doubles
// ---------------------------------------------------------------------------

// recordingPrefetchUsecase records every URL the prefetch path fetched, in
// order. It stands in for the third-class ArticleUsecase, whose
// FetchCompliantArticle is the call that asks the scraping-policy gate and
// therefore reserves the publisher's crawl-delay window.
type recordingPrefetchUsecase struct {
	mu     sync.Mutex
	calls  []string
	err    error
	block  chan struct{}
	called chan string
}

func newRecordingPrefetchUsecase() *recordingPrefetchUsecase {
	return &recordingPrefetchUsecase{called: make(chan string, 32)}
}

func (r *recordingPrefetchUsecase) Execute(_ context.Context, _ string) (*string, error) {
	body := "unused"
	return &body, nil
}

func (r *recordingPrefetchUsecase) FetchCompliantArticle(ctx context.Context, articleURL *url.URL, user domain.UserContext) (string, string, string, error) {
	return r.FetchCompliantArticleWithRefresh(ctx, articleURL, user, false)
}

func (r *recordingPrefetchUsecase) FetchCompliantArticleWithRefresh(_ context.Context, articleURL *url.URL, _ domain.UserContext, _ bool) (string, string, string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, articleURL.String())
	r.mu.Unlock()
	r.called <- articleURL.String()
	if r.block != nil {
		<-r.block
	}
	return "body", "art-1", "", r.err
}

func (r *recordingPrefetchUsecase) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// recordingHostSlots stands in for the shared HostRateLimiter. It records the
// order in which turns were requested so a test can assert that the slot is
// taken before the usecase (and therefore before the policy gate) is reached.
type recordingHostSlots struct {
	mu     sync.Mutex
	waits  []string
	refuse bool
	waited chan string
}

func newRecordingHostSlots() *recordingHostSlots {
	return &recordingHostSlots{waited: make(chan string, 32)}
}

func (s *recordingHostSlots) WaitForHost(_ context.Context, rawURL string) error {
	s.mu.Lock()
	s.waits = append(s.waits, rawURL)
	s.mu.Unlock()
	s.waited <- rawURL
	if s.refuse {
		return errors.New("host busy")
	}
	return nil
}

func (s *recordingHostSlots) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.waits...)
}

// stubStoredArticles answers the "is it already stored?" probe.
type stubStoredArticles struct {
	stored map[string]*domain.ArticleContent
	err    error
	mu     sync.Mutex
	asked  []string
}

func (s *stubStoredArticles) FetchArticleByURL(_ context.Context, articleURL string) (*domain.ArticleContent, error) {
	s.mu.Lock()
	s.asked = append(s.asked, articleURL)
	s.mu.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return s.stored[articleURL], nil
}

func prefetchTestUser() *domain.UserContext {
	return &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		SessionID: "session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

func prefetchTestContext() context.Context {
	return domain.SetUserContext(context.Background(), prefetchTestUser())
}

type prefetchHarness struct {
	handler *Handler
	usecase *recordingPrefetchUsecase
	slots   *recordingHostSlots
	probe   *stubStoredArticles
}

// prefetchTestHosts are the hosts these tests use. They are declared to
// FEED_ALLOWED_HOSTS so url_validator answers from the allowlist instead of
// resolving them: a unit test that needs DNS is a unit test that fails in a
// sandbox, and the behaviour under test is the ordering of the gates, not the
// resolver.
var prefetchTestHosts = []string{
	"example.com", "example.org",
	"h1.example.com", "h2.example.com", "h3.example.com", "h4.example.com",
	"h5.example.com", "h6.example.com", "h7.example.com", "h8.example.com",
	"h9.example.com", "h10.example.com", "h11.example.com",
}

func newPrefetchHarness(t *testing.T) *prefetchHarness {
	t.Helper()
	logger.InitLogger()
	t.Setenv("FEED_ALLOWED_HOSTS", strings.Join(prefetchTestHosts, ","))

	uc := newRecordingPrefetchUsecase()
	slots := newRecordingHostSlots()
	probe := &stubStoredArticles{stored: map[string]*domain.ArticleContent{}}

	h := NewHandler(ArticleHandlerDeps{
		PrefetchArticle:   uc,
		PrefetchHostSlots: slots,
		PrefetchProbe:     probe,
		PrefetchWiring: ArticlePrefetchWiring{
			Enabled:  true,
			SlotWait: 50 * time.Millisecond,
		},
	}, &config.Config{}, logger.Logger)

	return &prefetchHarness{handler: h, usecase: uc, slots: slots, probe: probe}
}

func prefetch(t *testing.T, h *Handler, ctx context.Context, urls ...string) *articlesv2.BatchPrefetchArticleContentResponse {
	t.Helper()
	resp, err := h.BatchPrefetchArticleContent(ctx, connect.NewRequest(&articlesv2.BatchPrefetchArticleContentRequest{
		Urls: urls,
	}))
	require.NoError(t, err)
	return resp.Msg
}

// ---------------------------------------------------------------------------
// tests
// ---------------------------------------------------------------------------

// A capability that is switched off must say so by name. Returning an empty
// success would make "prefetch is disabled" and "prefetch warmed nothing this
// time" the same observation from the client's side, which is the shape
// ADR-000966 replaced with FAILED_PRECONDITION.
func TestBatchPrefetchArticleContent_DisabledIsNamed(t *testing.T) {
	logger.InitLogger()

	h := NewHandler(ArticleHandlerDeps{
		PrefetchWiring: ArticlePrefetchWiring{
			Enabled:        false,
			DisabledReason: "RATE_LIMIT_PREFETCH_SLOT_WAIT=off",
		},
	}, &config.Config{}, logger.Logger)

	_, err := h.BatchPrefetchArticleContent(prefetchTestContext(),
		connect.NewRequest(&articlesv2.BatchPrefetchArticleContentRequest{
			Urls: []string{"https://example.com/a"},
		}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeFailedPrecondition, connectErr.Code())
	require.Contains(t, connectErr.Message(), "RATE_LIMIT_PREFETCH_SLOT_WAIT=off")

	// ADR-000963: this refusal is ours, not a publisher's. Stamping it host
	// would charge a third party for our own configuration.
	require.Empty(t, connectErr.Meta().Get(FailureScopeHeader))
}

// Declaring the capability enabled while leaving it unwired is a DI bug, and
// it must not be reachable at request time. Rule 8 forbids discovering it as
// a nil check inside business code; the composition root is where it belongs.
func TestNewHandler_EnabledPrefetchMustBeWired(t *testing.T) {
	logger.InitLogger()

	require.Panics(t, func() {
		NewHandler(ArticleHandlerDeps{
			PrefetchWiring: ArticlePrefetchWiring{Enabled: true, SlotWait: time.Second},
		}, &config.Config{}, logger.Logger)
	})
}

// A prefetch budget of zero means "queue for this host for as long as the
// context allows" — the setting that lets background warming sit in front of
// a user who is waiting. It is refused rather than silently accepted.
func TestNewHandler_EnabledPrefetchRejectsUnboundedSlotWait(t *testing.T) {
	logger.InitLogger()

	uc := newRecordingPrefetchUsecase()
	require.Panics(t, func() {
		NewHandler(ArticleHandlerDeps{
			PrefetchArticle:   uc,
			PrefetchHostSlots: newRecordingHostSlots(),
			PrefetchProbe:     &stubStoredArticles{stored: map[string]*domain.ArticleContent{}},
			PrefetchWiring:    ArticlePrefetchWiring{Enabled: true, SlotWait: 0},
		}, &config.Config{}, logger.Logger)
	})
}

// The crawl-delay window is *reserved* by the act of asking
// (scraping_policy_gateway: a granted CanFetchArticle stamps
// lastRequestTime), so a second URL on the same host in the same batch could
// only ever be refused — after having spent the host's turn. One entry per
// host is therefore a correctness rule, not a throughput tweak.
func TestBatchPrefetchArticleContent_AtMostOneURLPerHost(t *testing.T) {
	hz := newPrefetchHarness(t)

	resp := prefetch(t, hz.handler, prefetchTestContext(),
		"https://example.com/one",
		"https://example.com/two",
		"https://example.org/three",
	)

	require.Equal(t, int32(2), resp.AcceptedCount)
	require.Equal(t, int32(1), resp.SkippedSameHostCount)

	waitForCalls(t, hz.usecase, 2)
	calls := hz.usecase.snapshot()
	require.Len(t, calls, 2)
	require.ElementsMatch(t, []string{"https://example.com/one", "https://example.org/three"}, calls)
}

// The ordering this test pins is the whole safety argument. The host slot is
// authoritative and consumes nothing when it is refused; the policy gate
// consumes the publisher's crawl-delay window the moment it grants. Asking the
// gate first and then shedding would burn a window the user's own read could
// have used.
func TestBatchPrefetchArticleContent_TakesHostSlotBeforeAskingPolicyGate(t *testing.T) {
	hz := newPrefetchHarness(t)
	hz.slots.refuse = true

	resp := prefetch(t, hz.handler, prefetchTestContext(), "https://example.com/one")
	require.Equal(t, int32(1), resp.AcceptedCount)

	select {
	case <-hz.slots.waited:
	case <-time.After(2 * time.Second):
		t.Fatal("prefetch never asked for a host slot")
	}

	// Give the warm goroutine room to do the wrong thing if it is going to.
	require.Never(t, func() bool { return len(hz.usecase.snapshot()) > 0 },
		300*time.Millisecond, 20*time.Millisecond,
		"a refused host slot must end the warm before the policy gate is asked")
}

// Shedding happens before anything is claimed: no host slot, no policy gate,
// no publisher. A dropped warm must cost strictly less than a kept one — that
// is the whole reason shedding is preferable to queueing here.
func TestBatchPrefetchArticleContent_ShedsWithoutTouchingAnyGate(t *testing.T) {
	hz := newPrefetchHarness(t)
	hz.usecase.block = make(chan struct{})
	t.Cleanup(func() { close(hz.usecase.block) })

	// Park five warms inside the usecase, one per host.
	first := prefetch(t, hz.handler, prefetchTestContext(),
		"https://h1.example.com/x", "https://h2.example.com/x", "https://h3.example.com/x",
		"https://h4.example.com/x", "https://h5.example.com/x")
	require.Equal(t, int32(5), first.AcceptedCount)
	waitForCalls(t, hz.usecase, 5)

	// The pool holds maxConcurrentContentWarms; five are gone, so the next
	// five URLs can only claim what is left and the rest must be shed.
	second := prefetch(t, hz.handler, prefetchTestContext(),
		"https://h6.example.com/x", "https://h7.example.com/x", "https://h8.example.com/x",
		"https://h9.example.com/x", "https://h10.example.com/x")

	remaining := int32(maxConcurrentContentWarms - 5)
	require.Equal(t, remaining, second.AcceptedCount)
	require.Equal(t, int32(5)-remaining, second.ShedCount)

	// The shed URLs never reached the host slot, and therefore never reached
	// the crawl-delay gate behind it.
	require.Never(t, func() bool { return len(hz.slots.snapshot()) > maxConcurrentContentWarms },
		300*time.Millisecond, 20*time.Millisecond,
		"a shed warm must not ask for a host slot")
}

// SSRF validation is the same gate FetchArticleContent applies. The batch path
// does not get a cheaper one because it is a batch.
func TestBatchPrefetchArticleContent_RejectsDisallowedURLs(t *testing.T) {
	hz := newPrefetchHarness(t)

	resp := prefetch(t, hz.handler, prefetchTestContext(),
		"file:///etc/passwd",
		"http://169.254.169.254/latest/meta-data/",
		"::not a url::",
	)

	require.Equal(t, int32(0), resp.AcceptedCount)
	require.Equal(t, int32(3), resp.RejectedCount)
	require.Never(t, func() bool { return len(hz.slots.snapshot()) > 0 },
		200*time.Millisecond, 20*time.Millisecond,
		"a rejected URL must not reach the host slot")
}

// An article whose body is already stored has nothing to warm. Asking the host
// for a turn anyway would spend the publisher's interval to learn what the
// database already knew — and deny it to the user's next real read.
func TestBatchPrefetchArticleContent_SkipsAlreadyStoredArticles(t *testing.T) {
	hz := newPrefetchHarness(t)
	hz.probe.stored["https://example.com/one"] = &domain.ArticleContent{
		ID:      "art-1",
		Content: string(make([]byte, 4096)),
	}

	resp := prefetch(t, hz.handler, prefetchTestContext(), "https://example.com/one")
	require.Equal(t, int32(1), resp.AcceptedCount)

	require.Never(t, func() bool { return len(hz.slots.snapshot()) > 0 },
		300*time.Millisecond, 20*time.Millisecond,
		"an already-stored article must not claim a host slot")
}

// One attempt per URL per call. Browser, BFF, alt-backend and publisher are
// four hops; a retry at any of them multiplies against the others.
func TestBatchPrefetchArticleContent_DoesNotRetry(t *testing.T) {
	hz := newPrefetchHarness(t)
	hz.usecase.err = errors.New("publisher said no")

	prefetch(t, hz.handler, prefetchTestContext(), "https://example.com/one")
	waitForCalls(t, hz.usecase, 1)

	require.Never(t, func() bool { return len(hz.usecase.snapshot()) > 1 },
		400*time.Millisecond, 25*time.Millisecond,
		"a failed warm must not be retried server-side")
}

func TestBatchPrefetchArticleContent_CapsTheBatch(t *testing.T) {
	hz := newPrefetchHarness(t)

	urls := []string{
		"https://h1.example.com/1", "https://h2.example.com/2", "https://h3.example.com/3",
		"https://h4.example.com/4", "https://h5.example.com/5", "https://h6.example.com/6",
		"https://h7.example.com/7",
	}
	resp := prefetch(t, hz.handler, prefetchTestContext(), urls...)

	require.Equal(t, int32(maxPrefetchArticleURLs), resp.AcceptedCount)
}

func TestBatchPrefetchArticleContent_RequiresAuthentication(t *testing.T) {
	hz := newPrefetchHarness(t)

	_, err := hz.handler.BatchPrefetchArticleContent(context.Background(),
		connect.NewRequest(&articlesv2.BatchPrefetchArticleContentRequest{
			Urls: []string{"https://example.com/a"},
		}))

	require.Error(t, err)
	var connectErr *connect.Error
	require.ErrorAs(t, err, &connectErr)
	require.Equal(t, connect.CodeUnauthenticated, connectErr.Code())
}

func TestBatchPrefetchArticleContent_EmptyBatchIsNotAnError(t *testing.T) {
	hz := newPrefetchHarness(t)

	resp := prefetch(t, hz.handler, prefetchTestContext())
	require.Equal(t, int32(0), resp.AcceptedCount)
}

func waitForCalls(t *testing.T, uc *recordingPrefetchUsecase, n int) {
	t.Helper()
	require.Eventually(t, func() bool { return len(uc.snapshot()) >= n },
		3*time.Second, 10*time.Millisecond,
		"expected at least %d warm(s) to reach the usecase", n)
}
