package global_search_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Mock implementations ---

type mockArticleSearch struct {
	result *domain.ArticleSearchSection
	err    error
}

func (m *mockArticleSearch) SearchArticlesForGlobal(_ context.Context, _ string, _ string, _ int) (*domain.ArticleSearchSection, error) {
	return m.result, m.err
}

type mockRecapSearch struct {
	result *domain.RecapSearchSection
	err    error
}

func (m *mockRecapSearch) SearchRecapsForGlobal(_ context.Context, _ string, _ int) (*domain.RecapSearchSection, error) {
	return m.result, m.err
}

type mockTagSearch struct {
	result *domain.TagSearchSection
	err    error
}

func (m *mockTagSearch) SearchTagsByPrefix(_ context.Context, _ string, _ int) (*domain.TagSearchSection, error) {
	return m.result, m.err
}

func userCtx() context.Context {
	return domain.SetUserContext(context.Background(), &domain.UserContext{
		UserID:    uuid.MustParse("01020304-0506-0708-090a-0b0c0d0e0f10"),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		ExpiresAt: time.Now().Add(1 * time.Hour),
	})
}

func TestGlobalSearchUsecase_AllSucceed(t *testing.T) {
	logger.InitLogger()

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{result: &domain.ArticleSearchSection{
			Hits:           []domain.GlobalArticleHit{{ID: "a1", Title: "AI Article"}},
			EstimatedTotal: 1,
		}},
		&mockRecapSearch{result: &domain.RecapSearchSection{
			Hits:           []domain.GlobalRecapHit{{ID: "r1", Genre: "Technology"}},
			EstimatedTotal: 1,
		}},
		&mockTagSearch{result: &domain.TagSearchSection{
			Hits:  []domain.GlobalTagHit{{TagName: "AI", ArticleCount: 50}},
			Total: 1,
		}},
	)

	result, err := uc.Execute(userCtx(), "AI", 5, 3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Query != "AI" {
		t.Errorf("expected query 'AI', got %q", result.Query)
	}
	if len(result.DegradedSections) != 0 {
		t.Errorf("expected no degraded sections, got %v", result.DegradedSections)
	}
	if result.Articles == nil || len(result.Articles.Hits) != 1 {
		t.Error("expected 1 article hit")
	}
	if result.Recaps == nil || len(result.Recaps.Hits) != 1 {
		t.Error("expected 1 recap hit")
	}
	if result.Tags == nil || len(result.Tags.Hits) != 1 {
		t.Error("expected 1 tag hit")
	}
}

func TestGlobalSearchUsecase_ArticlesFail_OtherSucceed(t *testing.T) {
	logger.InitLogger()

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{err: errors.New("meilisearch down")},
		&mockRecapSearch{result: &domain.RecapSearchSection{
			Hits: []domain.GlobalRecapHit{{ID: "r1"}},
		}},
		&mockTagSearch{result: &domain.TagSearchSection{
			Hits: []domain.GlobalTagHit{{TagName: "AI"}},
		}},
	)

	result, err := uc.Execute(userCtx(), "AI", 5, 3, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.DegradedSections) != 1 || result.DegradedSections[0] != "articles" {
		t.Errorf("expected ['articles'] degraded, got %v", result.DegradedSections)
	}
	if result.Articles != nil {
		t.Error("expected nil articles section on failure")
	}
	if result.Recaps == nil || len(result.Recaps.Hits) != 1 {
		t.Error("expected 1 recap hit")
	}
	if result.Tags == nil || len(result.Tags.Hits) != 1 {
		t.Error("expected 1 tag hit")
	}
}

func TestGlobalSearchUsecase_AllFail(t *testing.T) {
	logger.InitLogger()

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{err: errors.New("articles down")},
		&mockRecapSearch{err: errors.New("recaps down")},
		&mockTagSearch{err: errors.New("tags down")},
	)

	_, err := uc.Execute(userCtx(), "AI", 5, 3, 10)
	if err == nil {
		t.Fatal("expected error when all sections fail, got nil")
	}
}

func TestGlobalSearchUsecase_EmptyQuery(t *testing.T) {
	logger.InitLogger()

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{},
		&mockRecapSearch{},
		&mockTagSearch{},
	)

	_, err := uc.Execute(userCtx(), "", 5, 3, 10)
	if err == nil {
		t.Fatal("expected error for empty query, got nil")
	}
}

func TestGlobalSearchUsecase_DefaultLimits(t *testing.T) {
	logger.InitLogger()

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{result: &domain.ArticleSearchSection{Hits: []domain.GlobalArticleHit{}}},
		&mockRecapSearch{result: &domain.RecapSearchSection{Hits: []domain.GlobalRecapHit{}}},
		&mockTagSearch{result: &domain.TagSearchSection{Hits: []domain.GlobalTagHit{}}},
	)

	result, err := uc.Execute(userCtx(), "test", 0, 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Query != "test" {
		t.Errorf("expected query 'test', got %q", result.Query)
	}
}

func TestGlobalSearchUsecase_NoUserContext(t *testing.T) {
	logger.InitLogger()

	uc := NewGlobalSearchUsecase(
		&mockArticleSearch{},
		&mockRecapSearch{},
		&mockTagSearch{},
	)

	_, err := uc.Execute(context.Background(), "AI", 5, 3, 10)
	if err == nil {
		t.Fatal("expected error for missing user context, got nil")
	}
}
