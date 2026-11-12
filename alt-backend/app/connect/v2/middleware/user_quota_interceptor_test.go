package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"alt/config"
	"alt/domain"
	augurv2 "alt/gen/proto/alt/augur/v2"
	"alt/gen/proto/alt/augur/v2/augurv2connect"
	feedsv2 "alt/gen/proto/alt/feeds/v2"
	"alt/gen/proto/alt/feeds/v2/feedsv2connect"
	morningletterv2 "alt/gen/proto/alt/morning_letter/v2"
	"alt/gen/proto/alt/morning_letter/v2/morningletterv2connect"
)

func testUserCtx(id uuid.UUID) context.Context {
	return testUserCtxWithParent(context.Background(), id)
}

func testUserCtxWithParent(parent context.Context, id uuid.UUID) context.Context {
	return domain.SetUserContext(parent, &domain.UserContext{
		UserID:    id,
		TenantID:  id,
		Email:     "reader@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	})
}

func testQuotaConfig(burst, maxInFlight, maxUsers int) config.UserQuotaConfig {
	opCfg := config.OperationQuotaConfig{
		Rate:        burst,
		Burst:       burst,
		Window:      time.Minute,
		MaxInFlight: maxInFlight,
	}
	return config.UserQuotaConfig{
		MaxTrackedUsers:   maxUsers,
		Summarize:         opCfg,
		AugurChat:         opCfg,
		MorningChat:       opCfg,
		MorningRegenerate: opCfg,
	}
}

type fakeFeedSummarizeService struct {
	feedsv2connect.UnimplementedFeedServiceHandler
	summarizeCalls atomic.Int32
	statsCalls     atomic.Int32
	blockCh        chan struct{}
	enteredCh      chan struct{}
}

func (f *fakeFeedSummarizeService) StreamSummarize(
	ctx context.Context,
	req *connect.Request[feedsv2.StreamSummarizeRequest],
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
) error {
	f.summarizeCalls.Add(1)
	if f.enteredCh != nil {
		f.enteredCh <- struct{}{}
	}
	if f.blockCh != nil {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return stream.Send(&feedsv2.StreamSummarizeResponse{
		ArticleId: req.Msg.GetArticleId(),
		IsFinal:   true,
	})
}

func (f *fakeFeedSummarizeService) GetFeedStats(
	_ context.Context,
	_ *connect.Request[feedsv2.GetFeedStatsRequest],
) (*connect.Response[feedsv2.GetFeedStatsResponse], error) {
	f.statsCalls.Add(1)
	return connect.NewResponse(&feedsv2.GetFeedStatsResponse{}), nil
}

type fakeAugurService struct {
	augurv2connect.UnimplementedAugurServiceHandler
	streamCalls atomic.Int32
}

func (f *fakeAugurService) StreamChat(
	_ context.Context,
	_ *connect.Request[augurv2.StreamChatRequest],
	stream *connect.ServerStream[augurv2.StreamChatResponse],
) error {
	f.streamCalls.Add(1)
	return stream.Send(&augurv2.StreamChatResponse{Kind: "done"})
}

type fakeMorningService struct {
	morningletterv2connect.UnimplementedMorningLetterServiceHandler
	morningletterv2connect.UnimplementedMorningLetterReadServiceHandler
	chatCalls  atomic.Int32
	regenCalls atomic.Int32
}

func (f *fakeMorningService) StreamChat(
	_ context.Context,
	_ *connect.Request[morningletterv2.StreamChatRequest],
	stream *connect.ServerStream[morningletterv2.StreamChatResponse],
) error {
	f.chatCalls.Add(1)
	return stream.Send(&morningletterv2.StreamChatResponse{Kind: "done"})
}

func (f *fakeMorningService) RegenerateLatest(
	_ context.Context,
	_ *connect.Request[morningletterv2.RegenerateLatestRequest],
) (*connect.Response[morningletterv2.RegenerateLatestResponse], error) {
	f.regenCalls.Add(1)
	return connect.NewResponse(&morningletterv2.RegenerateLatestResponse{Regenerated: true}), nil
}

type ctxInjectMiddleware struct {
	mu  sync.Mutex
	ctx func(r *http.Request) context.Context
}

func (m *ctxInjectMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		fn := m.ctx
		m.mu.Unlock()
		if fn != nil {
			r = r.WithContext(fn(r))
		}
		next.ServeHTTP(w, r)
	})
}

func TestUserQuotaInterceptor_PerUserTokenBucketAndIsolation(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(2, 4, 100))
	require.NoError(t, err)

	userA := uuid.New()
	userB := uuid.New()

	inject := &ctxInjectMiddleware{
		ctx: func(r *http.Request) context.Context {
			if r.Header.Get("X-Test-User") == "B" {
				return testUserCtx(userB)
			}
			return testUserCtx(userA)
		},
	}

	feedSvc := &fakeFeedSummarizeService{}
	augurSvc := &fakeAugurService{}
	morningSvc := &fakeMorningService{}

	opts := connect.WithInterceptors(NewUserQuotaInterceptor(limiter).Interceptor())
	mux := http.NewServeMux()
	p1, h1 := feedsv2connect.NewFeedServiceHandler(feedSvc, opts)
	p2, h2 := augurv2connect.NewAugurServiceHandler(augurSvc, opts)
	p3, h3 := morningletterv2connect.NewMorningLetterServiceHandler(morningSvc, opts)
	p4, h4 := morningletterv2connect.NewMorningLetterReadServiceHandler(morningSvc, opts)
	mux.Handle(p1, h1)
	mux.Handle(p2, h2)
	mux.Handle(p3, h3)
	mux.Handle(p4, h4)

	srv := httptest.NewServer(inject.wrap(mux))
	defer srv.Close()

	feedClient := feedsv2connect.NewFeedServiceClient(srv.Client(), srv.URL)
	forceRefresh := true
	articleID := "art-1"

	for i := 0; i < 2; i++ {
		req := connect.NewRequest(&feedsv2.StreamSummarizeRequest{
			ArticleId:    &articleID,
			ForceRefresh: &forceRefresh,
		})
		req.Header().Set("X-Alt-User-Id", userB.String())
		stream, callErr := feedClient.StreamSummarize(context.Background(), req)
		require.NoError(t, callErr)
		for stream.Receive() {
		}
		require.NoError(t, stream.Err())
		_ = stream.Close()
	}
	assert.Equal(t, int32(2), feedSvc.summarizeCalls.Load())

	// 3rd call (N+1) for userA inside the window must be rejected without reaching handler,
	// even when request headers attempt to spoof userB.
	overReq := connect.NewRequest(&feedsv2.StreamSummarizeRequest{
		ArticleId:    &articleID,
		ForceRefresh: &forceRefresh,
	})
	overReq.Header().Set("X-Alt-User-Id", userB.String())
	overStream, callErr := feedClient.StreamSummarize(context.Background(), overReq)
	if callErr == nil {
		for overStream.Receive() {
		}
		callErr = overStream.Err()
		_ = overStream.Close()
	}
	require.Error(t, callErr)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(callErr))
	assert.Equal(t, int32(2), feedSvc.summarizeCalls.Load())

	// User B's call passes independently.
	otherUserReq := connect.NewRequest(&feedsv2.StreamSummarizeRequest{
		ArticleId:    &articleID,
		ForceRefresh: &forceRefresh,
	})
	otherUserReq.Header().Set("X-Test-User", "B")
	otherStream, callErr := feedClient.StreamSummarize(context.Background(), otherUserReq)
	require.NoError(t, callErr)
	for otherStream.Receive() {
	}
	require.NoError(t, otherStream.Err())
	_ = otherStream.Close()
	assert.Equal(t, int32(3), feedSvc.summarizeCalls.Load())

	// Non-model procedure on FeedService is untouched even when userA's summarize bucket is empty.
	_, statsErr := feedClient.GetFeedStats(context.Background(), connect.NewRequest(&feedsv2.GetFeedStatsRequest{}))
	require.NoError(t, statsErr)
	assert.Equal(t, int32(1), feedSvc.statsCalls.Load())

	// Verify Augur StreamChat, MorningLetter StreamChat, and MorningLetter RegenerateLatest are each enforced.
	augurClient := augurv2connect.NewAugurServiceClient(srv.Client(), srv.URL)
	for i := 0; i < 2; i++ {
		st, aErr := augurClient.StreamChat(context.Background(), connect.NewRequest(&augurv2.StreamChatRequest{}))
		require.NoError(t, aErr)
		for st.Receive() {
		}
		require.NoError(t, st.Err())
		_ = st.Close()
	}
	st3, aErr := augurClient.StreamChat(context.Background(), connect.NewRequest(&augurv2.StreamChatRequest{}))
	if aErr == nil {
		for st3.Receive() {
		}
		aErr = st3.Err()
		_ = st3.Close()
	}
	require.Error(t, aErr)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(aErr))
	assert.Equal(t, int32(2), augurSvc.streamCalls.Load())

	mlClient := morningletterv2connect.NewMorningLetterServiceClient(srv.Client(), srv.URL)
	for i := 0; i < 2; i++ {
		st, mErr := mlClient.StreamChat(context.Background(), connect.NewRequest(&morningletterv2.StreamChatRequest{}))
		require.NoError(t, mErr)
		for st.Receive() {
		}
		require.NoError(t, st.Err())
		_ = st.Close()
	}
	mlOver, mErr := mlClient.StreamChat(context.Background(), connect.NewRequest(&morningletterv2.StreamChatRequest{}))
	if mErr == nil {
		for mlOver.Receive() {
		}
		mErr = mlOver.Err()
		_ = mlOver.Close()
	}
	require.Error(t, mErr)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(mErr))
	assert.Equal(t, int32(2), morningSvc.chatCalls.Load())

	mlReadClient := morningletterv2connect.NewMorningLetterReadServiceClient(srv.Client(), srv.URL)
	for i := 0; i < 2; i++ {
		_, rErr := mlReadClient.RegenerateLatest(context.Background(), connect.NewRequest(&morningletterv2.RegenerateLatestRequest{}))
		require.NoError(t, rErr)
	}
	_, rErr := mlReadClient.RegenerateLatest(context.Background(), connect.NewRequest(&morningletterv2.RegenerateLatestRequest{}))
	require.Error(t, rErr)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(rErr))
	assert.Equal(t, int32(2), morningSvc.regenCalls.Load())
}

