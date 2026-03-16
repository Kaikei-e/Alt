package rest_feeds

import (
	"alt/di"
	"alt/domain"
	"alt/usecase/opml_usecase"
	"alt/utils/logger"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.InitLogger()
}

// testExportPort implements opml_port.ExportOPMLPort for testing.
type testExportPort struct {
	links []*domain.FeedLinkForExport
	err   error
}

func (p *testExportPort) FetchFeedLinksForExport(_ context.Context) ([]*domain.FeedLinkForExport, error) {
	return p.links, p.err
}

// testImportPort implements opml_port.ImportOPMLPort for testing.
type testImportPort struct {
	result *domain.OPMLImportResult
	err    error
}

func (p *testImportPort) RegisterFeedLinkBulk(_ context.Context, _ []string) (*domain.OPMLImportResult, error) {
	return p.result, p.err
}

func createExportContainer(links []*domain.FeedLinkForExport, err error) *di.ApplicationComponents {
	port := &testExportPort{links: links, err: err}
	return &di.ApplicationComponents{
		ExportOPMLUsecase: opml_usecase.NewExportOPMLUsecase(port),
	}
}

func createImportContainer(result *domain.OPMLImportResult, err error) *di.ApplicationComponents {
	port := &testImportPort{result: result, err: err}
	return &di.ApplicationComponents{
		ImportOPMLUsecase: opml_usecase.NewImportOPMLUsecase(port),
	}
}

func TestRestHandleExportOPML_Success(t *testing.T) {
	e := echo.New()

	links := []*domain.FeedLinkForExport{
		{URL: "https://example.com/feed.xml", Title: "Example Feed"},
	}

	container := createExportContainer(links, nil)

	req := httptest.NewRequest(http.MethodGet, "/v1/rss-feed-link/export/opml", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := RestHandleExportOPML(container)
	err := handler(c)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/xml", rec.Header().Get("Content-Type"))
	assert.Contains(t, rec.Header().Get("Content-Disposition"), "alt-feeds.opml")
	assert.Contains(t, rec.Body.String(), `xmlUrl="https://example.com/feed.xml"`)
}

func TestRestHandleImportOPML_Success(t *testing.T) {
	e := echo.New()

	expectedResult := &domain.OPMLImportResult{
		Total:    2,
		Imported: 1,
		Skipped:  1,
	}

	container := createImportContainer(expectedResult, nil)

	opmlContent := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Test</title></head>
  <body>
    <outline text="Feed 1" type="rss" xmlUrl="https://example.com/feed.xml" />
    <outline text="Feed 2" type="rss" xmlUrl="https://other.com/rss" />
  </body>
</opml>`

	body, contentType := createMultipartForm(t, "file", "feeds.opml", opmlContent)

	req := httptest.NewRequest(http.MethodPost, "/v1/rss-feed-link/import/opml", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := RestHandleImportOPML(container)
	err := handler(c)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, rec.Code)

	var result domain.OPMLImportResult
	err = json.Unmarshal(rec.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 1, result.Imported)
	assert.Equal(t, 1, result.Skipped)
}

func TestRestHandleImportOPML_MissingFile(t *testing.T) {
	e := echo.New()

	container := createImportContainer(nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/v1/rss-feed-link/import/opml", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := RestHandleImportOPML(container)
	err := handler(c)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRestHandleImportOPML_FileTooLarge(t *testing.T) {
	e := echo.New()

	container := createImportContainer(nil, nil)

	largeContent := strings.Repeat("x", maxOPMLFileSize+1)
	body, contentType := createMultipartForm(t, "file", "large.opml", largeContent)

	req := httptest.NewRequest(http.MethodPost, "/v1/rss-feed-link/import/opml", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	handler := RestHandleImportOPML(container)
	err := handler(c)

	require.NoError(t, err)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func createMultipartForm(t *testing.T, fieldName, fileName, content string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(fieldName, fileName)
	require.NoError(t, err)
	_, err = io.Copy(part, strings.NewReader(content))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return &buf, writer.FormDataContentType()
}
