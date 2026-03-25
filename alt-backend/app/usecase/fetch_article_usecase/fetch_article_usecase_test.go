package fetch_article_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/port/rag_integration_port"
	"alt/utils/logger"
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestMain(m *testing.M) {
	logger.InitLogger()
	os.Exit(m.Run())
}

func TestFetchArticleUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	// テストデータ
	articleURL := "https://example.com/article"
	// MinArticleLength (100) を満たす長いテキスト
	expectedContent := "test article content needs to be long enough to pass the minimum length check in the cleaner utility. This text is intentionally made longer to ensure it exceeds the 100 character limit required by the ExtractArticleText function."

	// モックの期待値設定
	mockArticleFetcher.EXPECT().
		FetchArticleContents(context.Background(), articleURL).
		Return(&expectedContent, nil).
		Times(1)

	// テスト実行
	result, err := usecase.Execute(context.Background(), articleURL)

	// 結果検証
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Error("Expected result to not be nil")
		return // Stop execution if result is nil
	}
	if *result != expectedContent {
		t.Errorf("Expected content length %d, got %d", len(expectedContent), len(*result))
	}
}

func TestFetchArticleUsecase_Execute_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	// テストデータ
	articleURL := "https://example.com/article"
	expectedError := errors.New("fetch error")

	// モックの期待値設定
	mockArticleFetcher.EXPECT().
		FetchArticleContents(context.Background(), articleURL).
		Return(nil, expectedError).
		Times(1)

	// テスト実行
	result, err := usecase.Execute(context.Background(), articleURL)

	// 結果検証
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if err.Error() != expectedError.Error() {
		t.Errorf("Expected error %v, got %v", expectedError, err)
	}
	if result != nil {
		t.Errorf("Expected result to be nil, got %v", result)
	}
}

func TestFetchArticleUsecase_Execute_ExtractsTextFromHTML(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	// テストデータ: 画像、スクリプト、スタイルを含むHTML
	articleURL := "https://example.com/article"
	rawHTML := `<html>
		<head><style>body { color: red; }</style></head>
		<body>
			<script>alert('xss')</script>
			<img src="https://example.com/image.jpg" alt="test"/>
			<p>This is the article content which needs to be long enough to pass validation.</p>
			<p>Second paragraph with text. We need to add more text here to ensure the total length exceeds 100 characters. Repeating some words just to be sure we have enough content for the extractor to accept it as a valid article.</p>
		</body>
	</html>`

	// モックの期待値設定
	mockArticleFetcher.EXPECT().
		FetchArticleContents(context.Background(), articleURL).
		Return(&rawHTML, nil).
		Times(1)

	// テスト実行
	result, err := usecase.Execute(context.Background(), articleURL)

	// 結果検証
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected result to not be nil")
	}

	// 画像タグ、スクリプト、スタイルが削除されていることを確認
	if contains(*result, "<img") {
		t.Error("Expected image tags to be removed from result")
	}
	if contains(*result, "<script") {
		t.Error("Expected script tags to be removed from result")
	}
	if contains(*result, "<style") {
		t.Error("Expected style tags to be removed from result")
	}
	if contains(*result, "alert") {
		t.Error("Expected script content to be removed from result")
	}

	// テキストコンテンツが含まれていることを確認
	if !contains(*result, "This is the article content") {
		t.Error("Expected article content to be preserved")
	}
	if !contains(*result, "Second paragraph") {
		t.Error("Expected second paragraph to be preserved")
	}
}

func TestFetchArticleUsecase_Execute_HandlesPlainText(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	// テストデータ: プレーンテキスト
	articleURL := "https://example.com/article"
	plainText := "This is plain text without any HTML tags. It also needs to be long enough to pass the minimum length check. We represent a simple text file or a response that has no HTML structure but contains valuable information that we want to preserve."

	// モックの期待値設定
	mockArticleFetcher.EXPECT().
		FetchArticleContents(context.Background(), articleURL).
		Return(&plainText, nil).
		Times(1)

	// テスト実行
	result, err := usecase.Execute(context.Background(), articleURL)

	// 結果検証
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("Expected result to not be nil")
	}
	if !contains(*result, plainText) {
		t.Errorf("Expected plain text to be preserved, got %s", *result)
	}
}

