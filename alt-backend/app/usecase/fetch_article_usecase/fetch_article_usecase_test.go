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
