package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"alt/orchestrator/gateway/rag_gateway"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeOwnerBackfillClient struct {
	batchesReceived  [][]rag_gateway.OwnerBackfillItem
	returnUpdated    int64
	returnAlreadySet int64
	returnNotFound   int64
	statusCode       int
	statusBody       string
	err              error
}

func (f *fakeOwnerBackfillClient) BackfillDocumentOwnersWithResponse(
	_ context.Context,
	body rag_gateway.BackfillDocumentOwnersJSONRequestBody,
	_ ...rag_gateway.RequestEditorFn,
) (*rag_gateway.BackfillDocumentOwnersResponse, error) {
	if f.err != nil {
		return nil, f.err
	}

	f.batchesReceived = append(f.batchesReceived, body.Items)

	code := f.statusCode
	if code == 0 {
		code = http.StatusOK
	}

	resp := &rag_gateway.BackfillDocumentOwnersResponse{
		HTTPResponse: &http.Response{
			StatusCode: code,
			Status:     http.StatusText(code),
		},
		Body: []byte(f.statusBody),
	}

	if code == http.StatusOK {
		resp.JSON200 = &rag_gateway.OwnerBackfillResponse{
			Updated:    f.returnUpdated,
			AlreadySet: f.returnAlreadySet,
			NotFound:   f.returnNotFound,
		}
	}

	return resp, nil
}

func TestProcessPairs_BatchingAndSumming(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Create 1250 test pairs
	const totalItems = 1250
	const batchSize = 500
	pairs := make([]ArticleOwnerPair, totalItems)
	for i := 0; i < totalItems; i++ {
		pairs[i] = ArticleOwnerPair{
			ArticleID: fmt.Sprintf("article-%04d", i),
			UserID:    "user-uuid-1234",
		}
	}

	fakeClient := &fakeOwnerBackfillClient{
		returnUpdated:    400,
		returnAlreadySet: 80,
		returnNotFound:   20,
	}

	stats, err := ProcessPairs(context.Background(), fakeClient, pairs, batchSize, false, logger)
	require.NoError(t, err)

	// 1250 items with batch size 500 => 3 batches (500, 500, 250)
	require.Len(t, fakeClient.batchesReceived, 3)
	assert.Len(t, fakeClient.batchesReceived[0], 500)
	assert.Len(t, fakeClient.batchesReceived[1], 500)
	assert.Len(t, fakeClient.batchesReceived[2], 250)

	assert.Equal(t, int64(3), stats.TotalBatches)
	assert.Equal(t, int64(1250), stats.TotalProcessed)
	assert.Equal(t, int64(400*3), stats.TotalUpdated)
	assert.Equal(t, int64(80*3), stats.TotalAlreadySet)
	assert.Equal(t, int64(20*3), stats.TotalNotFound)
}

func TestProcessPairs_DryRun(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	pairs := []ArticleOwnerPair{
		{ArticleID: "a1", UserID: "u1"},
		{ArticleID: "a2", UserID: "u2"},
	}

	fakeClient := &fakeOwnerBackfillClient{
		returnUpdated: 2,
	}

	stats, err := ProcessPairs(context.Background(), fakeClient, pairs, 500, true, logger)
	require.NoError(t, err)

	assert.Empty(t, fakeClient.batchesReceived, "dry run must not make RPC calls")
	assert.Equal(t, int64(2), stats.TotalProcessed)
	assert.Equal(t, int64(0), stats.TotalUpdated)
}