func TestUserQuotaInterceptor_PerUserConcurrencyCapIsolatesUsersAndReleases(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(10, 2, 100))
	require.NoError(t, err)

	userABlockCh := make(chan struct{})
	enteredCh := make(chan uuid.UUID, 8)
	var summarizeCalls atomic.Int32

	feedSvc := &perUserBlockingFeedService{
		calls:     &summarizeCalls,
		enteredCh: enteredCh,
		blockUser: uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
		blockCh:   userABlockCh,
	}
	userA := feedSvc.blockUser
	userB := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")

	inject := &ctxInjectMiddleware{
		ctx: func(r *http.Request) context.Context {
			uid, parseErr := uuid.Parse(r.Header.Get("X-Test-UID"))
			if parseErr != nil {
				return context.Background()
			}
			return testUserCtx(uid)
		},
	}

	opts := connect.WithInterceptors(NewUserQuotaInterceptor(limiter).Interceptor())
	mux := http.NewServeMux()
	p, h := feedsv2connect.NewFeedServiceHandler(feedSvc, opts)
	mux.Handle(p, h)

	srv := httptest.NewServer(inject.wrap(mux))
	defer srv.Close()

	client := feedsv2connect.NewFeedServiceClient(srv.Client(), srv.URL)

	var wg sync.WaitGroup
	startStream := func(uid uuid.UUID) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := connect.NewRequest(&feedsv2.StreamSummarizeRequest{})
			req.Header().Set("X-Test-UID", uid.String())
			st, sErr := client.StreamSummarize(context.Background(), req)
			if sErr == nil {
				for st.Receive() {
				}
				_ = st.Close()
			}
		}()
	}

	startStream(userA)
	startStream(userA)

	assert.Equal(t, userA, <-enteredCh)
	assert.Equal(t, userA, <-enteredCh)

	// User A's 3rd concurrent stream exceeds user A's per-user cap of 2 and is rejected.
	overReq := connect.NewRequest(&feedsv2.StreamSummarizeRequest{})
	overReq.Header().Set("X-Test-UID", userA.String())
	overStream, overErr := client.StreamSummarize(context.Background(), overReq)
	if overErr == nil {
		for overStream.Receive() {
		}
		overErr = overStream.Err()
		_ = overStream.Close()
	}
	require.Error(t, overErr)
	assert.Equal(t, connect.CodeResourceExhausted, connect.CodeOf(overErr))
	assert.Equal(t, int32(2), summarizeCalls.Load())

	// User B is admitted concurrently while User A holds 2 in-flight slots.
	userBReq := connect.NewRequest(&feedsv2.StreamSummarizeRequest{})
	userBReq.Header().Set("X-Test-UID", userB.String())
	userBStream, userBErr := client.StreamSummarize(context.Background(), userBReq)
	require.NoError(t, userBErr)
	for userBStream.Receive() {
	}
	require.NoError(t, userBStream.Err())
	_ = userBStream.Close()
	assert.Equal(t, userB, <-enteredCh)
	assert.Equal(t, int32(3), summarizeCalls.Load())

	// Release User A's active streams and verify User A is admitted again afterwards.
	close(userABlockCh)
	wg.Wait()

	afterReq := connect.NewRequest(&feedsv2.StreamSummarizeRequest{})
	afterReq.Header().Set("X-Test-UID", userA.String())
	afterStream, afterErr := client.StreamSummarize(context.Background(), afterReq)
	require.NoError(t, afterErr)
	for afterStream.Receive() {
	}
	require.NoError(t, afterStream.Err())
	_ = afterStream.Close()
	assert.Equal(t, int32(4), summarizeCalls.Load())
}

