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
	"testing"

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

func TestFetchArticleUsecase_FetchCompliantArticle_UpsertsToRAG(t *testing.T) {
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
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code with mocks and sanitization logic.</p></body></html>"
	contentStr := "Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code with mocks and sanitization logic."
	articleID := "article-123"

	// Mock expectations
	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), contentStr).Return(articleID, nil)

	// Expect UpsertArticle to be called
	mockRag.EXPECT().UpsertArticle(gomock.Any(), upsertMatcher{
		check: func(input rag_integration_port.UpsertArticleInput) bool {
			return input.ArticleID == articleID && input.Body == contentStr && input.URL == articleURLStr
		},
	}).Return(nil)

	// Execute
	content, articleID, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	// Verify
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != contentStr {
		t.Errorf("Expected content %s, got %s", contentStr, content)
	}
	if articleID == "" {
		t.Errorf("Expected non-empty article ID")
	}
}
