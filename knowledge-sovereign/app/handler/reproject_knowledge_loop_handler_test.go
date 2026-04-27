package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"knowledge-sovereign/driver/sovereign_db"
)

type fakeReprojectRepo struct {
	result         sovereign_db.KnowledgeLoopReprojectResult
	err            error
	called         int
	checkpointSeq  int64
	checkpointErr  error
	checkpointName string
}

func (f *fakeReprojectRepo) TruncateKnowledgeLoopProjections(_ context.Context) (sovereign_db.KnowledgeLoopReprojectResult, error) {
	f.called++
	return f.result, f.err
}

func (f *fakeReprojectRepo) GetProjectionCheckpoint(_ context.Context, projectorName string) (int64, error) {
	f.checkpointName = projectorName
	return f.checkpointSeq, f.checkpointErr
}

func TestKnowledgeLoopReprojectHandler_HappyPath(t *testing.T) {
	repo := &fakeReprojectRepo{
		result: sovereign_db.KnowledgeLoopReprojectResult{
			EntriesTruncated:  42,
			SessionTruncated:  3,
			SurfacesTruncated: 12,
			CheckpointReset:   true,
		},
	}
	h := NewKnowledgeLoopReprojectHandler(repo)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/admin/knowledge-loop/reproject", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if repo.called != 1 {
		t.Errorf("repo called %d times; want 1", repo.called)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["ok"] != true {
		t.Errorf("ok = %v; want true", body["ok"])
	}
	if body["entries_truncated"] != float64(42) {
		t.Errorf("entries_truncated = %v; want 42", body["entries_truncated"])
	}
	if body["checkpoint_reset"] != true {
		t.Errorf("checkpoint_reset = %v; want true", body["checkpoint_reset"])
	}
}

func TestKnowledgeLoopReprojectHandler_RepoErrorReturns500(t *testing.T) {
	repo := &fakeReprojectRepo{err: errors.New("boom")}
	h := NewKnowledgeLoopReprojectHandler(repo)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/admin/knowledge-loop/reproject", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"ok":false`) {
		t.Errorf("body = %q; want ok:false", body)
	}
	if !strings.Contains(body, "boom") {
		t.Errorf("body = %q; want error message echoed", body)
	}
}

func TestKnowledgeLoopReprojectHandler_StatusReturnsVersionAndCheckpoint(t *testing.T) {
	repo := &fakeReprojectRepo{checkpointSeq: 42}
	h := NewKnowledgeLoopReprojectHandler(repo)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/admin/knowledge-loop/reproject/status", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	if repo.checkpointName != "knowledge-loop-projector" {
		t.Errorf("checkpoint queried for %q; want knowledge-loop-projector",
			repo.checkpointName)
	}
	if repo.called != 0 {
		t.Errorf("status endpoint must not mutate; truncate called %d", repo.called)
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["why_mapping_version"] == nil {
		t.Error("why_mapping_version missing")
	}
	if body["last_event_seq"] != float64(42) {
		t.Errorf("last_event_seq = %v; want 42", body["last_event_seq"])
	}
	if body["projector_name"] != "knowledge-loop-projector" {
		t.Errorf("projector_name = %v; want knowledge-loop-projector", body["projector_name"])
	}
}

// GET to the reproject route must NOT trigger reproject — the admin button
// always uses POST and a runaway crawler / curl typo cannot wipe projections
// just by issuing a GET.
func TestKnowledgeLoopReprojectHandler_GetIsNotAllowed(t *testing.T) {
	repo := &fakeReprojectRepo{}
	h := NewKnowledgeLoopReprojectHandler(repo)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/admin/knowledge-loop/reproject", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d; want 405", rec.Code)
	}
	if repo.called != 0 {
		t.Errorf("repo called %d times on GET; want 0 — destructive op must be POST-only", repo.called)
	}
}