type perUserBlockingFeedService struct {
	feedsv2connect.UnimplementedFeedServiceHandler
	calls     *atomic.Int32
	enteredCh chan uuid.UUID
	blockUser uuid.UUID
	blockCh   chan struct{}
}

func (f *perUserBlockingFeedService) StreamSummarize(
	ctx context.Context,
	req *connect.Request[feedsv2.StreamSummarizeRequest],
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
) error {
	f.calls.Add(1)
	u, _ := domain.GetUserFromContext(ctx)
	var uid uuid.UUID
	if u != nil {
		uid = u.UserID
	}
	if f.enteredCh != nil {
		f.enteredCh <- uid
	}
	if f.blockCh != nil && uid == f.blockUser {
		select {
		case <-f.blockCh:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return stream.Send(&feedsv2.StreamSummarizeResponse{
		ArticleId: req.Msg.GetArticleId(),
		IsFinal:   true,
	})
}

func TestUserQuotaInterceptor_RejectsUnauthenticated(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(5, 2, 100))
	require.NoError(t, err)

	feedSvc := &fakeFeedSummarizeService{}
	morningSvc := &fakeMorningService{}
	opts := connect.WithInterceptors(NewUserQuotaInterceptor(limiter).Interceptor())

	mux := http.NewServeMux()
	p1, h1 := feedsv2connect.NewFeedServiceHandler(feedSvc, opts)
	p2, h2 := morningletterv2connect.NewMorningLetterReadServiceHandler(morningSvc, opts)
	mux.Handle(p1, h1)
	mux.Handle(p2, h2)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	feedClient := feedsv2connect.NewFeedServiceClient(srv.Client(), srv.URL)
	st, sErr := feedClient.StreamSummarize(context.Background(), connect.NewRequest(&feedsv2.StreamSummarizeRequest{}))
	if sErr == nil {
		for st.Receive() {
		}
		sErr = st.Err()
		_ = st.Close()
	}
	require.Error(t, sErr)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(sErr))
	assert.Equal(t, int32(0), feedSvc.summarizeCalls.Load())

	mlReadClient := morningletterv2connect.NewMorningLetterReadServiceClient(srv.Client(), srv.URL)
	_, rErr := mlReadClient.RegenerateLatest(context.Background(), connect.NewRequest(&morningletterv2.RegenerateLatestRequest{}))
	require.Error(t, rErr)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(rErr))
	assert.Equal(t, int32(0), morningSvc.regenCalls.Load())
}

