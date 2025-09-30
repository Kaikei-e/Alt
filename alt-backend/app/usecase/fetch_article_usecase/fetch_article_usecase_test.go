package fetch_article_usecase

import (
	"alt/mocks"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFetchArticleUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher)

	// テストデータ
	articleURL := "https://example.com/article"
	expectedContent := "test article content"

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
	}
	if *result != expectedContent {
		t.Errorf("Expected %s, got %s", expectedContent, *result)
	}
}

func TestFetchArticleUsecase_Execute_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// モックの準備
	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	usecase := NewArticleUsecase(mockArticleFetcher)

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
	usecase := NewArticleUsecase(mockArticleFetcher)

	// テストデータ: 画像、スクリプト、スタイルを含むHTML
	articleURL := "https://example.com/article"
	rawHTML := `<html>
		<head><style>body { color: red; }</style></head>
		<body>
			<script>alert('xss')</script>
			<img src="https://example.com/image.jpg" alt="test"/>
			<p>This is the article content.</p>
			<p>Second paragraph with text.</p>
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
	usecase := NewArticleUsecase(mockArticleFetcher)

	// テストデータ: プレーンテキスト
	articleURL := "https://example.com/article"
	plainText := "This is plain text without any HTML tags."

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
	usecase := NewArticleUsecase(mockArticleFetcher)

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