func TestFetchArticleUsecase_Execute_ReturnsErrorForEmptyContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	// テストデータ: 空のコンテンツ
	articleURL := "https://example.com/article"
	emptyContent := ""

	// モックの期待値設定
	mockArticleFetcher.EXPECT().
		FetchArticleContents(context.Background(), articleURL).
		Return(&emptyContent, nil).
		Times(1)

	// テスト実行
	result, err := usecase.Execute(context.Background(), articleURL)

	// 結果検証
	if err == nil {
		t.Error("Expected error for empty content, got nil")
	}
	if result != nil {
		t.Errorf("Expected nil result for empty content, got %v", result)
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type upsertMatcher struct {
	check func(rag_integration_port.UpsertArticleInput) bool
}

func (m upsertMatcher) Matches(x interface{}) bool {
	input, ok := x.(rag_integration_port.UpsertArticleInput)
	if !ok {
		return false
	}
	return m.check(input)
}

func (m upsertMatcher) String() string {
	return "matches upsert input"
}

func TestFetchCompliantArticle_ScrapingPolicyDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)
	mockScrapingPolicy := mocks.NewMockScrapingPolicyPort(ctrl)

	usecase := NewArticleUsecaseWithScrapingPolicy(
		mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag, mockScrapingPolicy,
	)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	// Article not in DB, domain not declined
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)

	// ScrapingPolicy denies the fetch
	mockScrapingPolicy.EXPECT().CanFetchArticle(gomock.Any(), articleURLStr).Return(false, nil)

	// Save declined domain
	mockRepo.EXPECT().SaveDeclinedDomain(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	// Execute
	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	// Verify: should return ComplianceError
	if err == nil {
		t.Fatal("Expected ComplianceError, got nil")
	}
	var complianceErr *domain.ComplianceError
	if !errors.As(err, &complianceErr) {
		t.Errorf("Expected ComplianceError, got %T: %v", err, err)
	}
}

func TestFetchCompliantArticle_ScrapingPolicyAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)
	mockScrapingPolicy := mocks.NewMockScrapingPolicyPort(ctrl)

	usecase := NewArticleUsecaseWithScrapingPolicy(
		mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag, mockScrapingPolicy,
	)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></body></html>"
	expectedContentHTML := "<div><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></div>"
	articleID := "article-456"

	// Article not in DB, domain not declined
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)

	// ScrapingPolicy allows the fetch
	mockScrapingPolicy.EXPECT().CanFetchArticle(gomock.Any(), articleURLStr).Return(true, nil)

	// Fetch and save
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(articleID, nil)
	// RAG upsert is async (goroutine); use AnyTimes to avoid race with ctrl.Finish
	mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Execute
	content, retID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected content %s, got %s", expectedContentHTML, content)
	}
	if retID != articleID {
		t.Errorf("Expected article ID %s, got %s", articleID, retID)
	}
	time.Sleep(50 * time.Millisecond) // allow async goroutine to complete
	ctrl.Finish()
}

func TestFetchCompliantArticle_ScrapingPolicyNil_FallbackToRobotsTxt(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	// Use original constructor without ScrapingPolicyPort
	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></body></html>"
	expectedContentHTML := "<div><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></div>"
	articleID := "article-789"

	// Article not in DB, domain not declined
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)

	// Fallback to robotsTxt.IsPathAllowed (since scrapingPolicyPort is nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

	// Fetch and save
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(articleID, nil)
	// RAG upsert is async (goroutine)
	mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Execute
	content, retID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected content %s, got %s", expectedContentHTML, content)
	}
	if retID != articleID {
		t.Errorf("Expected article ID %s, got %s", articleID, retID)
	}
	time.Sleep(50 * time.Millisecond)
	ctrl.Finish()
}

func TestFetchArticleUsecase_FetchCompliantArticle_UpsertsToRAG(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code with mocks and sanitization logic.</p></body></html>"
	// ExtractArticleHTML returns sanitized HTML, not plain text
	expectedContentHTML := "<div><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code with mocks and sanitization logic.</p></div>"
	articleID := "article-123"

	// Mock expectations
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(articleID, nil)

	// RAG upsert is async; use WaitGroup to synchronize with goroutine
	var ragWg sync.WaitGroup
	ragWg.Add(1)
	mockRag.EXPECT().UpsertArticle(gomock.Any(), upsertMatcher{
		check: func(input rag_integration_port.UpsertArticleInput) bool {
			return input.ArticleID == articleID && input.Body == expectedContentHTML && input.URL == articleURLStr
		},
	}).DoAndReturn(func(_ context.Context, _ rag_integration_port.UpsertArticleInput) error {
		ragWg.Done()
		return nil
	})

	// Execute
	content, retArticleID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	// Verify synchronous results
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected content %s, got %s", expectedContentHTML, content)
	}
	if retArticleID == "" {
		t.Errorf("Expected non-empty article ID")
	}

	// Wait for async RAG upsert goroutine to complete
	ragWg.Wait()
	ctrl.Finish()
}

