package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

const (
	leakedSecret = "supersecret-db-password"
	leakedPath   = "/var/lib/postgresql/secret-cluster/pgdata"
)

func wrappedInternalErr() error {
	return fmt.Errorf("pq: connect %s password=%s: %w", leakedPath, leakedSecret, errors.New("connection refused"))
}

func assertNoInternalLeak(t *testing.T, body string) {
	t.Helper()
	assert.NotContains(t, body, leakedSecret)
	assert.NotContains(t, body, leakedPath)
}

type fakeSnapshotRepo struct {
	listErr   error
	latestErr error
	maxSeqErr error
}

func (f *fakeSnapshotRepo) InsertSnapshot(context.Context, *sovereign_db.SnapshotMetadata) error {
	return nil
}

func (f *fakeSnapshotRepo) UpdateSnapshotStatus(context.Context, uuid.UUID, string) error {
	return nil
}

func (f *fakeSnapshotRepo) GetLatestValidSnapshot(context.Context) (*sovereign_db.SnapshotMetadata, error) {
	if f.latestErr != nil {
		return nil, f.latestErr
	}
	return &sovereign_db.SnapshotMetadata{}, nil
}

func (f *fakeSnapshotRepo) ListSnapshots(context.Context, int) ([]sovereign_db.SnapshotMetadata, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return nil, nil
}

func (f *fakeSnapshotRepo) ExportTableToWriter(context.Context, string, io.Writer) (int64, error) {
	return 0, nil
}

func (f *fakeSnapshotRepo) GetMaxEventSeq(context.Context) (int64, error) {
	if f.maxSeqErr != nil {
		return 0, f.maxSeqErr
	}
	return 1, nil
}

func (f *fakeSnapshotRepo) GetTableRowCount(context.Context, string) (int, error) {
	return 0, nil
}

func (f *fakeSnapshotRepo) GetActiveProjectionVersion(context.Context) (*sovereign_db.ProjectionVersion, error) {
	return nil, nil
}

func snapshotMux(t *testing.T, repo SnapshotRepository) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	NewSnapshotHandler(repo, t.TempDir(), "test-build", "00001").RegisterRoutes(mux)
	return mux
}

func TestCreateSnapshot_DoesNotLeakInternalError(t *testing.T) {
	repo := &fakeSnapshotRepo{maxSeqErr: wrappedInternalErr()}

	req := httptest.NewRequest(http.MethodPost, "/admin/snapshots/create", nil)
	rec := httptest.NewRecorder()
	snapshotMux(t, repo).ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"snapshot failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}

func TestListSnapshots_DoesNotLeakInternalError(t *testing.T) {
	repo := &fakeSnapshotRepo{listErr: wrappedInternalErr()}

	req := httptest.NewRequest(http.MethodGet, "/admin/snapshots/list", nil)
	rec := httptest.NewRecorder()
	snapshotMux(t, repo).ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"snapshot list failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}

func TestGetLatestSnapshot_DoesNotLeakInternalError(t *testing.T) {
	repo := &fakeSnapshotRepo{latestErr: wrappedInternalErr()}

	req := httptest.NewRequest(http.MethodGet, "/admin/snapshots/latest", nil)
	rec := httptest.NewRecorder()
	snapshotMux(t, repo).ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"snapshot latest failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}
