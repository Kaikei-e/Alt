package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

type fakeStorageRepo struct {
	stats []sovereign_db.TableStorageInfo
	err   error
}

func (f *fakeStorageRepo) GetStorageStats(_ context.Context) ([]sovereign_db.TableStorageInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.stats, nil
}

func (f *fakeStorageRepo) ListPartitions(_ context.Context, _ string) ([]sovereign_db.PartitionInfo, error) {
	return nil, nil
}

func TestStorageHandler(t *testing.T) {
	t.Run("returns table storage stats JSON", func(t *testing.T) {
		repo := &fakeStorageRepo{
			stats: []sovereign_db.TableStorageInfo{
				{TableName: "knowledge_events", RowCount: 42, TotalSize: "128 kB", TotalBytes: 131072},
			},
		}
		mux := http.NewServeMux()
		NewStorageHandler(repo).RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/admin/storage/stats", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var resp storageStatsResponse
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.Tables, 1)
		assert.Equal(t, "knowledge_events", resp.Tables[0].TableName)
		assert.Equal(t, int64(42), resp.Tables[0].RowCount)
	})

	t.Run("normalizes nil slice to empty JSON array", func(t *testing.T) {
		mux := http.NewServeMux()
		NewStorageHandler(&fakeStorageRepo{stats: nil}).RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/admin/storage/stats", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.JSONEq(t, `{"tables":[]}`, rec.Body.String())
	})

	t.Run("sanitizes internal repository errors on 500", func(t *testing.T) {
		mux := http.NewServeMux()
		NewStorageHandler(&fakeStorageRepo{err: errors.New("pq: fatal database error")}).RegisterRoutes(mux)

		req := httptest.NewRequest(http.MethodGet, "/admin/storage/stats", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)

		require.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.NotContains(t, rec.Body.String(), "pq: fatal")
		assert.Contains(t, rec.Body.String(), "failed to get storage stats")
	})
}
