package archive_article_gateway

import (
	"alt/port/archive_article_port"
	"context"
	"errors"
	"testing"
)

type stubSaver struct {
	called    bool
	url       string
	title     string
	content   string
	returnErr error
}

func (s *stubSaver) SaveArticle(ctx context.Context, url, title, content string) error {
	if ctx == nil {
		panic("context must not be nil")
	}
	s.called = true
	s.url = url
	s.title = title
	s.content = content
	return s.returnErr
}

func TestArchiveArticleGateway_SaveArticle_Success(t *testing.T) {
	saver := &stubSaver{}
	gateway := NewArchiveArticleGateway(saver)

	record := archive_article_port.ArticleRecord{
		URL:     "https://example.com/article",
		Title:   "Example",
		Content: "<html>body</html>",
	}

	if err := gateway.SaveArticle(context.Background(), record); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !saver.called {
		t.Fatal("expected saver to be invoked")
	}
	if saver.url != record.URL {
		t.Fatalf("expected URL %q, got %q", record.URL, saver.url)
	}
	if saver.title != record.Title {
		t.Fatalf("expected Title %q, got %q", record.Title, saver.title)
	}
	if saver.content != record.Content {
		t.Fatalf("expected Content %q, got %q", record.Content, saver.content)
	}
}

func TestArchiveArticleGateway_SaveArticle_MissingRepo(t *testing.T) {
	gateway := NewArchiveArticleGateway(nil)
	record := archive_article_port.ArticleRecord{URL: "https://example.com", Content: "body"}

	if err := gateway.SaveArticle(context.Background(), record); err == nil {
		t.Fatal("expected error when repository is nil")
	}
}

func TestArchiveArticleGateway_SaveArticle_InvalidInput(t *testing.T) {
	saver := &stubSaver{}
	gateway := NewArchiveArticleGateway(saver)

	invalidCases := []archive_article_port.ArticleRecord{
		{URL: "", Content: "body"},
		{URL: "   ", Content: "body"},
		{URL: "https://example.com", Content: ""},
		{URL: "https://example.com", Content: "   "},
	}

	for _, tc := range invalidCases {
		if err := gateway.SaveArticle(context.Background(), tc); err == nil {
			t.Fatalf("expected error for invalid input %+v", tc)
		}
	}

	if saver.called {
		t.Fatal("saver should not be invoked for invalid input")
	}
}

func TestArchiveArticleGateway_SaveArticle_SaverError(t *testing.T) {
	returnErr := errors.New("db failure")
	saver := &stubSaver{returnErr: returnErr}
	gateway := NewArchiveArticleGateway(saver)
	record := archive_article_port.ArticleRecord{URL: "https://example.com", Title: "Example", Content: "data"}

	err := gateway.SaveArticle(context.Background(), record)
	if !errors.Is(err, returnErr) {
		t.Fatalf("expected error %v, got %v", returnErr, err)
	}
	if !saver.called {
		t.Fatal("expected saver to be called")
	}
}
