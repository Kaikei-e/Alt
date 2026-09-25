package fetch_article_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/orchestrator/port/scraping_policy_port"
	"context"
	"errors"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"
)

func TestFetchCompliantArticle_ScrapingPolicyDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockScrapingPolicy := mocks.NewMockScrapingPolicyPort(ctrl)

	usecase := NewArticleUsecaseWithScrapingPolicy(
		mockArticleFetcher, mockRobotsTxt, mockRepo, mockScrapingPolicy,
	)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockScrapingPolicy.EXPECT().CanFetchArticle(gomock.Any(), articleURLStr).Return(false, nil)
	mockRepo.EXPECT().SaveDeclinedDomain(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err == nil {
		t.Fatal("Expected ComplianceError, got nil")
	}
	var complianceErr *domain.ComplianceError
	if !errors.As(err, &complianceErr) {
		t.Errorf("Expected ComplianceError, got %T: %v", err, err)
	}
}

func TestFetchCompliantArticle_CrawlDelayIsTransientAndNotDeclined(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockScrapingPolicy := mocks.NewMockScrapingPolicyPort(ctrl)

	usecase := NewArticleUsecaseWithScrapingPolicy(
		mockArticleFetcher, mockRobotsTxt, mockRepo, mockScrapingPolicy,
	)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userContext := domain.UserContext{
		UserID: uuid.MustParse("00000000-0000-0000-0000-000000000001"),
	}

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)

	mockScrapingPolicy.EXPECT().
		CanFetchArticle(gomock.Any(), articleURLStr).
		Return(false, scraping_policy_port.ErrCrawlDelayNotElapsed)

	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err == nil {
		t.Fatal("expected a transient rate-limit error, got nil")
	}
	var rateErr *domain.RateLimitedError
	if !errors.As(err, &rateErr) {
		t.Fatalf("expected *domain.RateLimitedError, got %T: %v", err, err)
	}
	var complianceErr *domain.ComplianceError
	if errors.As(err, &complianceErr) {
		t.Error("a crawl-delay miss must not surface as a permanent ComplianceError")
	}
	if rateErr.RetryAfter <= 0 {
		t.Errorf("expected a positive RetryAfter so the client can back off, got %v", rateErr.RetryAfter)
	}
}

func TestFetchCompliantArticle_ScrapingPolicyAllowed(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)
	mockScrapingPolicy := mocks.NewMockScrapingPolicyPort(ctrl)

	usecase := NewArticleUsecaseWithScrapingPolicy(
		mockArticleFetcher, mockRobotsTxt, mockRepo, mockScrapingPolicy,
	)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></body></html>"
	expectedContentHTML := "<div><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></div>"
	articleID := "article-456"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockScrapingPolicy.EXPECT().CanFetchArticle(gomock.Any(), articleURLStr).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(articleID, nil)

	content, retID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected content %s, got %s", expectedContentHTML, content)
	}
	if retID != articleID {
		t.Errorf("Expected article ID %s, got %s", articleID, retID)
	}
	ctrl.Finish()
}

func TestFetchCompliantArticle_ScrapingPolicyNil_FallbackToRobotsTxt(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></body></html>"
	expectedContentHTML := "<div><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></div>"
	articleID := "article-789"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(articleID, nil)

	content, retID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected content %s, got %s", expectedContentHTML, content)
	}
	if retID != articleID {
		t.Errorf("Expected article ID %s, got %s", articleID, retID)
	}
	ctrl.Finish()
}

func TestFetchArticleUsecase_FetchCompliantArticle_DefersRAGToOutbox(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code with mocks and sanitization logic.</p></body></html>"
	expectedContentHTML := "<div><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code with mocks and sanitization logic.</p></div>"
	articleID := "article-123"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), expectedContentHTML).Return(articleID, nil)

	content, retArticleID, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if content != expectedContentHTML {
		t.Errorf("Expected content %s, got %s", expectedContentHTML, content)
	}
	if retArticleID == "" {
		t.Errorf("Expected non-empty article ID")
	}

	ctrl.Finish()
}

func TestFetchCompliantArticle_DoesNotCallRAGDirectly(t *testing.T) {
	ctrl := gomock.NewController(t)

	mockArticleFetcher := mocks.NewMockFetchArticlePort(ctrl)
	mockRobotsTxt := mocks.NewMockRobotsTxtPolicyPort(ctrl)
	mockRepo := mocks.NewMockArticleRepository(ctrl)

	usecase := NewArticleUsecase(mockArticleFetcher, mockRobotsTxt, mockRepo)

	articleURLStr := "https://example.com/article"
	articleURL, _ := url.Parse(articleURLStr)
	userID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	userContext := domain.UserContext{UserID: userID}
	rawHTML := "<html><body><p>Article content needs to be very long. We are adding more text to satisfy the 100 char limit. This is a very interesting article about testing Go code.</p></body></html>"
	articleID := "article-async"

	mockRepo.EXPECT().FetchArticleByURL(gomock.Any(), articleURLStr).Return(nil, nil)
	mockRepo.EXPECT().IsDomainDeclined(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	mockRobotsTxt.EXPECT().IsPathAllowed(gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	mockArticleFetcher.EXPECT().FetchArticleContents(gomock.Any(), articleURLStr).Return(&rawHTML, nil)
	mockRepo.EXPECT().SaveArticle(gomock.Any(), articleURLStr, gomock.Any(), gomock.Any()).Return(articleID, nil)

	_, _, _, err := usecase.FetchCompliantArticle(context.Background(), articleURL, userContext)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	ctrl.Finish()
}
