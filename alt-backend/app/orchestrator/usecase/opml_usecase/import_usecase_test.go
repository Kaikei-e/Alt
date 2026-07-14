package opml_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestImportOPMLUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)

	opmlXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Test Feeds</title></head>
  <body>
    <outline text="Example" type="rss" xmlUrl="https://example.com/feed.xml" />
    <outline text="Blog" type="rss" xmlUrl="https://blog.test.org/rss" />
  </body>
</opml>`)

	expectedResult := &domain.OPMLImportResult{
		Total:    2,
		Imported: 2,
		Skipped:  0,
		Failed:   0,
	}

	mockPort.EXPECT().RegisterFeedLinkBulk(gomock.Any(), []string{
		"https://example.com/feed.xml",
		"https://blog.test.org/rss",
	}).Return(expectedResult, nil)

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), opmlXML)

	require.NoError(t, err)
	assert.Equal(t, expectedResult, result)
}

func TestImportOPMLUsecase_Execute_NestedOutlines(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)

	opmlXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Nested Feeds</title></head>
  <body>
    <outline text="Tech">
      <outline text="Go Blog" type="rss" xmlUrl="https://go.dev/blog/feed.atom" />
      <outline text="Rust Blog" type="rss" xmlUrl="https://blog.rust-lang.org/feed.xml" />
    </outline>
    <outline text="News" type="rss" xmlUrl="https://news.example.com/rss" />
  </body>
</opml>`)

	expectedResult := &domain.OPMLImportResult{
		Total:    3,
		Imported: 3,
	}

	mockPort.EXPECT().RegisterFeedLinkBulk(gomock.Any(), []string{
		"https://go.dev/blog/feed.atom",
		"https://blog.rust-lang.org/feed.xml",
		"https://news.example.com/rss",
	}).Return(expectedResult, nil)

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), opmlXML)

	require.NoError(t, err)
	assert.Equal(t, 3, result.Total)
	assert.Equal(t, 3, result.Imported)
}

func TestImportOPMLUsecase_Execute_EmptyOPML(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)
	// No RegisterFeedLinkBulk call expected

	opmlXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Empty</title></head>
  <body></body>
</opml>`)

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), opmlXML)

	require.NoError(t, err)
	assert.Equal(t, 0, result.Total)
}

func TestImportOPMLUsecase_Execute_InvalidXML(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), []byte("not valid xml"))

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "parse OPML")
}

func TestImportOPMLUsecase_Execute_DuplicateURLsInOPML(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)

	opmlXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Dupes</title></head>
  <body>
    <outline text="Feed 1" type="rss" xmlUrl="https://example.com/feed.xml" />
    <outline text="Feed 1 Copy" type="rss" xmlUrl="https://example.com/feed.xml" />
    <outline text="Feed 2" type="rss" xmlUrl="https://other.com/rss" />
  </body>
</opml>`)

	// Duplicates within the OPML are de-duped before calling the port
	expectedResult := &domain.OPMLImportResult{
		Total:    2,
		Imported: 2,
	}

	mockPort.EXPECT().RegisterFeedLinkBulk(gomock.Any(), []string{
		"https://example.com/feed.xml",
		"https://other.com/rss",
	}).Return(expectedResult, nil)

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), opmlXML)

	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
}

func TestImportOPMLUsecase_Execute_PortError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)

	opmlXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Test</title></head>
  <body>
    <outline text="Feed" type="rss" xmlUrl="https://example.com/feed.xml" />
  </body>
</opml>`)

	mockPort.EXPECT().RegisterFeedLinkBulk(gomock.Any(), gomock.Any()).Return(nil, errors.New("db error"))

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), opmlXML)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestImportOPMLUsecase_Execute_OutlinesWithoutXMLURL(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImportOPMLPort(ctrl)

	// Category outlines without xmlUrl should be skipped
	opmlXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>Test</title></head>
  <body>
    <outline text="Category Only" />
    <outline text="Feed" type="rss" xmlUrl="https://example.com/feed.xml" />
  </body>
</opml>`)

	expectedResult := &domain.OPMLImportResult{
		Total:    1,
		Imported: 1,
	}

	mockPort.EXPECT().RegisterFeedLinkBulk(gomock.Any(), []string{
		"https://example.com/feed.xml",
	}).Return(expectedResult, nil)

	usecase := NewImportOPMLUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), opmlXML)

	require.NoError(t, err)
	assert.Equal(t, 1, result.Total)
}
