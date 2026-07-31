package feed_url_to_id_gateway

import (
	"context"
	"errors"
	"testing"

	"alt/utils/logger"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubResolver records what it was asked and answers per URL.
type stubResolver struct {
	answers map[string]string
	errs    map[string]error
	asked   []string
}

func (s *stubResolver) GetFeedIDByURL(_ context.Context, feedURL string) (string, error) {
	s.asked = append(s.asked, feedURL)
	if err, ok := s.errs[feedURL]; ok {
		return "", err
	}
	if id, ok := s.answers[feedURL]; ok {
		return id, nil
	}
	return "", connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
}

func notFound() error {
	return connect.NewError(connect.CodeNotFound, errors.New("feed not found"))
}

// TestGetFeedIDByURL is the retry ladder this gateway exists for. Every case
// here is about *how many* lookups happen and *which* URL each one carries,
// because that is the whole behaviour that stayed on this side when the lookup
// itself moved to alt-data-hub (ADR-000954 Wave 3, catalog W2-10).
func TestGetFeedIDByURL(t *testing.T) {
	logger.InitLogger()

	tests := []struct {
		name      string
		feedURL   string
		resolver  *stubResolver
		wantID    string
		wantErr   bool
		wantAsked []string
	}{
		{
			// The literal URL is tried first and, when it hits, nothing else
			// happens. Normalising first would have been simpler and wrong:
			// the registrar stored some URLs in non-canonical form, and those
			// rows are only reachable by the string the caller has.
			name:      "literal hit asks once",
			feedURL:   "https://example.com/rss.xml",
			resolver:  &stubResolver{answers: map[string]string{"https://example.com/rss.xml": "feed-1"}},
			wantID:    "feed-1",
			wantAsked: []string{"https://example.com/rss.xml"},
		},
		{
			// The case the retry was added for: a trailing slash makes a
			// registered feed look unregistered, which used to surface as a
			// 262-line burst in the log rather than as a feed.
			name:    "not found falls back to the canonical form",
			feedURL: "https://Example.com/rss.xml/",
			resolver: &stubResolver{answers: map[string]string{
				"https://example.com/rss.xml": "feed-1",
			}},
			wantID:    "feed-1",
			wantAsked: []string{"https://Example.com/rss.xml/", "https://example.com/rss.xml"},
		},
		{
			// A URL that is already canonical gets one attempt, not two. The
			// second would send identical bytes and cost a round trip that
			// cannot change the answer.
			name:      "canonical url is not retried",
			feedURL:   "https://example.com/rss.xml",
			resolver:  &stubResolver{},
			wantErr:   true,
			wantAsked: []string{"https://example.com/rss.xml"},
		},
		{
			// A fault is not an absence. Retrying it would double the load on
			// a provider that is already failing, and — worse — a caller that
			// saw the second NotFound would conclude the feed is unregistered
			// when the truth is that nobody looked.
			name:    "a fault is not retried",
			feedURL: "https://Example.com/rss.xml/",
			resolver: &stubResolver{errs: map[string]error{
				"https://Example.com/rss.xml/": connect.NewError(connect.CodeUnavailable, errors.New("data hub down")),
			}},
			wantErr:   true,
			wantAsked: []string{"https://Example.com/rss.xml/"},
		},
		{
			// Both attempts miss: the caller gets the error, and gets it from
			// the URL it actually asked about.
			name:    "both attempts miss",
			feedURL: "https://Example.com/rss.xml/",
			resolver: &stubResolver{errs: map[string]error{
				"https://Example.com/rss.xml/": notFound(),
				"https://example.com/rss.xml":  notFound(),
			}},
			wantErr:   true,
			wantAsked: []string{"https://Example.com/rss.xml/", "https://example.com/rss.xml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gateway := NewFeedURLToIDGateway(tt.resolver)

			id, err := gateway.GetFeedIDByURL(context.Background(), tt.feedURL)
			if tt.wantErr {
				require.Error(t, err)
				assert.Empty(t, id)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantID, id)
			}
			assert.Equal(t, tt.wantAsked, tt.resolver.asked)
		})
	}
}

// TestNewFeedURLToIDGatewayRefusesNil pins the replacement for the
// `if g.alt_db == nil` guard this gateway used to open with.
//
// That guard turned a wiring mistake into "database connection not available"
// on every call — a runtime symptom for a construction-time fault, and one that
// looks like an outage. Refusing at construction makes the same mistake a
// process that does not start (CLAUDE.md rule 8).
func TestNewFeedURLToIDGatewayRefusesNil(t *testing.T) {
	assert.Panics(t, func() { NewFeedURLToIDGateway(nil) })
}