func TestEchoUserQuotaMiddleware_RateConcurrencyAndUnauthenticated(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(1, 1, 100))
	require.NoError(t, err)

	e := echo.New()
	var handlerCalls atomic.Int32
	mw := EchoUserQuotaMiddleware(limiter, OpAugurChat)
	e.POST("/sse/v1/rag/answer", func(c echo.Context) error {
		handlerCalls.Add(1)
		return c.String(http.StatusOK, "ok")
	}, mw)

	// Unauthenticated request is rejected without reaching handler.
	unauthReq := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader("{}"))
	unauthRec := httptest.NewRecorder()
	e.ServeHTTP(unauthRec, unauthReq)
	assert.Equal(t, http.StatusUnauthorized, unauthRec.Code)
	assert.Equal(t, int32(0), handlerCalls.Load())

	// Authenticated userA: 1st passes, 2nd returns 429 with Retry-After.
	userA := uuid.New()
	req1 := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader("{}"))
	req1 = req1.WithContext(testUserCtx(userA))
	rec1 := httptest.NewRecorder()
	e.ServeHTTP(rec1, req1)
	assert.Equal(t, http.StatusOK, rec1.Code)
	assert.Equal(t, int32(1), handlerCalls.Load())

	req2 := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader("{}"))
	req2 = req2.WithContext(testUserCtx(userA))
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusTooManyRequests, rec2.Code)
	retryAfter := rec2.Header().Get("Retry-After")
	require.NotEmpty(t, retryAfter)
	sec, convErr := strconv.Atoi(retryAfter)
	require.NoError(t, convErr)
	assert.GreaterOrEqual(t, sec, 1)
	assert.Equal(t, int32(1), handlerCalls.Load())

	// UserB still passes.
	userB := uuid.New()
	reqB := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader("{}"))
	reqB = reqB.WithContext(testUserCtx(userB))
	recB := httptest.NewRecorder()
	e.ServeHTTP(recB, reqB)
	assert.Equal(t, http.StatusOK, recB.Code)
	assert.Equal(t, int32(2), handlerCalls.Load())
}

