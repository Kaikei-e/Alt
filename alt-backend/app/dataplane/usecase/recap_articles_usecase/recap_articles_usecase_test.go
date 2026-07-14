package recap_articles_usecase

import (
	"alt/domain"
	"alt/mocks"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func sampleArticlePage() *domain.RecapArticlesPage {
	uid := uuid.New()
	return &domain.RecapArticlesPage{
		Total:    1,
		Page:     2,
		PageSize: 500,
		HasMore:  true,
		Articles: []domain.RecapArticle{
			{
				ID:       uid,
				FullText: "hello world",
			},
		},
	}
}

func TestRecapArticlesUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRecapArticlesPort(ctrl)
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 2000, MaxRangeDays: 8}
	usecase := NewRecapArticlesUsecase(repo, cfg)

	from := time.Now().Add(-48 * time.Hour)
	to := time.Now().Add(-24 * time.Hour)
	fields := []string{"title", "fulltext"}
	lang := "ja"
	input := Input{
		From:     from,
		To:       to,
		Page:     2,
		PageSize: 1200,
		LangHint: &lang,
		Fields:   fields,
	}

	expectedQuery := domain.RecapArticlesQuery{
		From:     from,
		To:       to,
		Page:     2,
		PageSize: 1200,
		LangHint: &lang,
		Fields:   fields,
	}

	page := sampleArticlePage()
	repo.EXPECT().FetchRecapArticles(gomock.Any(), expectedQuery).Return(page, nil)

	result, err := usecase.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, page, result)
}

func TestRecapArticlesUsecase_Execute_DefaultsAndValidation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRecapArticlesPort(ctrl)
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}
	usecase := NewRecapArticlesUsecase(repo, cfg)

	from := time.Now().Add(-72 * time.Hour)
	to := time.Now()
	input := Input{
		From:     from,
		To:       to,
		Page:     0,
		PageSize: 0,
		Fields:   []string{"title"},
	}

	expectedQuery := domain.RecapArticlesQuery{
		From:     from,
		To:       to,
		Page:     1,
		PageSize: cfg.DefaultPageSize,
		Fields:   []string{"title"},
	}

	page := sampleArticlePage()
	repo.EXPECT().FetchRecapArticles(gomock.Any(), expectedQuery).Return(page, nil)

	result, err := usecase.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, page, result)
}

func TestRecapArticlesUsecase_Execute_ValidationErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRecapArticlesPort(ctrl)
	cfg := Config{DefaultPageSize: 100, MaxPageSize: 500, MaxRangeDays: 5}
	usecase := NewRecapArticlesUsecase(repo, cfg)

	from := time.Now().Add(-10 * 24 * time.Hour)
	to := time.Now()

	tests := []struct {
		name  string
		input Input
	}{
		{
			name: "from after to",
			input: Input{
				From: to,
				To:   from,
			},
		},
		{
			name: "range too large",
			input: Input{
				From: from,
				To:   to,
			},
		},
		{
			name: "page invalid",
			input: Input{
				From: from.Add(6 * 24 * time.Hour),
				To:   to,
				Page: -1,
			},
		},
		{
			name: "page size too big",
			input: Input{
				From:     from.Add(6 * 24 * time.Hour),
				To:       to,
				Page:     1,
				PageSize: 1000,
			},
		},
		{
			name: "invalid field",
			input: Input{
				From:   from.Add(6 * 24 * time.Hour),
				To:     to,
				Page:   1,
				Fields: []string{"foo"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := usecase.Execute(context.Background(), tt.input)
			require.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestRecapArticlesUsecase_Execute_PropagatesRepoError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRecapArticlesPort(ctrl)
	cfg := Config{DefaultPageSize: 100, MaxPageSize: 200, MaxRangeDays: 7}
	usecase := NewRecapArticlesUsecase(repo, cfg)

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()
	input := Input{From: from, To: to, Page: 1, PageSize: 100}

	repoErr := errors.New("db down")
	repo.EXPECT().FetchRecapArticles(gomock.Any(), gomock.Any()).Return(nil, repoErr)

	result, err := usecase.Execute(context.Background(), input)
	assert.ErrorIs(t, err, repoErr)
	assert.Nil(t, result)
}
