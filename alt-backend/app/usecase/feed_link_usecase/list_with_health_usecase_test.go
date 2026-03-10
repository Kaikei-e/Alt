package feed_link_usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"alt/domain"
	"alt/mocks"
)

func TestListFeedLinksWithHealthUsecase_Execute_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFeedLinkPort(ctrl)

	expected := []*domain.FeedLinkWithHealth{
		{
			FeedLink:     domain.FeedLink{ID: uuid.New(), URL: "https://example.com/feed.xml"},
			Availability: &domain.FeedLinkAvailability{IsActive: true, ConsecutiveFailures: 0},
		},
		{
			FeedLink:     domain.FeedLink{ID: uuid.New(), URL: "https://blog.example.org/rss"},
			Availability: nil,
		},
	}

	mockPort.EXPECT().ListFeedLinksWithHealth(gomock.Any()).Return(expected, nil)

	usecase := NewListFeedLinksWithHealthUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestListFeedLinksWithHealthUsecase_Execute_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFeedLinkPort(ctrl)
	mockPort.EXPECT().ListFeedLinksWithHealth(gomock.Any()).Return(nil, errors.New("db error"))

	usecase := NewListFeedLinksWithHealthUsecase(mockPort)
	result, err := usecase.Execute(context.Background())

	assert.Error(t, err)
	assert.Nil(t, result)
}