func TestUserQuotaLimiter_BoundedMemoryEvictsLRU(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(5, 5, 3))
	require.NoError(t, err)

	for i := 0; i < 5; i++ {
		rel, acqErr := limiter.Acquire(testUserCtx(uuid.New()), OpSummarize)
		require.NoError(t, acqErr)
		rel()
	}

	assert.Equal(t, 3, limiter.TrackedUsers(OpSummarize))
}

func TestUserQuotaLimiter_InFlightEntryNeverEvicted(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(10, 1, 2))
	require.NoError(t, err)

	userA := uuid.New()
	relA, err := limiter.Acquire(testUserCtx(userA), OpSummarize)
	require.NoError(t, err)

	// Push 4 idle users through the bounded map (capacity 2) while userA is in-flight.
	for i := 0; i < 4; i++ {
		relOther, otherErr := limiter.Acquire(testUserCtx(uuid.New()), OpSummarize)
		require.NoError(t, otherErr)
		relOther()
	}

	// userA's entry must still be present with inFlight == 1, so a 2nd concurrent acquire fails.
	_, secondErr := limiter.Acquire(testUserCtx(userA), OpSummarize)
	require.Error(t, secondErr)

	// Calling release multiple times is idempotent and restores userA's slot exactly once.
	relA()
	relA()

	relAAfter, afterErr := limiter.Acquire(testUserCtx(userA), OpSummarize)
	require.NoError(t, afterErr)
	_, thirdErr := limiter.Acquire(testUserCtx(userA), OpSummarize)
	require.Error(t, thirdErr)
	relAAfter()
}

