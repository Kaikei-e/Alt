package preprocessor_summarize_gateway

import (
	"alt/domain"
	"alt/orchestrator/driver/preprocessor_client"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMapClientStatusToPortStatus(t *testing.T) {
	if mapClientStatusToPortStatus(nil) != nil {
		t.Fatal("expected nil for nil input")
	}

	src := &preprocessor_client.SummarizeStatus{
		JobID:        "job-1",
		Status:       "completed",
		Summary:      "A summary",
		ErrorMessage: "",
		ArticleID:    "article-1",
	}

	res := mapClientStatusToPortStatus(src)
	if res == nil {
		t.Fatal("expected non-nil result")
	}
	if res.JobID != "job-1" || res.Status != "completed" || res.Summary != "A summary" || res.ArticleID != "article-1" {
		t.Errorf("unexpected mapped result: %+v", res)
	}
}

func TestGateway_QueueSummarize_ForwardsOrigin(t *testing.T) {
	tests := []struct {
		name           string
		origin         domain.SummarizeOrigin
		expectedOrigin string
	}{
		{
			name:           "user origin",
			origin:         domain.SummarizeOriginUser,
			expectedOrigin: "user",
		},
		{
			name:           "system origin",
			origin:         domain.SummarizeOriginSystem,
			expectedOrigin: "system",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var receivedBody map[string]string
			decodeErrCh := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				decodeErr := json.NewDecoder(r.Body).Decode(&receivedBody)
				decodeErrCh <- decodeErr
				if decodeErr != nil {
					http.Error(w, decodeErr.Error(), http.StatusBadRequest)
					return
				}
				w.WriteHeader(http.StatusAccepted)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"job_id": "job-1",
					"status": "pending",
				})
			}))
			defer server.Close()

			gw := NewGateway(preprocessor_client.NewClientWithTransport(server.URL, server.Client().Transport))
			jobID, err := gw.QueueSummarize(context.Background(), "article-1", "title", tc.origin)
			require.NoError(t, err)
			require.NoError(t, <-decodeErrCh)
			assert.Equal(t, "job-1", jobID)
			assert.Equal(t, tc.expectedOrigin, receivedBody["origin"])
		})
	}
}

func TestGateway_QueueSummarize_RejectsInvalidOrigin(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	gw := NewGateway(preprocessor_client.NewClientWithTransport(server.URL, server.Client().Transport))
	_, err := gw.QueueSummarize(context.Background(), "article-1", "title", "")
	require.ErrorIs(t, err, domain.ErrInvalidSummarizeOrigin)
	assert.False(t, called)
}
