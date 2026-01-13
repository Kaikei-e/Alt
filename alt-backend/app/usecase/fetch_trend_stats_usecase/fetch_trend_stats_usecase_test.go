package fetch_trend_stats_usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"alt/port/trend_stats_port"
	"alt/utils/logger"

	"github.com/stretchr/testify/assert"
)

func init() {
	logger.InitLogger()
}

// MockTrendStatsPort is a mock implementation of TrendStatsPort
type MockTrendStatsPort struct {
	result *trend_stats_port.TrendDataResponse
	err    error
}

func (m *MockTrendStatsPort) Execute(ctx context.Context, window string) (*trend_stats_port.TrendDataResponse, error) {
	return m.result, m.err
}

func TestFetchTrendStatsUsecase_Execute_Success(t *testing.T) {
	mockResult := &trend_stats_port.TrendDataResponse{
		DataPoints: []trend_stats_port.TrendDataPoint{
			{
				Timestamp:    time.Now().Add(-1 * time.Hour),
				Articles:     10,
				Summarized:   5,
				FeedActivity: 2,
			},
			{
				Timestamp:    time.Now(),
				Articles:     15,
				Summarized:   8,
				FeedActivity: 3,
			},
		},
		Granularity: "hourly",
		Window:      "24h",
	}

	mockPort := &MockTrendStatsPort{
		result: mockResult,
		err:    nil,
	}

	usecase := NewFetchTrendStatsUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), "24h")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 2, len(result.DataPoints))
	assert.Equal(t, "hourly", result.Granularity)
	assert.Equal(t, "24h", result.Window)
}

func TestFetchTrendStatsUsecase_Execute_PortError(t *testing.T) {
	mockPort := &MockTrendStatsPort{
		result: nil,
		err:    errors.New("database error"),
	}

	usecase := NewFetchTrendStatsUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), "24h")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to fetch trend stats")
}

func TestFetchTrendStatsUsecase_Execute_EmptyResult(t *testing.T) {
	mockResult := &trend_stats_port.TrendDataResponse{
		DataPoints:  []trend_stats_port.TrendDataPoint{},
		Granularity: "hourly",
		Window:      "4h",
	}

	mockPort := &MockTrendStatsPort{
		result: mockResult,
		err:    nil,
	}

	usecase := NewFetchTrendStatsUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), "4h")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 0, len(result.DataPoints))
}

func TestFetchTrendStatsUsecase_Execute_AllWindows(t *testing.T) {
	windows := []string{"4h", "24h", "3d", "7d"}

	for _, window := range windows {
		t.Run(window, func(t *testing.T) {
			mockResult := &trend_stats_port.TrendDataResponse{
				DataPoints:  []trend_stats_port.TrendDataPoint{},
				Granularity: "hourly",
				Window:      window,
			}

			mockPort := &MockTrendStatsPort{
				result: mockResult,
				err:    nil,
			}

			usecase := NewFetchTrendStatsUsecase(mockPort)
			result, err := usecase.Execute(context.Background(), window)

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, window, result.Window)
		})
	}
}

// TestTrendStatsPortInterface ensures we can use the usecase with the port interface
func TestNewFetchTrendStatsUsecase(t *testing.T) {
	mockPort := &MockTrendStatsPort{}
	usecase := NewFetchTrendStatsUsecase(mockPort)
	assert.NotNil(t, usecase)
}