func TestFetchCompliantArticle_RAGUpsertIsAsync(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></body></html>"
	articleID := "article-async"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return(articleID, nil)

	// RAG mock blocks for 500ms; FetchCompliantArticle should return before that
	ragStarted := make(chan struct{})
	ragDone := make(chan struct{})
	mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ rag_integration_port.UpsertArticleInput) error {
			close(ragStarted)
			time.Sleep(500 * time.Millisecond)
			close(ragDone)
			return nil
		},
	)

	start := time.Now()
	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// The method should return almost immediately, well before the 500ms RAG sleep
	if elapsed > 200*time.Millisecond {
		t.Errorf("FetchCompliantArticle took %v, expected < 200ms (RAG should be async)", elapsed)
	}

	// Wait for async goroutine to complete before ctrl.Finish()
	<-ragDone
	ctrl.Finish()
}

func TestFetchCompliantArticle_SingleflightDeduplicates(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://zenn.dev/test/articles/duplicate-fetch"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about singleflight deduplication testing.</p></body></html>"
	articleID := "article-sf"

	// DB lookup: both goroutines will find no article
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil).Times(2)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).Times(2)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(2)

	// KEY ASSERTION: FetchArticleContents must be called ONLY ONCE thanks to singleflight
	fetchStarted := make(chan struct{})
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).DoAndReturn(
		func(ctx context.Context, _ string) (*string, error) {
			close(fetchStarted)
			// Simulate slow fetch so second request arrives during first
			time.Sleep(100 * time.Millisecond)
			return &rawHTML, nil
		},
	).Times(1)

	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return(articleID, nil).Times(1)
	mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	// Run two concurrent calls for the same URL
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
	time.Sleep(50 * time.Millisecond) // allow async goroutine

	// Both calls should succeed with the same content
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
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a refreshed article about testing Go code.</p></body></html>"
	articleID := "article-refresh"

	// KEY: FetchArticleByURL should NOT be called when forceRefresh=true
	// (no mockRepo.EXPECT().FetchArticleByURL)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return(articleID, nil)
	mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	// Content must exceed minFulltextContentLength (500 bytes) to be treated as a cache hit
	longContent := strings.Repeat("This is a cached full article with enough content. ", 15) // ~750 bytes
	existingArticle := &domain.ArticleContent{
		ID:      "existing-article-id",
		Content: longContent,
	}

	// DB lookup returns existing article with long content
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(existingArticle, nil)
	mockRepo.EXPECT().FetchOgImageURLByArticleID(gomock.Any(), existingArticle.ID).Return("", nil)

	// KEY: FetchArticleContents should NOT be called when article has sufficient content
	// (no mockArticleFetcher.EXPECT().FetchArticleContents)

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
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/inoreader-short"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	// Short cached content (RSS summary from Inoreader, below minFulltextContentLength)
	shortCached := &domain.ArticleContent{
		ID:      "short-article-id",
		Content: "This is a short RSS summary from Inoreader. It contains only a few sentences.",
	}

	// Full article fetched from web
	rawHTML := "<html><body><p>" + strings.Repeat("Full article content fetched from web. ", 30) + "</p></body></html>"
	expectedContentHTML := "<div><p>" + strings.Repeat("Full article content fetched from web. ", 30) + "</p></div>"
	webArticleID := "web-fetched-article-id"

	// DB lookup returns article with short content
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(shortCached, nil)
	// Short content should NOT return cached — instead proceed to compliance + web fetch
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(webArticleID, nil)
	mockRag.EXPECT().UpsertArticle(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

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
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/full-article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	// Long cached content (full article, above minFulltextContentLength)
	longContent := strings.Repeat("This is a full article with rich content from the web. ", 20) // ~1080 bytes
	existingArticle := &domain.ArticleContent{
		ID:      "long-article-id",
		Content: longContent,
	}

	// DB lookup returns article with long content — should return cached without web fetch
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(existingArticle, nil)
	mockRepo.EXPECT().FetchOgImageURLByArticleID(gomock.Any(), existingArticle.ID).Return("https://example.com/og.jpg", nil)

	// KEY: FetchArticleContents should NOT be called — long cached content is sufficient
	// (no mockArticleFetcher.EXPECT().FetchArticleContents)

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
	mockRobotsTxt := mocks.NewMockRobotsTxtPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockRag := mocks.NewMockRagIntegrationPort(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo, mockRag)

	articleURLStr := "https://example.com/slow-article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)

	// Simulate a fetch that respects context cancellation (the 25s usecase timeout)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).DoAndReturn(
		func(ctx context.Context, _ string) (*string, error) {
			// Verify that the context has a deadline (from the 25s timeout)
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("Expected context to have a deadline from web fetch timeout")
				content := "no deadline"
				return &content, nil
			}
			// The deadline should be ~25s from now (give or take)
			remaining := time.Until(deadline)
			if remaining > 26*time.Second || remaining < 20*time.Second {
				t.Errorf("Expected ~25s timeout, got remaining %v", remaining)
			}
			return nil, context.DeadlineExceeded
		},
	)

	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)
	if err == nil {
		t.Fatal("Expected error from timed-out fetch")
	}
}
