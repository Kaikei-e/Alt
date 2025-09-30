package archive_article_usecase

import (
	"alt/mocks"
	"alt/port/archive_article_port"
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"
)

type recordingSaver struct {
	t          *testing.T
	called     bool
	lastRecord archive_article_port.ArticleRecord
	returnErr  error
}

func (s *recordingSaver) SaveArticle(ctx context.Context, record archive_article_port.ArticleRecord) error {
	if ctx == nil {
		s.t.Fatalf("expected context to be non-nil")
	}
	s.called = true
	s.lastRecord = record
	return s.returnErr
}

func TestArchiveArticleUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{
		URL:   "https://example.com/article",
		Title: "Example Article",
	}
	content := "<p>article body</p>"

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(&content, nil)

	err := usecase.Execute(context.Background(), input)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !saver.called {
		t.Fatal("expected saver to be called")
	}
	if saver.lastRecord.URL != input.URL {
		t.Fatalf("expected URL %q, got %q", input.URL, saver.lastRecord.URL)
	}
	if saver.lastRecord.Title != input.Title {
		t.Fatalf("expected Title %q, got %q", input.Title, saver.lastRecord.Title)
	}
	if saver.lastRecord.Content != content {
		t.Fatalf("expected Content %q, got %q", content, saver.lastRecord.Content)
	}
}

func TestArchiveArticleUsecase_Execute_InvalidURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "   "}

	err := usecase.Execute(context.Background(), input)

	if err == nil {
		t.Fatal("expected error for empty URL")
	}
	if saver.called {
		t.Fatal("saver should not be called on validation error")
	}
}

func TestArchiveArticleUsecase_Execute_FetchError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "https://example.com/article"}
	fetchErr := errors.New("fetch failed")

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(nil, fetchErr)

	err := usecase.Execute(context.Background(), input)

	if err == nil {
		t.Fatal("expected error when fetcher fails")
	}
	if !errors.Is(err, fetchErr) {
		t.Fatalf("expected fetch error, got %v", err)
	}
	if saver.called {
		t.Fatal("saver should not be called when fetch fails")
	}
}

func TestArchiveArticleUsecase_Execute_SaveError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t, returnErr: errors.New("save failed")}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "https://example.com/article"}
	content := "<html>body</html>"

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(&content, nil)

	err := usecase.Execute(context.Background(), input)

	if err == nil {
		t.Fatal("expected error when saver fails")
	}
	if !errors.Is(err, saver.returnErr) {
		t.Fatalf("expected save error, got %v", err)
	}
	if !saver.called {
		t.Fatal("expected saver to be called")
	}
}

func TestArchiveArticleUsecase_Execute_EmptyTitleUsesURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "https://example.com/article", Title: ""}
	content := "body"

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(&content, nil)

	err := usecase.Execute(context.Background(), input)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if saver.lastRecord.Title != input.URL {
		t.Fatalf("expected title fallback to URL %q, got %q", input.URL, saver.lastRecord.Title)
	}
}
