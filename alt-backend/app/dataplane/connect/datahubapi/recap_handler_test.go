package datahubapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"alt/dataplane/usecase/feeds_in_window_usecase"
	"alt/dataplane/usecase/recap_articles_usecase"
	"alt/domain"
	datahubv1 "alt/gen/proto/services/datahub/v1"
)

// fakeRecapArticlesUsecase is a testify-less inline mock for the handler's
// recap article window fetch.
type fakeRecapArticlesUsecase struct {
	err      error
	lastCall struct {
		page     int
		pageSize int
		from     time.Time
		to       time.Time
	}
	returnPage *domain.RecapArticlesPage
}

func (f *fakeRecapArticlesUsecase) Execute(_ context.Context, input recap_articles_usecase.Input) (*domain.RecapArticlesPage, error) {
	f.lastCall.page = input.Page
	f.lastCall.pageSize = input.PageSize
	f.lastCall.from = input.From
	f.lastCall.to = input.To
	return f.returnPage, f.err
}

func TestListRecapArticles_SuccessMapsDomainToProto(t *testing.T) {
	articleID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	title := "Test Article"
	sourceURL := "https://example.com/a"
	langHint := "en"
	publishedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)

	uc := &fakeRecapArticlesUsecase{
		returnPage: &domain.RecapArticlesPage{
			Total:    1,
			Page:     1,
			PageSize: 500,
			HasMore:  false,
			Articles: []domain.RecapArticle{{
				ID:          articleID,
				Title:       &title,
				FullText:    "Body text here.",
				SourceURL:   &sourceURL,
				LangHint:    &langHint,
				PublishedAt: &publishedAt,
			}},
		},
	}

	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	pageReq := int32(2)
	pageSizeReq := int32(100)
	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
		From:     "2026-04-14T00:00:00Z",
		To:       "2026-04-15T00:00:00Z",
		Page:     &pageReq,
		PageSize: &pageSizeReq,
	})

	resp, err := h.ListRecapArticles(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.GetTotal() != 1 {
		t.Errorf("Total = %d, want 1", resp.Msg.GetTotal())
	}
	if resp.Msg.GetRange().GetFrom() != "2026-04-14T00:00:00Z" {
		t.Errorf("Range.From = %q", resp.Msg.GetRange().GetFrom())
	}
	if len(resp.Msg.GetArticles()) != 1 {
		t.Fatalf("Articles len = %d, want 1", len(resp.Msg.GetArticles()))
	}
	got := resp.Msg.GetArticles()[0]
	if got.GetArticleId() != articleID.String() {
		t.Errorf("ArticleId = %q", got.GetArticleId())
	}
	if got.Title == nil || got.GetTitle() != title {
		t.Errorf("Title = %v", got.Title)
	}
	if got.GetFulltext() != "Body text here." {
		t.Errorf("Fulltext = %q", got.GetFulltext())
	}
	if got.PublishedAt == nil || got.GetPublishedAt() != "2026-04-15T12:00:00Z" {
		t.Errorf("PublishedAt = %v", got.PublishedAt)
	}
	if uc.lastCall.page != 2 || uc.lastCall.pageSize != 100 {
		t.Errorf("usecase got page=%d size=%d", uc.lastCall.page, uc.lastCall.pageSize)
	}
}

