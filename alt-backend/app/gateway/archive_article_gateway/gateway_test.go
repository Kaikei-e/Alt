package archive_article_gateway

import (
	"alt/domain"
	"alt/port/archive_article_port"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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

	// Create context with user
	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := domain.SetUserContext(context.Background(), userCtx)

	record := archive_article_port.ArticleRecord{
		URL:     "https://example.com/article",
		Title:   "Example",
		Content: "<html>body</html>",
	}

	if err := gateway.SaveArticle(ctx, record); err != nil {
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

	// Create context with user
	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := domain.SetUserContext(context.Background(), userCtx)

	record := archive_article_port.ArticleRecord{URL: "https://example.com", Content: "body"}

	if err := gateway.SaveArticle(ctx, record); err == nil {
		t.Fatal("expected error when repository is nil")
	}
}

func TestArchiveArticleGateway_SaveArticle_InvalidInput(t *testing.T) {
	saver := &stubSaver{}
	gateway := NewArchiveArticleGateway(saver)

	// Create context with user
	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := domain.SetUserContext(context.Background(), userCtx)

	invalidCases := []archive_article_port.ArticleRecord{
		{URL: "", Content: "body"},
		{URL: "   ", Content: "body"},
		{URL: "https://example.com", Content: ""},
		{URL: "https://example.com", Content: "   "},
	}

	for _, tc := range invalidCases {
		if err := gateway.SaveArticle(ctx, tc); err == nil {
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

	// Create context with user
	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	ctx := domain.SetUserContext(context.Background(), userCtx)

	record := archive_article_port.ArticleRecord{URL: "https://example.com", Title: "Example", Content: "data"}

	err := gateway.SaveArticle(ctx, record)
	if !errors.Is(err, returnErr) {
		t.Fatalf("expected error %v, got %v", returnErr, err)
	}
	if !saver.called {
		t.Fatal("expected saver to be called")
	}
}

func TestArchiveArticleGateway_SaveArticle_MissingUserContext(t *testing.T) {
	// Simulate a saver that returns an error when user context is missing
	// (This is what the real driver does)
	saver := &stubSaver{returnErr: errors.New("user context required: user context not found")}
	gateway := NewArchiveArticleGateway(saver)

	// Use context without user
	ctx := context.Background()

	record := archive_article_port.ArticleRecord{
		URL:     "https://example.com/article",
		Title:   "Example",
		Content: "body",
	}

	err := gateway.SaveArticle(ctx, record)
	if err == nil {
		t.Fatal("expected error when user context is missing")
	}
	// Gateway should propagate the error from the saver
	if !errors.Is(err, saver.returnErr) {
		t.Fatalf("expected wrapped error containing saver error, got %v", err)
	}
	// Saver should be called (gateway doesn't validate user context, driver does)
	if !saver.called {
		t.Fatal("saver should be called (even though it will fail)")
	}
}
