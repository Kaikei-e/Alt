//go:build contract

package contract

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// cmd/datahub wires the unscoped tag cloud for callers that send no user id,
// which is what the rag-orchestrator deployed before per-user scoping sends.
// The stub has to answer them the same way or its deployed pact verifies red
// against a provider that serves it.
func TestProviderStub_FetchTagCloudAnswersCallersWithoutUserID(t *testing.T) {
	url := fmt.Sprintf("http://127.0.0.1:%d/services.datahub.v1.DataHubService/FetchTagCloud", startStubServer(t))

	resp, err := http.Post(url, "application/json", strings.NewReader(`{"limit":300}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Tags []struct {
			TagName      string `json:"tagName"`
			ArticleCount int    `json:"articleCount"`
		} `json:"tags"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.NotEmpty(t, body.Tags)
}