func TestProcessPairs_ExitsNonZeroOn4xx(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	pairs := []ArticleOwnerPair{
		{ArticleID: "a1", UserID: "u1"},
	}

	fakeClient := &fakeOwnerBackfillClient{
		statusCode: http.StatusBadRequest,
		statusBody: `{"error":"invalid user_id"}`,
	}

	_, err := ProcessPairs(context.Background(), fakeClient, pairs, 500, false, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestProcessPairs_ExitsNonZeroOn5xx(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	pairs := []ArticleOwnerPair{
		{ArticleID: "a1", UserID: "u1"},
	}

	fakeClient := &fakeOwnerBackfillClient{
		statusCode: http.StatusInternalServerError,
		statusBody: `{"error":"database connection failed"}`,
	}

	_, err := ProcessPairs(context.Background(), fakeClient, pairs, 500, false, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestProcessPairs_ExitsNonZeroOnNetworkError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	pairs := []ArticleOwnerPair{
		{ArticleID: "a1", UserID: "u1"},
	}

	fakeClient := &fakeOwnerBackfillClient{
		err: errors.New("connection refused"),
	}

	_, err := ProcessPairs(context.Background(), fakeClient, pairs, 500, false, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

type fakeArticlePageFetcher struct {
	pages         [][]ArticleOwnerPair
	callCount     int
	startAfterIDs []string
	err           error
}

func (f *fakeArticlePageFetcher) FetchPage(_ context.Context, startAfterID string, _ int) ([]ArticleOwnerPair, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.startAfterIDs = append(f.startAfterIDs, startAfterID)
	if f.callCount >= len(f.pages) {
		return nil, nil
	}
	page := f.pages[f.callCount]
	f.callCount++
	return page, nil
}

func TestStreamAndBackfill_KeysetPaginationAndDelegation(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	fetcher := &fakeArticlePageFetcher{
		pages: [][]ArticleOwnerPair{
			{
				{ArticleID: "art-01", UserID: "user-1"},
				{ArticleID: "art-02", UserID: "user-2"},
			},
			{
				{ArticleID: "art-03", UserID: "user-1"},
			},
		},
	}

	fakeClient := &fakeOwnerBackfillClient{
		returnUpdated: 1,
	}

	stats, err := streamAndBackfill(context.Background(), fetcher, fakeClient, 2, "", false, logger)
	require.NoError(t, err)

	assert.Equal(t, int64(3), stats.TotalProcessed)
	assert.Equal(t, int64(2), stats.TotalBatches) // 1 full batch of 2, 1 batch of 1
	assert.Equal(t, []string{"", "art-02"}, fetcher.startAfterIDs)
	require.Len(t, fakeClient.batchesReceived, 2)
	assert.Len(t, fakeClient.batchesReceived[0], 2)
	assert.Len(t, fakeClient.batchesReceived[1], 1)
}

func TestStreamAndBackfill_DryRun_NoRPCs(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	fetcher := &fakeArticlePageFetcher{
		pages: [][]ArticleOwnerPair{
			{
				{ArticleID: "art-01", UserID: "user-1"},
				{ArticleID: "art-02", UserID: "user-2"},
			},
		},
	}

	fakeClient := &fakeOwnerBackfillClient{}

	stats, err := streamAndBackfill(context.Background(), fetcher, fakeClient, 2, "", true, logger)
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.TotalProcessed)
	assert.Equal(t, int64(0), stats.TotalUpdated)
	assert.Empty(t, fakeClient.batchesReceived, "dry-run must not send any RPC batches to rag-orchestrator")
}

func TestStreamAndBackfill_ResumeFromStartAfterID(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	fetcher := &fakeArticlePageFetcher{
		pages: [][]ArticleOwnerPair{
			{
				{ArticleID: "art-50", UserID: "user-1"},
			},
		},
	}

	fakeClient := &fakeOwnerBackfillClient{}

	_, err := streamAndBackfill(context.Background(), fetcher, fakeClient, 10, "art-49", false, logger)
	require.NoError(t, err)

	require.NotEmpty(t, fetcher.startAfterIDs)
	assert.Equal(t, "art-49", fetcher.startAfterIDs[0], "first page fetch must resume from start-after-id")
}

func TestStreamAndBackfill_FetchError_ReturnsError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	fetcher := &fakeArticlePageFetcher{
		err: errors.New("db query error"),
	}

	fakeClient := &fakeOwnerBackfillClient{}

	_, err := streamAndBackfill(context.Background(), fetcher, fakeClient, 10, "", false, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db query error")
}

func TestStreamAndBackfill_ClientError_ReturnsError(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	fetcher := &fakeArticlePageFetcher{
		pages: [][]ArticleOwnerPair{
			{
				{ArticleID: "art-01", UserID: "user-1"},
			},
		},
	}

	fakeClient := &fakeOwnerBackfillClient{
		statusCode: http.StatusBadRequest,
		statusBody: `{"error":"invalid user_id"}`,
	}

	_, err := streamAndBackfill(context.Background(), fetcher, fakeClient, 10, "", false, logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}