func TestListRecapArticles_InvalidFromMissing(t *testing.T) {
	uc := &fakeRecapArticlesUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{To: "2026-04-15T00:00:00Z"})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecapArticles_InvalidFromNotRFC3339(t *testing.T) {
	uc := &fakeRecapArticlesUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{From: "yesterday", To: "2026-04-15T00:00:00Z"})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecapArticles_UsecaseErrorMapsToInvalidArgument(t *testing.T) {
	uc := &fakeRecapArticlesUsecase{err: errors.New("page_size must be <= 2000")}
	h, _, _, _, _, _ := setupHandler(t)
	WithRecapArticlesUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
		From: "2026-04-14T00:00:00Z",
		To:   "2026-04-15T00:00:00Z",
	})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListRecapArticles_NotConfiguredReturnsUnimplemented(t *testing.T) {
	h, _, _, _, _, _ := setupHandler(t)
	// No WithRecapArticlesUsecase call.

	req := connect.NewRequest(&datahubv1.ListRecapArticlesRequest{
		From: "2026-04-14T00:00:00Z",
		To:   "2026-04-15T00:00:00Z",
	})
	_, err := h.ListRecapArticles(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeUnimplemented {
		t.Errorf("expected Unimplemented, got %v", err)
	}
}

// ── ListFeedsInWindow ──

type fakeFeedsInWindowUsecase struct {
	err      error
	lastCall struct {
		page     int
		pageSize int
		from     time.Time
		to       time.Time
	}
	returnPage *domain.FeedsInWindowPage
}

func (f *fakeFeedsInWindowUsecase) Execute(_ context.Context, input feeds_in_window_usecase.Input) (*domain.FeedsInWindowPage, error) {
	f.lastCall.page = input.Page
	f.lastCall.pageSize = input.PageSize
	f.lastCall.from = input.From
	f.lastCall.to = input.To
	return f.returnPage, f.err
}

func TestListFeedsInWindow_SuccessMapsDomainToProto(t *testing.T) {
	feedID := "feed-123"
	title := "Example Headline"
	description := "<p>Example lede.</p>"
	websiteURL := "https://example.com/post"
	pubDate := time.Date(2026, 3, 20, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 3, 20, 10, 5, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 20, 10, 5, 0, 0, time.UTC)
	articleID := "art-123"
	feedLinkID := "link-456"
	ogImageURL := "https://example.com/og.png"

	uc := &fakeFeedsInWindowUsecase{
		returnPage: &domain.FeedsInWindowPage{
			Total:    2064,
			Page:     1,
			PageSize: 500,
			HasMore:  true,
			Feeds: []domain.FeedRow{{
				ID:          feedID,
				Title:       title,
				Description: description,
				WebsiteURL:  websiteURL,
				PubDate:     pubDate,
				CreatedAt:   createdAt,
				UpdatedAt:   updatedAt,
				ArticleID:   &articleID,
				IsRead:      false,
				FeedLinkID:  &feedLinkID,
				OgImageURL:  &ogImageURL,
			}},
		},
	}

	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	pageReq := int32(1)
	pageSizeReq := int32(500)
	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{
		From:     "2026-03-19T00:00:00Z",
		To:       "2026-03-26T00:00:00Z",
		Page:     &pageReq,
		PageSize: &pageSizeReq,
	})

	resp, err := h.ListFeedsInWindow(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Msg.GetTotal() != 2064 {
		t.Errorf("Total = %d, want 2064", resp.Msg.GetTotal())
	}
	if resp.Msg.GetPage() != 1 {
		t.Errorf("Page = %d, want 1", resp.Msg.GetPage())
	}
	if resp.Msg.GetPageSize() != 500 {
		t.Errorf("PageSize = %d, want 500", resp.Msg.GetPageSize())
	}
	if !resp.Msg.GetHasMore() {
		t.Errorf("HasMore = %v, want true", resp.Msg.GetHasMore())
	}
	if len(resp.Msg.GetFeeds()) != 1 {
		t.Fatalf("Feeds len = %d, want 1", len(resp.Msg.GetFeeds()))
	}

	f := resp.Msg.GetFeeds()[0]
	if f.GetId() != feedID {
		t.Errorf("Id = %q, want %q", f.GetId(), feedID)
	}
	if f.GetTitle() != title {
		t.Errorf("Title = %q, want %q", f.GetTitle(), title)
	}
	if f.GetDescription() != description {
		t.Errorf("Description = %q, want %q", f.GetDescription(), description)
	}
	if f.GetWebsiteUrl() != websiteURL {
		t.Errorf("WebsiteUrl = %q, want %q", f.GetWebsiteUrl(), websiteURL)
	}
	if f.GetPubDate() == nil || !f.GetPubDate().AsTime().Equal(pubDate) {
		t.Errorf("PubDate = %v, want %v", f.GetPubDate(), pubDate)
	}
	if f.GetCreatedAt() == nil || !f.GetCreatedAt().AsTime().Equal(createdAt) {
		t.Errorf("CreatedAt = %v, want %v", f.GetCreatedAt(), createdAt)
	}
	if f.GetUpdatedAt() == nil || !f.GetUpdatedAt().AsTime().Equal(updatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", f.GetUpdatedAt(), updatedAt)
	}
	if f.ArticleId == nil || f.GetArticleId() != articleID {
		t.Errorf("ArticleId = %v, want %v", f.ArticleId, articleID)
	}
	if f.GetIsRead() != false {
		t.Errorf("IsRead = %v, want false", f.GetIsRead())
	}
	if f.FeedLinkId == nil || f.GetFeedLinkId() != feedLinkID {
		t.Errorf("FeedLinkId = %v, want %v", f.FeedLinkId, feedLinkID)
	}
	if f.OgImageUrl == nil || f.GetOgImageUrl() != ogImageURL {
		t.Errorf("OgImageUrl = %v, want %v", f.OgImageUrl, ogImageURL)
	}

	if uc.lastCall.page != 1 || uc.lastCall.pageSize != 500 {
		t.Errorf("usecase got page=%d size=%d", uc.lastCall.page, uc.lastCall.pageSize)
	}
}

func TestListFeedsInWindow_InvalidFromMissing(t *testing.T) {
	uc := &fakeFeedsInWindowUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{To: "2026-03-26T00:00:00Z"})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListFeedsInWindow_InvalidFromNotRFC3339(t *testing.T) {
	uc := &fakeFeedsInWindowUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{From: "not-a-date", To: "2026-03-26T00:00:00Z"})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListFeedsInWindow_InvalidToMissing(t *testing.T) {
	uc := &fakeFeedsInWindowUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{From: "2026-03-19T00:00:00Z"})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListFeedsInWindow_InvalidToNotRFC3339(t *testing.T) {
	uc := &fakeFeedsInWindowUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{From: "2026-03-19T00:00:00Z", To: "not-a-date"})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListFeedsInWindow_InvalidPage(t *testing.T) {
	uc := &fakeFeedsInWindowUsecase{}
	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	pageReq := int32(0)
	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{
		From: "2026-03-19T00:00:00Z",
		To:   "2026-03-26T00:00:00Z",
		Page: &pageReq,
	})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListFeedsInWindow_UsecaseErrorMapsToInvalidArgument(t *testing.T) {
	uc := &fakeFeedsInWindowUsecase{err: errors.New("from must be before to")}
	h, _, _, _, _, _ := setupHandler(t)
	WithFeedsInWindowUsecase(uc)(h)

	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{
		From: "2026-03-26T00:00:00Z",
		To:   "2026-03-19T00:00:00Z",
	})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeInvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListFeedsInWindow_NotConfiguredReturnsUnimplemented(t *testing.T) {
	h, _, _, _, _, _ := setupHandler(t)
	// No WithFeedsInWindowUsecase call.

	req := connect.NewRequest(&datahubv1.ListFeedsInWindowRequest{
		From: "2026-03-19T00:00:00Z",
		To:   "2026-03-26T00:00:00Z",
	})
	_, err := h.ListFeedsInWindow(context.Background(), req)
	if err == nil {
		t.Fatal("expected error")
	}
	connErr := &connect.Error{}
	ok := errors.As(err, &connErr)
	if !ok || connErr.Code() != connect.CodeUnimplemented {
		t.Errorf("expected Unimplemented, got %v", err)
	}
}
