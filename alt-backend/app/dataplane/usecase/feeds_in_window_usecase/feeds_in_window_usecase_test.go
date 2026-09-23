package feeds_in_window_usecase

import (
	"alt/domain"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeFeedsInWindowPort struct {
	page      *domain.FeedsInWindowPage
	err       error
	lastQuery domain.FeedsInWindowQuery
}

func (f *fakeFeedsInWindowPort) FetchFeedsInWindow(_ context.Context, query domain.FeedsInWindowQuery) (*domain.FeedsInWindowPage, error) {
	f.lastQuery = query
	if f.err != nil {
		return nil, f.err
	}
	if f.page != nil {
		// Return copy
		cp := *f.page
		return &cp, nil
	}
	return &domain.FeedsInWindowPage{
		Total:    0,
		Page:     query.Page,
		PageSize: query.PageSize,
		HasMore:  false,
		Feeds:    []domain.FeedRow{},
	}, nil
}

func sampleFeedPage() *domain.FeedsInWindowPage {
	articleID := "art-001"
	feedLinkID := "feed-link-001"
	ogImageURL := "https://example.com/image.jpg"
	return &domain.FeedsInWindowPage{
		Total:    2064,
		Page:     1,
		PageSize: 500,
		HasMore:  true,
		Feeds: []domain.FeedRow{
			{
				ID:          "feed-001",
				Title:       "Example Feed",
				Description: "<p>Example HTML description</p>",
				WebsiteURL:  "https://example.com/post",
				PubDate:     time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
				CreatedAt:   time.Date(2026, 3, 20, 1, 0, 0, 0, time.UTC),
				UpdatedAt:   time.Date(2026, 3, 20, 1, 0, 0, 0, time.UTC),
				ArticleID:   &articleID,
				IsRead:      false,
				FeedLinkID:  &feedLinkID,
				OgImageURL:  &ogImageURL,
			},
		},
	}
}

func TestFeedsInWindowUsecase_Execute_Success(t *testing.T) {
	page := sampleFeedPage()
	repo := &fakeFeedsInWindowPort{page: page}
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}
	usecase := NewFeedsInWindowUsecase(repo, cfg)

	from := time.Now().Add(-48 * time.Hour)
	to := time.Now().Add(-24 * time.Hour)
	input := Input{
		From:     from,
		To:       to,
		Page:     1,
		PageSize: 500,
	}

	result, err := usecase.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 2064, result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 500, result.PageSize)
	assert.True(t, result.HasMore)
	assert.Len(t, result.Feeds, 1)
	assert.Equal(t, "feed-001", result.Feeds[0].ID)
	assert.Equal(t, from, repo.lastQuery.From)
	assert.Equal(t, to, repo.lastQuery.To)
	assert.Equal(t, 1, repo.lastQuery.Page)
	assert.Equal(t, 500, repo.lastQuery.PageSize)
}

func TestFeedsInWindowUsecase_Execute_Defaults(t *testing.T) {
	repo := &fakeFeedsInWindowPort{page: sampleFeedPage()}
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}
	usecase := NewFeedsInWindowUsecase(repo, cfg)

	from := time.Now().Add(-72 * time.Hour)
	to := time.Now()
	input := Input{
		From:     from,
		To:       to,
		Page:     0, // default to 1
		PageSize: 0, // default to 500
	}

	result, err := usecase.Execute(context.Background(), input)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 500, result.PageSize)
	assert.Equal(t, 1, repo.lastQuery.Page)
	assert.Equal(t, 500, repo.lastQuery.PageSize)
}

func TestFeedsInWindowUsecase_Execute_Clamping(t *testing.T) {
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}

	tests := []struct {
		name             string
		inputPageSize    int
		expectedPageSize int
	}{
		{
			name:             "page size zero defaults to 500",
			inputPageSize:    0,
			expectedPageSize: 500,
		},
		{
			name:             "page size exceeds 1000 clamped to 1000",
			inputPageSize:    1500,
			expectedPageSize: 1000,
		},
		{
			name:             "page size negative clamped to 1",
			inputPageSize:    -5,
			expectedPageSize: 1,
		},
		{
			name:             "page size within 1..1000 kept as is",
			inputPageSize:    250,
			expectedPageSize: 250,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeFeedsInWindowPort{}
			usecase := NewFeedsInWindowUsecase(repo, cfg)
			from := time.Now().Add(-24 * time.Hour)
			to := time.Now()

			_, err := usecase.Execute(context.Background(), Input{
				From:     from,
				To:       to,
				Page:     1,
				PageSize: tt.inputPageSize,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedPageSize, repo.lastQuery.PageSize)
		})
	}
}

