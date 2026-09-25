package article_content_extractor_gateway

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_NilLoggerPanics(t *testing.T) {
	assert.Panics(t, func() {
		New(nil)
	})
}

func TestExtractArticleContent_BlocksPrivateIPs(t *testing.T) {
	gw := New(slog.Default())
	ctx := context.Background()

	privateURLs := []string{
		"http://127.0.0.1/secret",
		"http://10.0.0.1/internal",
		"http://192.168.1.1/admin",
		"http://172.16.0.1/metadata",
	}

	for _, urlStr := range privateURLs {
		t.Run(urlStr, func(t *testing.T) {
			_, _, err := gw.ExtractArticleContent(ctx, urlStr)
			require.Error(t, err, "should block private IP: %s", urlStr)
			assert.Contains(t, err.Error(), "not allowed", "error should indicate URL is not allowed")
		})
	}
}

func TestExtractArticleContent_BlocksMetadataEndpoints(t *testing.T) {
	gw := New(slog.Default())
	ctx := context.Background()

	metadataURLs := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://metadata.google.internal/computeMetadata/v1/",
	}

	for _, urlStr := range metadataURLs {
		t.Run(urlStr, func(t *testing.T) {
			_, _, err := gw.ExtractArticleContent(ctx, urlStr)
			require.Error(t, err, "should block metadata endpoint: %s", urlStr)
		})
	}
}

func TestExtractArticleContent_BlocksNonHTTPSchemes(t *testing.T) {
	gw := New(slog.Default())
	ctx := context.Background()

	badSchemes := []string{
		"ftp://example.com/file",
		"file:///etc/passwd",
		"gopher://example.com/",
	}

	for _, urlStr := range badSchemes {
		t.Run(urlStr, func(t *testing.T) {
			_, _, err := gw.ExtractArticleContent(ctx, urlStr)
			require.Error(t, err, "should block non-HTTP scheme: %s", urlStr)
		})
	}
}

func TestExtractArticleContent_Success(t *testing.T) {
	articleBody := "This is a sufficiently long article content that is extracted from the web page for testing purposes. It must exceed the minimum article length threshold to be accepted as extracted plain text."
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "<html><head><title>Test Title</title></head><body><article><p>%s</p></article></body></html>", articleBody)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("FEED_ALLOWED_HOSTS", u.Hostname())

	gw := New(slog.Default())
	gw.SetTestingMode(true)

	content, title, err := gw.ExtractArticleContent(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, "Test Title", title)
	assert.Equal(t, articleBody, content)
}

func TestExtractArticleContent_Non200Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("FEED_ALLOWED_HOSTS", u.Hostname())

	gw := New(slog.Default())
	gw.SetTestingMode(true)

	_, _, err = gw.ExtractArticleContent(context.Background(), server.URL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server returned status 404")
}

func TestExtractArticleContent_Limit2MB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		chunk := make([]byte, 1024*1024)
		for i := range chunk {
			chunk[i] = 'a'
		}
		for i := 0; i < 3; i++ {
			_, _ = w.Write(chunk)
		}
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("FEED_ALLOWED_HOSTS", u.Hostname())

	gw := New(slog.Default())
	gw.SetTestingMode(true)

	content, _, err := gw.ExtractArticleContent(context.Background(), server.URL)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(content), 2*1024*1024)
}

func TestExtractArticleContent_RawHTMLFallback(t *testing.T) {
	rawHTML := `<html><head><title>No Paragraphs</title></head><body><div>Just plain div without text parser finding article body</div></body></html>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, rawHTML)
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	t.Setenv("FEED_ALLOWED_HOSTS", u.Hostname())

	gw := New(slog.Default())
	gw.SetTestingMode(true)

	content, title, err := gw.ExtractArticleContent(context.Background(), server.URL)
	require.NoError(t, err)
	assert.Equal(t, "No Paragraphs", title)
	assert.Contains(t, content, "Just plain div")
}

func TestExtractArticleContent_UsesParsedURL(t *testing.T) {
	raw := "http://127.0.0.1@example.com/"
	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "example.com", parsed.Host)
	assert.NotNil(t, parsed.User)
	assert.Equal(t, "127.0.0.1", parsed.User.String())

	gw := New(slog.Default())
	_, _, err = gw.ExtractArticleContent(context.Background(), raw)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "userinfo not allowed")
}
