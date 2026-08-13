package backend_api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	"search-indexer/domain"
	datahubv1 "search-indexer/gen/proto/services/datahub/v1"
	"search-indexer/gen/proto/services/datahub/v1/datahubv1connect"
)

// TestToDriverArticle_PublishedAt covers the mapping that decides what
// SearchWithDateFilter and Acolyte's weekly_briefing window see. The article
// fetch is the only source of a document's timestamps once the fat event
// payload is gone, so a real published_at must never be overwritten by
// created_at.
func TestToDriverArticle_PublishedAt(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)
	publishedAt := time.Date(2026, 3, 20, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		name  string
		proto *datahubv1.ArticleWithTags
		want  time.Time
	}{
		{
			name: "uses published_at when the backend supplies it",
			proto: &datahubv1.ArticleWithTags{
				Id:          "art-001",
				CreatedAt:   timestamppb.New(createdAt),
				PublishedAt: timestamppb.New(publishedAt),
			},
			want: publishedAt,
		},
		{
			name: "falls back to created_at when published_at is unset",
			proto: &datahubv1.ArticleWithTags{
				Id:        "art-002",
				CreatedAt: timestamppb.New(createdAt),
			},
			want: createdAt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := toDriverArticle(tt.proto)
			if !got.PublishedAt.Equal(tt.want) {
				t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, tt.want)
			}
			if !got.CreatedAt.Equal(createdAt) {
				t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, createdAt)
			}
		})
	}
}

// TestClient_HTTPClient_HasTimeout verifies the Connect-RPC client uses an
// http.Client with a non-zero Timeout, protecting search-indexer from
// alt-backend hangs (ADR-000717 / C-003).
func TestClient_HTTPClient_HasTimeout(t *testing.T) {
	t.Parallel()

	hc := DefaultHTTPClient()
	if hc.Timeout == 0 {
		t.Fatal("http.Client.Timeout must be non-zero to prevent resource exhaustion")
	}
	if hc.Timeout > 60*time.Second {
		t.Fatalf("http.Client.Timeout=%s is too permissive (want <= 60s)", hc.Timeout)
	}
	if hc == http.DefaultClient {
		t.Fatal("must not reuse http.DefaultClient; construct a dedicated Client")
	}
}

// TestClient_TimesOutOnSlowBackend verifies that a hanging backend server
// causes the client to return within the configured timeout rather than
// blocking forever.
func TestClient_TimesOutOnSlowBackend(t *testing.T) {
	t.Parallel()

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block longer than any sane timeout.
		time.Sleep(5 * time.Second)
	}))
	defer slow.Close()

	c := NewClient(slow.URL, "test-token", nil)

	// Bound the call with a short context so the test completes quickly even
	// if the http.Client timeout regresses.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, _, err := c.GetArticlesWithTags(ctx, nil, "", 10)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error from slow backend, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("call took %s; expected fast timeout via context/client", elapsed)
	}
}

// notFoundDataHub answers GetArticleByID exactly the way alt-data-hub does
// for a missing (or soft-deleted) row: the handler turns every repository
// error, pgx.ErrNoRows included, into connect.CodeNotFound.
type notFoundDataHub struct {
	datahubv1connect.UnimplementedDataHubServiceHandler
	code connect.Code
}

func (s *notFoundDataHub) GetArticleByID(context.Context, *connect.Request[datahubv1.GetArticleByIDRequest]) (*connect.Response[datahubv1.GetArticleByIDResponse], error) {
	return nil, connect.NewError(s.code, errors.New("article not found"))
}

func newDataHubTestServer(t *testing.T, code connect.Code) *Client {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(datahubv1connect.NewDataHubServiceHandler(&notFoundDataHub{code: code}))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return NewClient(srv.URL, "test-token", srv.Client())
}

// TestClient_GetArticleByID_NotFoundIsSentinel pins the only signal the batch
// indexer can act on. alt-data-hub reports a missing row as a Connect
// NotFound error, never as an empty response, so the driver must translate
// that code into domain.ErrArticleNotFound -- otherwise
// ExecuteBatchArticles's skip branch is unreachable and one deleted article
// drags every healthy article in the same batch to the DLQ.
func TestClient_GetArticleByID_NotFoundIsSentinel(t *testing.T) {
	t.Parallel()

	c := newDataHubTestServer(t, connect.CodeNotFound)

	article, err := c.GetArticleByID(context.Background(), "art-missing")

	if article != nil {
		t.Fatalf("GetArticleByID() article = %+v, want nil", article)
	}
	if !errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("GetArticleByID() error = %v, want errors.Is(err, domain.ErrArticleNotFound)", err)
	}
}

// TestClient_GetArticleByID_OtherCodesAreNotSentinel keeps the translation
// narrow: a transport or backend failure must still abort the batch and get
// retried instead of being silently skipped as "article gone".
func TestClient_GetArticleByID_OtherCodesAreNotSentinel(t *testing.T) {
	t.Parallel()

	c := newDataHubTestServer(t, connect.CodeUnavailable)

	_, err := c.GetArticleByID(context.Background(), "art-001")

	if err == nil {
		t.Fatal("GetArticleByID() error = nil, want an error")
	}
	if errors.Is(err, domain.ErrArticleNotFound) {
		t.Fatalf("GetArticleByID() error = %v, must not be ErrArticleNotFound for a non-NotFound failure", err)
	}
}
