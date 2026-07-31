package alt_db

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestFetchTrendStats_NilPool(t *testing.T) {
	repo := NewAltDBRepository(nil)
	assert.Nil(t, repo, "repository should be nil when pool is nil")
}

func TestFetchTrendStats_CancelledContext(t *testing.T) {
	repo := &DashboardRepository{pool: nil}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.FetchTrendStatsForUser(ctx, statsTestUserID, "24h")
	assert.Error(t, err, "should return error with cancelled context")
}

// TestFetchTrendStats_ZeroUserRefused replaces the missing-user-context test
// that stood here before ADR-000954 Wave 3 batch 5.
//
// The owner is an argument now, so the failure this guards against changed
// shape: not "nobody is signed in" but "the caller passed the zero UUID". The
// query has no other tenant predicate, so an unguarded zero owner would return
// an empty series that looks exactly like a quiet week.
func TestFetchTrendStats_ZeroUserRefused(t *testing.T) {
	repo := &DashboardRepository{pool: nil}

	_, err := repo.FetchTrendStatsForUser(context.Background(), uuid.Nil, "24h")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authentication required")
}

// TestFetchTrendStats_UnsupportedWindowRefused pins that the closed set is
// enforced before any connection is used: the window selects both the lower
// bound and the date_trunc unit, so there is no sensible behaviour for a value
// outside it.
func TestFetchTrendStats_UnsupportedWindowRefused(t *testing.T) {
	repo := &DashboardRepository{pool: nil}

	_, err := repo.FetchTrendStatsForUser(context.Background(), statsTestUserID, "90d")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported window")
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
			"COUNT(DISTINCT asumm.article_id) AS summarized",
			"FULL OUTER JOIN summarized_buckets sb ON ab.bucket = sb.bucket",
			"FULL OUTER JOIN feed_activity fa ON COALESCE(ab.bucket, sb.bucket) = fa.bucket",
		}, false},
		{"daily", "daily", []string{
			"date_trunc('day'",
			"a.user_id = $2",
			"COUNT(DISTINCT asumm.article_id) AS summarized",
			"FULL OUTER JOIN summarized_buckets sb ON ab.bucket = sb.bucket",
			"FULL OUTER JOIN feed_activity fa ON COALESCE(ab.bucket, sb.bucket) = fa.bucket",
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
