package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"knowledge-sovereign/driver/sovereign_db"
)

type fakeRebuildRepo struct {
	calls  []sovereign_db.ProjectionRebuildTarget
	result sovereign_db.ProjectionRebuildResult
	err    error
}

func (f *fakeRebuildRepo) RebuildProjection(_ context.Context, target sovereign_db.ProjectionRebuildTarget) (sovereign_db.ProjectionRebuildResult, error) {
	f.calls = append(f.calls, target)
	if f.err != nil {
		return sovereign_db.ProjectionRebuildResult{}, f.err
	}
	return f.result, nil
}

func rebuildMux(repo ProjectionRebuildRepository) *http.ServeMux {
	mux := http.NewServeMux()
	NewProjectionRebuildHandler(repo).RegisterRoutes(mux)
	return mux
}

func TestProjectionRebuildHandler_RebuildsAllowlistedTarget(t *testing.T) {
	repo := &fakeRebuildRepo{result: sovereign_db.ProjectionRebuildResult{
		Target:           "knowledge-home",
		Tables:           []string{"knowledge_home_items", "today_digest_view", "recall_candidate_view"},
		TablesTruncated:  3,
		ProjectorName:    "knowledge-home-projector",
		CheckpointBefore: 1379513,
	}}

	req := httptest.NewRequest(http.MethodPost, "/admin/projections/rebuild",
		strings.NewReader(`{"target":"knowledge-home"}`))
	rec := httptest.NewRecorder()
	rebuildMux(repo).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	require.Len(t, repo.calls, 1)
	assert.Equal(t, "knowledge-home", repo.calls[0].Name())
	assert.Equal(t, "knowledge-home-projector", repo.calls[0].ProjectorName())

	var got sovereign_db.ProjectionRebuildResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "knowledge-home", got.Target)
	assert.Equal(t, 3, got.TablesTruncated)
	assert.Equal(t, int64(1379513), got.CheckpointBefore,
		"the operator must get back the checkpoint the rebuild reset from")
}

// The endpoint takes a target name, never a table name. An operator (or a
// scripted caller) must not be able to reach the event log or the dedupe
// registry through it, and a rejected request must never touch the database.
func TestProjectionRebuildHandler_RejectsAnythingOutsideTheAllowlist(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"the event log", `{"target":"knowledge_events"}`},
		{"the dedupe registry", `{"target":"knowledge_event_dedupes"}`},
		{"the user event log", `{"target":"knowledge_user_events"}`},
		{"a read-model table name", `{"target":"knowledge_home_items"}`},
		{"an unknown target", `{"target":"everything"}`},
		{"an empty target", `{"target":""}`},
		{"no target field", `{}`},
		{"an empty body", ``},
		{"malformed json", `{`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRebuildRepo{}
			req := httptest.NewRequest(http.MethodPost, "/admin/projections/rebuild", strings.NewReader(tc.body))
			rec := httptest.NewRecorder()
			rebuildMux(repo).ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Empty(t, repo.calls, "a rejected request must never reach the database")
		})
	}
}

func TestProjectionRebuildHandler_RepositoryFailureIsServerError(t *testing.T) {
	repo := &fakeRebuildRepo{err: errors.New("lock timeout")}

	req := httptest.NewRequest(http.MethodPost, "/admin/projections/rebuild",
		strings.NewReader(`{"target":"knowledge-trail"}`))
	rec := httptest.NewRecorder()
	rebuildMux(repo).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "lock timeout")
}

// The targets endpoint is the operator's preview: it says exactly which tables
// a rebuild would empty and which checkpoint it would reset, before anything is
// destroyed.
func TestProjectionRebuildHandler_ListsRebuildableTargets(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/projections/rebuild/targets", nil)
	rec := httptest.NewRecorder()
	rebuildMux(&fakeRebuildRepo{}).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Targets []struct {
			Target        string   `json:"target"`
			Tables        []string `json:"tables"`
			ProjectorName string   `json:"projector_name"`
		} `json:"targets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Targets, 2)

	byName := map[string][]string{}
	projectors := map[string]string{}
	for _, tgt := range body.Targets {
		byName[tgt.Target] = tgt.Tables
		projectors[tgt.Target] = tgt.ProjectorName
	}

	assert.Equal(t, []string{"knowledge_home_items", "today_digest_view", "recall_candidate_view"}, byName["knowledge-home"])
	assert.Equal(t, "knowledge-home-projector", projectors["knowledge-home"])
	assert.Equal(t, []string{"knowledge_trail_footprints", "knowledge_trail_branches", "knowledge_trail_act_outcomes"}, byName["knowledge-trail"])
	assert.Equal(t, "knowledge-trail-projector", projectors["knowledge-trail"])

	for _, tgt := range body.Targets {
		for _, table := range tgt.Tables {
			assert.NotContains(t, []string{"knowledge_events", "knowledge_event_dedupes", "knowledge_user_events"}, table,
				"the advertised targets must never include a source-of-truth table")
		}
	}
}

// Both routes must live under /admin/, because that prefix is what
// requireAdminToken gates on the metrics port. A rebuild endpoint registered
// anywhere else would be reachable unauthenticated.
func TestProjectionRebuildHandler_RoutesAreAdminScoped(t *testing.T) {
	mux := rebuildMux(&fakeRebuildRepo{})

	for _, probe := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/admin/projections/rebuild"},
		{http.MethodGet, "/admin/projections/rebuild/targets"},
	} {
		_, pattern := mux.Handler(httptest.NewRequest(probe.method, probe.path, nil))
		require.NotEmpty(t, pattern, "%s %s must be registered", probe.method, probe.path)
		assert.True(t, strings.Contains(pattern, "/admin/"),
			"route %q must sit under /admin/ so the admin token gate covers it", pattern)
	}
}
