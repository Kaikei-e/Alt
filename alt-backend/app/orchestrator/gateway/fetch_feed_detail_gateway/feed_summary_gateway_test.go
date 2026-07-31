package fetch_feed_detail_gateway

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These used to assert that a nil *AltDBRepository produced "database
// connection not available" on every call. That test could only ever fail if
// someone deleted the nil check, and it passed for a gateway that had never
// been wired at all — which is the failure ADR-000928 is about. With the
// summary read served by alt-data-hub the constructor refuses a nil store
// outright, so the interesting questions moved: does a miss stay a miss, and
// does an error stay an error.

type summaryStoreStub struct {
	summary *domain.FeedSummary
	err     error
	gotURL  *url.URL
}

func (s *summaryStoreStub) FetchFeedSummary(_ context.Context, feedURL *url.URL) (*domain.FeedSummary, error) {
	s.gotURL = feedURL
	return s.summary, s.err
}

func TestFeedSummaryGateway_FetchFeedDetails(t *testing.T) {
	feedURL, err := url.Parse("https://example.com/feed.xml")
	require.NoError(t, err)

	tests := []struct {
		name  string
		store *summaryStoreStub
		want  *domain.FeedSummary
		// wantErr is the whole point of the miss case: a summary that has not
		// been generated yet is nil-without-error, because the summarise path
		// reads that as "go generate one" rather than as a failure.
		wantErr bool
	}{
		{
			name:  "returns the summary",
			store: &summaryStoreStub{summary: &domain.FeedSummary{Summary: "要約"}},
			want:  &domain.FeedSummary{Summary: "要約"},
		},
		{
			name:  "a missing summary is nil without error",
			store: &summaryStoreStub{},
			want:  nil,
		},
		{
			name:    "a data plane failure surfaces",
			store:   &summaryStoreStub{err: errors.New("connection reset")},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := NewFeedSummaryGateway(tt.store)

			got, err := gateway.FetchFeedDetails(context.Background(), feedURL)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, feedURL, tt.store.gotURL)
		})
	}
}

// A gateway built with no store refuses at construction rather than at the
// first request. The alternative — a nil check on every call — makes "nobody
// wired this" indistinguishable from "the database is down", which is the
// silent-fallback failure CLAUDE.md rule 8 forbids.
func TestNewFeedSummaryGateway_RefusesNilStore(t *testing.T) {
	assert.Panics(t, func() { NewFeedSummaryGateway(nil) })
}
