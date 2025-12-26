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
	content := "<p>article body needs to be long enough to pass the validation check. We are adding more text here to ensure we cross the 100 character threshold required by the cleaner utility.</p>"
	expected := "article body needs to be long enough to pass the validation check. We are adding more text here to ensure we cross the 100 character threshold required by the cleaner utility."

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
	if saver.lastRecord.Content != expected {
		t.Fatalf("expected Content %q, got %q", expected, saver.lastRecord.Content)
	}
}

func TestArchiveArticleUsecase_Execute_StripsNonTextContent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "https://example.com/article"}
	raw := `<html><head><title>Ignored</title><script>alert('x')</script></head><body><p>Hello</p><p>World. This text is extended to meet the minimum length requirement of 100 characters. We need to ensure that the sanitization process preserves this content while stripping out the unwanted tags.</p></body></html>`

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(&raw, nil)

	err := usecase.Execute(context.Background(), input)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if saver.lastRecord.Content != "Hello\n\nWorld. This text is extended to meet the minimum length requirement of 100 characters. We need to ensure that the sanitization process preserves this content while stripping out the unwanted tags." {
		t.Fatalf("expected sanitized paragraphs, for %q", saver.lastRecord.Content)
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
	content := "<html><body><p>This article content is specifically made longer to pass the minimum length check of 100 characters. We need to ensure that even when simulating a save error, the extraction logic proceeds far enough to attempt the save.</p></body></html>"

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

func TestArchiveArticleUsecase_Execute_EmptyAfterExtraction(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "https://example.com"}
	raw := "<script>alert('x')</script>"

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(&raw, nil)

	err := usecase.Execute(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when extraction yields empty text")
	}
	if saver.called {
		t.Fatal("saver should not be invoked when extracted content is empty")
	}
}

func TestArchiveArticleUsecase_Execute_EmptyTitleUsesURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	fetcher := mocks.NewMockFetchArticlePort(ctrl)
	saver := &recordingSaver{t: t}
	usecase := NewArchiveArticleUsecase(fetcher, saver)

	input := ArchiveArticleInput{URL: "https://example.com/article", Title: ""}
	content := "This body content must also be long enough to pass the validation check. We are testing the empty title fallback mechanism, but the content extraction must succeed first for the usecase to proceed to saving."

	fetcher.EXPECT().FetchArticleContents(gomock.Any(), input.URL).Return(&content, nil)

	err := usecase.Execute(context.Background(), input)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if saver.lastRecord.Title != input.URL {
		t.Fatalf("expected title fallback to URL %q, got %q", input.URL, saver.lastRecord.Title)
	}
}