func TestUserQuotaInterceptor_ReleasesOnPanicAndStreamCancel(t *testing.T) {
	limiter, err := NewUserQuotaLimiter(testQuotaConfig(10, 1, 16))
	require.NoError(t, err)

	userA := uuid.New()

	// 1. Panic inside Echo handler releases the per-user in-flight slot.
	e := echo.New()
	mw := EchoUserQuotaMiddleware(limiter, OpAugurChat)
	shouldPanic := true
	e.POST("/sse/v1/rag/answer", func(c echo.Context) error {
		if shouldPanic {
			panic("boom")
		}
		return c.String(http.StatusOK, "ok")
	}, mw)

	assert.Panics(t, func() {
		req := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader("{}"))
		req = req.WithContext(testUserCtx(userA))
		e.ServeHTTP(httptest.NewRecorder(), req)
	})

	shouldPanic = false
	reqAfterPanic := httptest.NewRequest(http.MethodPost, "/sse/v1/rag/answer", strings.NewReader("{}"))
	reqAfterPanic = reqAfterPanic.WithContext(testUserCtx(userA))
	recAfterPanic := httptest.NewRecorder()
	e.ServeHTTP(recAfterPanic, reqAfterPanic)
	assert.Equal(t, http.StatusOK, recAfterPanic.Code)

	stopCh := make(chan struct{})
	var closeStop sync.Once
	defer closeStop.Do(func() { close(stopCh) })
	enteredCh := make(chan struct{}, 4)
	exitedCh := make(chan struct{}, 4)
	feedSvc := &cancelTestFeedService{
		stopCh:    stopCh,
		enteredCh: enteredCh,
		exitedCh:  exitedCh,
	}
	inject := &ctxInjectMiddleware{
		ctx: func(r *http.Request) context.Context { return testUserCtxWithParent(r.Context(), userA) },
	}
	opts := connect.WithInterceptors(NewUserQuotaInterceptor(limiter).Interceptor())
	mux := http.NewServeMux()
	p, h := feedsv2connect.NewFeedServiceHandler(feedSvc, opts)
	mux.Handle(p, h)
	srv := httptest.NewUnstartedServer(inject.wrap(mux))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()

	client := feedsv2connect.NewFeedServiceClient(srv.Client(), srv.URL)
	cancelCtx, cancel := context.WithCancel(context.Background())
	st, sErr := client.StreamSummarize(cancelCtx, connect.NewRequest(&feedsv2.StreamSummarizeRequest{}))
	require.NoError(t, sErr)
	require.True(t, st.Receive())
	<-enteredCh

	cancel()
	_ = st.Close()
	<-exitedCh

	closeStop.Do(func() { close(stopCh) })
	stAfter, afterErr := client.StreamSummarize(context.Background(), connect.NewRequest(&feedsv2.StreamSummarizeRequest{}))
	require.NoError(t, afterErr)
	for stAfter.Receive() {
	}
	require.NoError(t, stAfter.Err())
	_ = stAfter.Close()
}

type cancelTestFeedService struct {
	feedsv2connect.UnimplementedFeedServiceHandler
	stopCh    <-chan struct{}
	enteredCh chan<- struct{}
	exitedCh  chan<- struct{}
}

func (f *cancelTestFeedService) StreamSummarize(
	ctx context.Context,
	_ *connect.Request[feedsv2.StreamSummarizeRequest],
	stream *connect.ServerStream[feedsv2.StreamSummarizeResponse],
) error {
	if err := stream.Send(&feedsv2.StreamSummarizeResponse{ArticleId: "chunk-1"}); err != nil {
		return err
	}
	select {
	case f.enteredCh <- struct{}{}:
	default:
	}
	defer func() {
		select {
		case f.exitedCh <- struct{}{}:
		default:
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-f.stopCh:
		return stream.Send(&feedsv2.StreamSummarizeResponse{ArticleId: "chunk-final", IsFinal: true})
	}
}

func TestUserQuota_NilLimiterPanicsInBusinessPath(t *testing.T) {
	assert.Panics(t, func() {
		mw := EchoUserQuotaMiddleware(nil, OpSummarize)
		e := echo.New()
		req := httptest.NewRequest(http.MethodPost, "/v1/feeds/summarize/stream", strings.NewReader("{}"))
		req = req.WithContext(testUserCtx(uuid.New()))
		c := e.NewContext(req, httptest.NewRecorder())
		_ = mw(func(c echo.Context) error { return nil })(c)
	})
}