func TestFeedsInWindowUsecase_Execute_HasMoreSemantics(t *testing.T) {
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}

	tests := []struct {
		name            string
		page            int
		pageSize        int
		total           int
		expectedHasMore bool
	}{
		{
			name:            "page 1 of 2064 has more",
			page:            1,
			pageSize:        500,
			total:           2064,
			expectedHasMore: true, // 1*500 < 2064
		},
		{
			name:            "page 4 of 2064 has more",
			page:            4,
			pageSize:        500,
			total:           2064,
			expectedHasMore: true, // 4*500 < 2064
		},
		{
			name:            "page 5 of 2064 does not have more",
			page:            5,
			pageSize:        500,
			total:           2064,
			expectedHasMore: false, // 5*500 >= 2064
		},
		{
			name:            "page 1 with total matching pageSize does not have more",
			page:            1,
			pageSize:        500,
			total:           500,
			expectedHasMore: false, // 1*500 >= 500
		},
		{
			name:            "total 0 does not have more",
			page:            1,
			pageSize:        500,
			total:           0,
			expectedHasMore: false, // 1*500 >= 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeFeedsInWindowPort{
				page: &domain.FeedsInWindowPage{
					Total:    tt.total,
					Page:     tt.page,
					PageSize: tt.pageSize,
				},
			}
			usecase := NewFeedsInWindowUsecase(repo, cfg)
			from := time.Now().Add(-24 * time.Hour)
			to := time.Now()

			res, err := usecase.Execute(context.Background(), Input{
				From:     from,
				To:       to,
				Page:     tt.page,
				PageSize: tt.pageSize,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.expectedHasMore, res.HasMore)
		})
	}
}

func TestFeedsInWindowUsecase_Execute_ValidationErrors(t *testing.T) {
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}
	usecase := NewFeedsInWindowUsecase(&fakeFeedsInWindowPort{}, cfg)

	from := time.Now().Add(-10 * 24 * time.Hour)
	to := time.Now()

	tests := []struct {
		name        string
		input       Input
		errContains string
	}{
		{
			name: "from is zero",
			input: Input{
				To: to,
			},
			errContains: "from/to parameters are required",
		},
		{
			name: "to is zero",
			input: Input{
				From: from,
			},
			errContains: "from/to parameters are required",
		},
		{
			name: "from after to",
			input: Input{
				From: to,
				To:   from,
			},
			errContains: "from must be before to",
		},
		{
			name: "from equal to to",
			input: Input{
				From: to,
				To:   to,
			},
			errContains: "from must be before to",
		},
		{
			name: "date range exceeds max days",
			input: Input{
				From: from,
				To:   to,
			},
			errContains: "date range exceeds",
		},
		{
			name: "page negative",
			input: Input{
				From: from.Add(8 * 24 * time.Hour),
				To:   to,
				Page: -1,
			},
			errContains: "page must be >= 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := usecase.Execute(context.Background(), tt.input)
			require.Error(t, err)
			assert.Nil(t, res)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestFeedsInWindowUsecase_Execute_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("db connection failed")
	repo := &fakeFeedsInWindowPort{err: repoErr}
	cfg := Config{DefaultPageSize: 500, MaxPageSize: 1000, MaxRangeDays: 7}
	usecase := NewFeedsInWindowUsecase(repo, cfg)

	from := time.Now().Add(-24 * time.Hour)
	to := time.Now()

	res, err := usecase.Execute(context.Background(), Input{From: from, To: to, Page: 1, PageSize: 500})
	require.Error(t, err)
	assert.ErrorIs(t, err, repoErr)
	assert.Nil(t, res)
}
