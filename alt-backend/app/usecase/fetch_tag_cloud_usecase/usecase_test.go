package fetch_tag_cloud_usecase

import (
	"alt/domain"
	"alt/utils/logger"
	"context"
	"errors"
	"testing"
)

// mockFetchTagCloudPort implements fetch_tag_cloud_port.FetchTagCloudPort for testing.
type mockFetchTagCloudPort struct {
	items         []*domain.TagCloudItem
	err           error
	cooccurrences []*domain.TagCooccurrence
	cooccErr      error
}

func (m *mockFetchTagCloudPort) FetchTagCloud(_ context.Context, _ int) ([]*domain.TagCloudItem, error) {
	return m.items, m.err
}

func (m *mockFetchTagCloudPort) FetchTagCooccurrences(_ context.Context, _ []string) ([]*domain.TagCooccurrence, error) {
	return m.cooccurrences, m.cooccErr
}

func TestFetchTagCloudUsecase_Execute(t *testing.T) {
	logger.InitLogger()

	ctx := context.Background()

	mockItems := []*domain.TagCloudItem{
		{TagName: "AI", ArticleCount: 142},
		{TagName: "Rust", ArticleCount: 87},
		{TagName: "Go", ArticleCount: 65},
	}

	tests := []struct {
		name      string
		limit     int
		mockItems []*domain.TagCloudItem
		mockErr   error
		wantCount int
		wantErr   bool
	}{
		{
			name:      "success with default limit",
			limit:     0,
			mockItems: mockItems,
			wantCount: 3,
		},
		{
			name:      "success with custom limit",
			limit:     100,
			mockItems: mockItems,
			wantCount: 3,
		},
		{
			name:    "limit exceeds max returns error",
			limit:   501,
			wantErr: true,
		},
		{
			name:      "negative limit uses default",
			limit:     -1,
			mockItems: mockItems,
			wantCount: 3,
		},
		{
			name:    "port returns error",
			limit:   100,
			mockErr: errors.New("database error"),
			wantErr: true,
		},
		{
			name:      "empty result",
			limit:     100,
			mockItems: nil,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port := &mockFetchTagCloudPort{
				items: tt.mockItems,
				err:   tt.mockErr,
			}

			usecase := NewFetchTagCloudUsecase(port)
			got, err := usecase.Execute(ctx, tt.limit)

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && len(got) != tt.wantCount {
				t.Errorf("Execute() got %d items, want %d", len(got), tt.wantCount)
			}
		})
	}
}
