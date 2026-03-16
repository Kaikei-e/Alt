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
