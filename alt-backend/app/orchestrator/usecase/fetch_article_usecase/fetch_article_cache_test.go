package fetch_article_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestFetchCompliantArticle_SingleflightDeduplicates(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://zenn.dev/test/articles/duplicate-fetch"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about singleflight deduplication testing.</p></body></html>"
	articleID := "article-sf"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil).Times(2)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).Times(2)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(2)

	fetchStarted := make(chan struct{})
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).DoAndReturn(
		func(ctx context.Context, _ string) (*string, error) {
			close(fetchStarted)
			time.Sleep(100 * time.Millisecond)
			return &rawHTML, nil
		},
	).Times(1)

	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return(articleID, nil).Times(2)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	contents := make([]string, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			content, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)
			errs[idx] = err
			contents[idx] = content
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 2; i++ {
		if errs[i] != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, errs[i])
		}
		if contents[i] == "" {
			t.Errorf("goroutine %d: expected non-empty content", i)
		}
	}
	if contents[0] != contents[1] {
		t.Errorf("expected identical content from both calls")
	}
	ctrl.Finish()
}

func TestFetchCompliantArticleWithRefresh_ForceRefreshSkipsDBCache(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a refreshed article about testing Go code.</p></body></html>"
	articleID := "article-refresh"

	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return(articleID, nil)

	content, retID, _, err := usecase.FetchCompliantArticleWithRefresh(context.Background(), articleURL, userContext, true)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content == "" {
		t.Error("Expected non-empty content")
	}
	if retID != articleID {
		t.Errorf("Expected article ID %s, got %s", articleID, retID)
	}
	time.Sleep(50 * time.Millisecond)
	ctrl.Finish()
}

func TestFetchCompliantArticleWithRefresh_NoForceRefreshUsesDBCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	longContent := strings.Repeat("This is a cached full article with enough content. ", 15)
	existingArticle := &domain.ArticleContent{
		ID:      "existing-article-id",
		Content: longContent,
	}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(existingArticle, nil)
	mockRepo.EXPECT().FetchOgImageURLByArticleID(gomock.Any(), existingArticle.ID).Return("", nil)

	content, retID, _, err := usecase.FetchCompliantArticleWithRefresh(context.Background(), articleURL, userContext, false)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != existingArticle.Content {
		t.Errorf("Expected cached content, got different content")
	}
	if retID != existingArticle.ID {
		t.Errorf("Expected article ID %s, got %s", existingArticle.ID, retID)
	}
}

func TestFetchCompliantArticle_ShortCachedContent_FetchesFromWeb(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/inoreader-short"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	shortCached := &domain.ArticleContent{
		ID:      "short-article-id",
		Content: "This is a short RSS summary from Inoreader. It contains only a few sentences.",
	}

	rawHTML := "<html><body><p>" + strings.Repeat("Full article content fetched from web. ", 30) + "</p></body></html>"
	expectedContentHTML := "<div><p>" + strings.Repeat("Full article content fetched from web. ", 30) + "</p></div>"
	webArticleID := "web-fetched-article-id"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(shortCached, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(webArticleID, nil)

	content, retID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected web-fetched content, got cached content")
	}
	if retID != webArticleID {
		t.Errorf("Expected web article ID %s, got %s", webArticleID, retID)
	}
	time.Sleep(50 * time.Millisecond)
	ctrl.Finish()
}

func TestFetchCompliantArticle_LongCachedContent_ReturnsCached(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/full-article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	longContent := strings.Repeat("This is a full article with rich content from the web. ", 20)
	existingArticle := &domain.ArticleContent{
		ID:      "long-article-id",
		Content: longContent,
	}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(existingArticle, nil)
	mockRepo.EXPECT().FetchOgImageURLByArticleID(gomock.Any(), existingArticle.ID).Return("https://example.com/og.jpg", nil)

	content, retID, ogImage, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if content != longContent {
		t.Error("Expected cached long content to be returned as-is")
	}
	if retID != existingArticle.ID {
		t.Errorf("Expected article ID %s, got %s", existingArticle.ID, retID)
	}
	if ogImage != "https://example.com/og.jpg" {
		t.Errorf("Expected og image URL, got %s", ogImage)
	}
}

func TestFetchCompliantArticle_WebFetchTimeout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/slow-article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).DoAndReturn(
		func(ctx context.Context, _ string) (*string, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("Expected context to have a deadline from web fetch timeout")
				content := "no deadline"
				return &content, nil
			}
			remaining := time.Until(deadline)
			if remaining > 9*time.Second || remaining < 5*time.Second {
				t.Errorf("Expected ~8s timeout, got remaining %v", remaining)
			}
			return nil, context.DeadlineExceeded
		},
	)

	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)
	if err == nil {
		t.Fatal("Expected error from timed-out fetch")
	}
}

