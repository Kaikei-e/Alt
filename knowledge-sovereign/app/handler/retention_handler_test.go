package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

type fakeRetentionRepo struct {
	partitions  map[string][]sovereign_db.PartitionInfo
	snapshot    *sovereign_db.SnapshotMetadata
	logs        []sovereign_db.RetentionLogEntry
	listErr     error
	snapshotErr error
	logsErr     error
}

func (f *fakeRetentionRepo) ListPartitions(_ context.Context, tableName string) ([]sovereign_db.PartitionInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.partitions[tableName], nil
}

func (f *fakeRetentionRepo) ExportTableToWriter(_ context.Context, _ string, _ io.Writer) (int64, error) {
	return 0, errors.New("ExportTableToWriter must not be reached")
}

func (f *fakeRetentionRepo) InsertRetentionLog(_ context.Context, _ sovereign_db.RetentionLogEntry) error {
	return nil
}

func (f *fakeRetentionRepo) ListRetentionLogs(_ context.Context, _ int) ([]sovereign_db.RetentionLogEntry, error) {
	if f.logsErr != nil {
		return nil, f.logsErr
	}
	return f.logs, nil
}

func (f *fakeRetentionRepo) GetLatestValidSnapshot(_ context.Context) (*sovereign_db.SnapshotMetadata, error) {
	if f.snapshotErr != nil {
		return nil, f.snapshotErr
	}
	return f.snapshot, nil
}

func (f *fakeRetentionRepo) GetMaxEventSeq(_ context.Context) (int64, error) {
	return 0, nil
}

func retentionMux(repo RetentionRepository, archiveDir string) *http.ServeMux {
	mux := http.NewServeMux()
	NewRetentionHandler(repo, archiveDir).RegisterRoutes(mux)
	return mux
}

// actionsField returns the raw JSON of the response's `actions` key, so the
// assertion sees `null` vs `[]` — decoding into []retentionAction would
// collapse both into an empty Go slice and pass either way.
func actionsField(t *testing.T, body []byte) string {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &fields), "body: %s", body)
	raw, ok := fields["actions"]
	require.True(t, ok, "response must always carry an actions key, body: %s", body)
	return string(raw)
}

// The admin panel iterates actions unconditionally, so a run that planned
// nothing must answer an empty array — the same nil-to-empty normalization the
// list endpoints already do before encoding.
func TestRunRetention_PlansNothingAndStillEncodesAnActionsArray(t *testing.T) {
	repo := &fakeRetentionRepo{snapshot: &sovereign_db.SnapshotMetadata{Status: "valid"}}

	req := httptest.NewRequest(http.MethodPost, "/admin/retention/run", strings.NewReader(`{"dry_run":true}`))
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Equal(t, "[]", actionsField(t, rec.Body.Bytes()))
}

// The missing-snapshot precondition answers 200 with the error text in the
// body, so that path must carry an iterable actions array too. The message is
// the static precondition text — it names no internals, and hiding it behind
// the generic "retention failed" would strip the one hint the operator can
// act on (create a snapshot first).
func TestRunRetention_MissingSnapshotStillEncodesAnActionsArray(t *testing.T) {
	repo := &fakeRetentionRepo{}

	req := httptest.NewRequest(http.MethodPost, "/admin/retention/run", strings.NewReader(`{"dry_run":false}`))
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "[]", actionsField(t, rec.Body.Bytes()))
	assert.Contains(t, rec.Body.String(), `"error":"no valid snapshot found; create a snapshot before running retention"`)
}

// A failing snapshot *lookup* is an internal error, not the missing-snapshot
// precondition, and must stay generic.
func TestRunRetention_SnapshotLookupErrorStaysGeneric(t *testing.T) {
	repo := &fakeRetentionRepo{snapshotErr: wrappedInternalErr()}

	req := httptest.NewRequest(http.MethodPost, "/admin/retention/run", strings.NewReader(`{"dry_run":false}`))
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"error":"retention failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}

func TestRunRetention_InvalidBodyDoesNotLeakDecoderError(t *testing.T) {
	repo := &fakeRetentionRepo{snapshot: &sovereign_db.SnapshotMetadata{Status: "valid"}}

	req := httptest.NewRequest(http.MethodPost, "/admin/retention/run", strings.NewReader(`{not-json`))
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"invalid request body"`)
	assert.NotContains(t, rec.Body.String(), "invalid character")
	assert.NotContains(t, rec.Body.String(), "not-json")
}

func TestRunRetention_DoesNotLeakInternalError(t *testing.T) {
	repo := &fakeRetentionRepo{
		snapshot: &sovereign_db.SnapshotMetadata{Status: "valid"},
		listErr:  wrappedInternalErr(),
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/retention/run", strings.NewReader(`{"dry_run":true}`))
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"retention failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}

// logAction stores raw err.Error() in the retention log for operators reading
// the DB; the HTTP status surface must not replay it (CWE-209 — the same leak
// the direct response paths already scrub).
func TestRetentionStatus_DoesNotLeakStoredErrorMessage(t *testing.T) {
	repo := &fakeRetentionRepo{
		logs: []sovereign_db.RetentionLogEntry{{
			Action:       "export",
			TargetTable:  "knowledge_events",
			Status:       "failed",
			ErrorMessage: wrappedInternalErr().Error(),
		}},
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/retention/status", nil)
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"status":"failed"`)
	assert.Contains(t, rec.Body.String(), `"error_message":"retention failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}

func TestRetentionStatus_DoesNotLeakInternalError(t *testing.T) {
	repo := &fakeRetentionRepo{logsErr: wrappedInternalErr()}

	req := httptest.NewRequest(http.MethodGet, "/admin/retention/status", nil)
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"retention status failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}

func TestEligiblePartitions_DoesNotLeakInternalError(t *testing.T) {
	repo := &fakeRetentionRepo{listErr: wrappedInternalErr()}

	req := httptest.NewRequest(http.MethodGet, "/admin/retention/eligible", nil)
	rec := httptest.NewRecorder()
	retentionMux(repo, t.TempDir()).ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"error":"retention eligible failed"`)
	assertNoInternalLeak(t, rec.Body.String())
}
