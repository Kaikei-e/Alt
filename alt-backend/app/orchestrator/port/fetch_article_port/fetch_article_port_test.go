package fetch_article_port

import (
	"alt/mocks"
	"context"
	"testing"

	"go.uber.org/mock/gomock"
)

func TestFetchArticlePortContract(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	expectedContent := "test article content"
	articleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	articleFetcher.EXPECT().FetchArticleContents(gomock.Any(), gomock.Any()).Return(&expectedContent, nil)

	result, err := articleFetcher.FetchArticleContents(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if result == nil {
		t.Fatalf("Expected result, got nil")
	}
	if *result != expectedContent {
		t.Fatalf("Expected %s, got %s", expectedContent, *result)
	}
}
