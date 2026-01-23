package alt_db

import (
	"context"
	"testing"
	"time"

	"alt/domain"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFetchTrendStats_NilPool(t *testing.T) {
	repo := NewAltDBRepository(nil)
	assert.Nil(t, repo, "repository should be nil when pool is nil")
}

func TestFetchTrendStats_CancelledContext(t *testing.T) {
	repo := &AltDBRepository{pool: nil}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Add user to context
	userCtx := &domain.UserContext{
		UserID:    uuid.New(),
		Email:     "test@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.New(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
	ctx = domain.SetUserContext(ctx, userCtx)

	_, err := repo.FetchTrendStats(ctx, "24h")
	assert.Error(t, err, "should return error with cancelled context")
}

func TestFetchTrendStats_NoUserContext(t *testing.T) {
	repo := &AltDBRepository{pool: nil}
	ctx := context.Background()

	_, err := repo.FetchTrendStats(ctx, "24h")
	assert.Error(t, err, "should return error when user context is missing")
	assert.Contains(t, err.Error(), "authentication required")
}

func TestParseWindow(t *testing.T) {
	tests := []struct {
		name        string
		window      string
		wantSeconds int
		wantGran    string
		wantErr     bool
	}{
		{"4 hours", "4h", 4 * 3600, "hourly", false},
		{"24 hours", "24h", 24 * 3600, "hourly", false},
		{"3 days", "3d", 3 * 24 * 3600, "daily", false},
		{"7 days", "7d", 7 * 24 * 3600, "daily", false},
		{"invalid", "invalid", 0, "", true},
		{"empty", "", 0, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seconds, granularity, err := parseWindow(tt.window)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantSeconds, seconds)
				assert.Equal(t, tt.wantGran, granularity)
			}
		})
	}
}

func TestBuildTrendQuery(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
		wantContain []string
		wantErr     bool
	}{
		{"hourly", "hourly", []string{
			"date_trunc('hour'",
			"a.user_id = $2",
			"LEFT JOIN article_summaries asumm ON a.id = asumm.article_id",
			"COUNT(DISTINCT asumm.article_id) AS summarized",
		}, false},
		{"daily", "daily", []string{
			"date_trunc('day'",
			"a.user_id = $2",
			"LEFT JOIN article_summaries asumm ON a.id = asumm.article_id",
			"COUNT(DISTINCT asumm.article_id) AS summarized",
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, err := buildTrendQuery(tt.granularity)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			for _, want := range tt.wantContain {
				assert.Contains(t, query, want)
			}
		})
	}
}

func TestBuildTrendQuery_InvalidGranularity(t *testing.T) {
	tests := []struct {
		name        string
		granularity string
	}{
		{"empty string", ""},
		{"sql injection attempt", "'; DROP TABLE articles; --"},
		{"invalid value", "weekly"},
		{"case sensitive", "HOURLY"},
		{"numeric", "123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := buildTrendQuery(tt.granularity)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "invalid granularity")
		})
	}
}