func TestFetchCompliantArticle_HitsCacheBelowOldTier1Threshold(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/short-article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	cachedContent := strings.Repeat("a", 200)
	cachedID := "article-short-cache"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(&domain.ArticleContent{
		ID:      cachedID,
		Content: cachedContent,
	}, nil)
	mockRepo.EXPECT().FetchOgImageURLByArticleID(gomock.Any(), cachedID).Return("https://og.example.com/img.png", nil)

	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), gomock.Any()).Times(0)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	content, retID, ogImage, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if content != cachedContent {
		t.Errorf("Expected cached content, got %q", content)
	}
	if retID != cachedID {
		t.Errorf("Expected cached id %q, got %q", cachedID, retID)
	}
	if ogImage != "https://og.example.com/img.png" {
		t.Errorf("Expected cached og image, got %q", ogImage)
	}
}

func TestFetchCompliantArticle_RefetchesWhenCachedContentIsTooShort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/empty-article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(&domain.ArticleContent{
		ID:      "article-too-short",
		Content: strings.Repeat("a", 50),
	}, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

	rawHTML := "<html><body><p>" + strings.Repeat("Article body ", 30) + "</p></body></html>"
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return("article-fresh", nil)

	_, retID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if retID != "article-fresh" {
		t.Errorf("Expected fresh fetch id, got %q", retID)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestFetchCompliantArticle_DefaultExternalFetchTimeoutIs8s(t *testing.T) {
	uc := NewArticleUsecase(nil, nil, nil).(*ArticleUsecaseImpl)
	if uc.externalFetchTimeout != 8*time.Second {
		t.Errorf("Expected default 8s timeout, got %v", uc.externalFetchTimeout)
	}
}

func TestFetchCompliantArticle_SingleflightUserScopeAndCancelIsolation(t *testing.T) {
	articleURLStr := "https://zenn.dev/test/articles/cross-user-singleflight"
	articleURL, _ := url.Parse(articleURLStr)
	rawHTML := "<html><head><title>Shared Title</title><meta property=\"og:image\" content=\"https://example.com/og.png\"></head><body><p>" +
		strings.Repeat("Article content for cross-user singleflight isolation test. ", 5) +
		"</p></body></html>"

	userAID := uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	userBID := uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	userCtxA := &domain.UserContext{
		UserID:    userAID,
		Email:     "usera@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	userCtxB := &domain.UserContext{
		UserID:    userBID,
		Email:     "userb@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	}

	t.Run("saves article per user and returns each user own article id", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
		mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
		mockRepo := mocks.NewMockArticleRepository(ctrl)
		usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

		mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil).Times(2)
		mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).Times(2)

		fetchEntered := make(chan struct{})
		userBAtPolicy := make(chan struct{})

		mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, _ *url.URL, _ string) (bool, error) {
				u, err := domain.GetUserFromContext(ctx)
				if err == nil && u.UserID == userBID {
					close(userBAtPolicy)
				}
				return true, nil
			},
		).Times(2)

		mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).DoAndReturn(
			func(_ context.Context, _ string) (*string, error) {
				close(fetchEntered)
				<-userBAtPolicy
				time.Sleep(20 * time.Millisecond)
				return &rawHTML, nil
			},
		).Times(1)

		var savedMu sync.Mutex
		savedByUser := make(map[uuid.UUID]int)
		headByArticleID := make(map[string]int)

		mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, _, _, _ string) (string, error) {
				u, err := domain.GetUserFromContext(ctx)
				if err != nil {
					t.Errorf("SaveArticle called without valid user context: %v", err)
					return "", err
				}
				savedMu.Lock()
				savedByUser[u.UserID]++
				savedMu.Unlock()
				switch u.UserID {
				case userAID:
					return "article-user-a", nil
				case userBID:
					return "article-user-b", nil
				default:
					return "article-unknown", nil
				}
			},
		).AnyTimes()

		mockRepo.EXPECT().SaveArticleHead(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, articleID, _, _ string) error {
				savedMu.Lock()
				headByArticleID[articleID]++
				savedMu.Unlock()
				return nil
			},
		).AnyTimes()

		ctxA := domain.SetUserContext(context.Background(), userCtxA)
		ctxB := domain.SetUserContext(context.Background(), userCtxB)

		var (
			wg         sync.WaitGroup
			idA, idB   string
			errA, errB error
		)

		wg.Add(2)
		go func() {
			defer wg.Done()
			_, idA, _, errA = usecase.FetchCompliantArticle(ctxA, articleURL, *userCtxA)
		}()
		go func() {
			defer wg.Done()
			<-fetchEntered
			_, idB, _, errB = usecase.FetchCompliantArticle(ctxB, articleURL, *userCtxB)
		}()
		wg.Wait()

		if errA != nil {
			t.Fatalf("user A unexpected error: %v", errA)
		}
		if errB != nil {
			t.Fatalf("user B unexpected error: %v", errB)
		}
		if savedByUser[userAID] != 1 {
			t.Errorf("SaveArticle calls for user A = %d, want 1", savedByUser[userAID])
		}
		if savedByUser[userBID] != 1 {
			t.Errorf("SaveArticle calls for user B = %d, want 1", savedByUser[userBID])
		}
		if idA != "article-user-a" {
			t.Errorf("user A got articleID %q, want %q", idA, "article-user-a")
		}
		if idB != "article-user-b" {
			t.Errorf("user B got articleID %q, want %q", idB, "article-user-b")
		}
		if headByArticleID["article-user-a"] != 1 || headByArticleID["article-user-b"] != 1 {
			t.Errorf("SaveArticleHead calls by articleID = %v, want 1 each for article-user-a and article-user-b", headByArticleID)
		}
	})

	t.Run("cancelling user A ctx does not fail user B request", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
		mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
		mockRepo := mocks.NewMockArticleRepository(ctrl)
		usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

		mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil).Times(2)
		mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).Times(2)

		fetchEntered := make(chan struct{})
		userBAtPolicy := make(chan struct{})
		releaseFetch := make(chan struct{})

		mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, _ *url.URL, _ string) (bool, error) {
				u, err := domain.GetUserFromContext(ctx)
				if err == nil && u.UserID == userBID {
					close(userBAtPolicy)
				}
				return true, nil
			},
		).Times(2)

		mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).DoAndReturn(
			func(fetchCtx context.Context, _ string) (*string, error) {
				close(fetchEntered)
				select {
				case <-releaseFetch:
				case <-fetchCtx.Done():
					return nil, fetchCtx.Err()
				}
				if err := fetchCtx.Err(); err != nil {
					return nil, err
				}
				return &rawHTML, nil
			},
		).Times(1)

		var savedMu sync.Mutex
		savedByUser := make(map[uuid.UUID]int)

		mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, _, _, _ string) (string, error) {
				if err := ctx.Err(); err != nil {
					return "", err
				}
				u, err := domain.GetUserFromContext(ctx)
				if err != nil {
					return "", err
				}
				savedMu.Lock()
				savedByUser[u.UserID]++
				savedMu.Unlock()
				if u.UserID == userBID {
					return "article-user-b", nil
				}
				return "article-user-a", nil
			},
		).AnyTimes()

		mockRepo.EXPECT().SaveArticleHead(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		ctxA, cancelA := context.WithCancel(domain.SetUserContext(context.Background(), userCtxA))
		defer cancelA()
		ctxB := domain.SetUserContext(context.Background(), userCtxB)

		var (
			wg   sync.WaitGroup
			idB  string
			errA error
			errB error
		)

		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _, _, errA = usecase.FetchCompliantArticle(ctxA, articleURL, *userCtxA)
		}()
		go func() {
			defer wg.Done()
			<-fetchEntered
			_, idB, _, errB = usecase.FetchCompliantArticle(ctxB, articleURL, *userCtxB)
		}()

		<-userBAtPolicy
		time.Sleep(20 * time.Millisecond)
		cancelA()
		time.Sleep(10 * time.Millisecond)
		close(releaseFetch)

		wg.Wait()

		if errA == nil {
			t.Errorf("expected user A request to fail after ctxA cancellation, got nil")
		}
		if errB != nil {
			t.Fatalf("user B failed when user A cancelled: %v", errB)
		}
		if idB != "article-user-b" {
			t.Errorf("user B got articleID %q, want %q", idB, "article-user-b")
		}
		if savedByUser[userBID] != 1 {
			t.Errorf("SaveArticle calls for user B = %d, want 1", savedByUser[userBID])
		}
		if savedByUser[userAID] != 0 {
			t.Errorf("SaveArticle calls for cancelled user A = %d, want 0", savedByUser[userAID])
		}
	})
}
