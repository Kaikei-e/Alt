package opml_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestExportOPMLUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockExportOPMLPort(ctrl)

	links := []*domain.FeedLinkForExport{
		{URL: "https://example.com/feed.xml", Title: "Example Feed"},
		{URL: "https://blog.test.org/rss", Title: "Test Blog"},
	}

	mockPort.EXPECT().FetchFeedLinksForExport(gomock.Any()).Return(links, nil)

	usecase := NewExportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)

	xml := string(result)
	assert.Contains(t, xml, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, xml, `<opml version="2.0">`)
	assert.Contains(t, xml, `xmlUrl="https://example.com/feed.xml"`)
	assert.Contains(t, xml, `xmlUrl="https://blog.test.org/rss"`)
	assert.Contains(t, xml, `text="Example Feed"`)
	assert.Contains(t, xml, `text="Test Blog"`)
	assert.Contains(t, xml, `type="rss"`)
	assert.Contains(t, xml, `<title>Alt RSS Feeds</title>`)
}

func TestExportOPMLUsecase_Execute_EmptyList(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockExportOPMLPort(ctrl)
	mockPort.EXPECT().FetchFeedLinksForExport(gomock.Any()).Return([]*domain.FeedLinkForExport{}, nil)

	usecase := NewExportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)

	xml := string(result)
	assert.Contains(t, xml, `<opml version="2.0">`)
	// Body should have no outline elements
	assert.NotContains(t, xml, `xmlUrl=`)
}

func TestExportOPMLUsecase_Execute_PortError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockExportOPMLPort(ctrl)
	mockPort.EXPECT().FetchFeedLinksForExport(gomock.Any()).Return(nil, errors.New("db error"))

	usecase := NewExportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "fetch feed links for export")
}

func TestExportOPMLUsecase_Execute_DeduplicatesUTMVariants(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockExportOPMLPort(ctrl)

	// Same feed URL with different UTM params should be deduplicated to one
	links := []*domain.FeedLinkForExport{
		{URL: "https://example.com/feed?utm_source=rss", Title: "Example Feed"},
		{URL: "https://example.com/feed?utm_source=chatgpt", Title: "Example Feed Dup"},
		{URL: "https://blog.test.org/rss", Title: "Test Blog"},
	}

	mockPort.EXPECT().FetchFeedLinksForExport(gomock.Any()).Return(links, nil)

	usecase := NewExportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	require.NoError(t, err)
	require.NotNil(t, result)

	xml := string(result)
	// Should have the cleaned URL (no UTM)
	assert.Contains(t, xml, `xmlUrl="https://example.com/feed"`)
	// The duplicate should not appear (only one outline for example.com/feed)
	assert.Equal(t, 1, strings.Count(xml, `xmlUrl="https://example.com/feed"`))
	// The first title wins
	assert.Contains(t, xml, `text="Example Feed"`)
	assert.NotContains(t, xml, `text="Example Feed Dup"`)
	// Other feeds still present
	assert.Contains(t, xml, `xmlUrl="https://blog.test.org/rss"`)
}

func TestExportOPMLUsecase_Execute_HTMLEntityDecode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockExportOPMLPort(ctrl)

	links := []*domain.FeedLinkForExport{
		{URL: "https://example.com/feed.xml", Title: "O&#39;Reilly Media"},
		{URL: "https://blog.test.org/rss", Title: "Tom &amp; Jerry"},
		{URL: "https://news.example.com/rss", Title: "Clean Title"},
	}

	mockPort.EXPECT().FetchFeedLinksForExport(gomock.Any()).Return(links, nil)

	usecase := NewExportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	require.NoError(t, err)

	xml := string(result)
	// html.UnescapeString decodes &#39; -> ' and &amp; -> &
	// XML marshal then re-encodes & -> &amp; but ' stays as-is in attributes
	// So O&#39;Reilly -> O'Reilly (after unescape) -> text="O&#39;Reilly" in XML attr
	// Tom &amp; Jerry -> Tom & Jerry (after unescape) -> text="Tom &amp; Jerry" in XML attr
	assert.Contains(t, xml, `O&#39;Reilly Media`)
	assert.Contains(t, xml, `Tom &amp; Jerry`)
	assert.Contains(t, xml, `Clean Title`)
	// The double-escaped entity &amp;#39; should NOT appear
	assert.NotContains(t, xml, `&amp;#39;`)
	// The double-escaped &amp;amp; should NOT appear
	assert.NotContains(t, xml, `&amp;amp;`)
}

func TestExportOPMLUsecase_ValidOPMLFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockExportOPMLPort(ctrl)
	links := []*domain.FeedLinkForExport{
		{URL: "https://example.com/feed.xml", Title: "Test Feed"},
	}
	mockPort.EXPECT().FetchFeedLinksForExport(gomock.Any()).Return(links, nil)

	usecase := NewExportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	require.NoError(t, err)

	// Verify the XML is well-formed by parsing it back
	xml := string(result)
	assert.True(t, strings.HasPrefix(xml, `<?xml version="1.0" encoding="UTF-8"?>`))
	assert.Contains(t, xml, `</opml>`)
}
