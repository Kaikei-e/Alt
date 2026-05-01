package knowledge_loop_usecase

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"alt/domain"

	"github.com/stretchr/testify/require"
)

// TestTransitionPolicyConformance pins the Knowledge Loop transition matrix
// to the canonical YAML at proto/alt/knowledge/loop/v1/loop_transition_policy.yaml.
// Any divergence between the YAML and ClassifyTransitionEvent fails the test —
// the YAML is the single source of truth (ADR-000876).
func TestTransitionPolicyConformance(t *testing.T) {
	policy := loadTransitionPolicyForTest(t)

	// Every allowed_edges entry must classify successfully via user_tap.
	// We use user_tap as the canonical trigger since it is universally
	// supported across all six edges.
	for _, e := range policy.AllowedEdges {
		from := stageEnum(e.From)
		to := stageEnum(e.To)
		eventType, err := ClassifyTransitionEvent(from, to, "TRANSITION_TRIGGER_USER_TAP")
		require.NoErrorf(t, err, "yaml edge %s->%s must be accepted by classifier", e.From, e.To)
		require.NotEmptyf(t, eventType, "yaml edge %s->%s must yield a non-empty event type", e.From, e.To)
	}

	// Forbidden pairs must reject with InvalidArgument.
	for _, e := range policy.ForbiddenEdges {
		from := stageEnum(e.From)
		to := stageEnum(e.To)
		_, err := ClassifyTransitionEvent(from, to, "TRANSITION_TRIGGER_USER_TAP")
		require.Errorf(t, err, "yaml forbidden edge %s->%s must be rejected", e.From, e.To)
		require.Truef(t, errors.Is(err, ErrInvalidArgument),
			"yaml forbidden edge %s->%s must reject with ErrInvalidArgument, got %v", e.From, e.To, err)
	}

	// Same-stage triggers must produce KnowledgeLoopDeferred or
	// KnowledgeLoopReviewed depending on intent.
	expectedSameStageEvents := map[string]string{
		"defer":         domain.EventKnowledgeLoopDeferred,
		"recheck":       domain.EventKnowledgeLoopReviewed,
		"archive":       domain.EventKnowledgeLoopReviewed,
		"mark_reviewed": domain.EventKnowledgeLoopReviewed,
	}
	for _, trig := range policy.SameStageTriggers {
		eventType, err := ClassifyTransitionEvent(stageEnum("orient"), stageEnum("orient"), triggerEnum(trig))
		require.NoErrorf(t, err, "yaml same-stage trigger %q must be accepted on identical from/to", trig)
		require.Equalf(t, expectedSameStageEvents[trig], eventType,
			"yaml same-stage trigger %q must classify to its canonical event type", trig)
	}
}

type policyEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type policyAllowedEdge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Triggers []string `json:"triggers"`
}

type transitionPolicy struct {
	Version           int                 `json:"version"`
	AllowedEdges      []policyAllowedEdge `json:"allowed_edges"`
	SameStageTriggers []string            `json:"same_stage_triggers"`
	ForbiddenEdges    []policyEdge        `json:"forbidden_edges"`
}

// loadTransitionPolicyForTest finds the proto JSON by walking up from this
// test file. Tests live deep in the alt-backend/app tree; the policy JSON
// lives in proto/alt/knowledge/loop/v1/. Walking up avoids hard-coding
// repo-relative paths in the test.
func loadTransitionPolicyForTest(t *testing.T) transitionPolicy {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller must succeed")

	dir := filepath.Dir(thisFile)
	for i := 0; i < 12; i++ {
		candidate := filepath.Join(dir, "proto", "alt", "knowledge", "loop", "v1", "loop_transition_policy.json")
		if _, err := os.Stat(candidate); err == nil {
			body, err := os.ReadFile(candidate)
			require.NoError(t, err)
			var p transitionPolicy
			require.NoError(t, json.Unmarshal(body, &p))
			require.Equal(t, 1, p.Version, "policy version must be 1; bump conformance test alongside any version change")
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("could not locate proto/alt/knowledge/loop/v1/loop_transition_policy.json from %s", thisFile)
	return transitionPolicy{}
}

func stageEnum(short string) string {
	switch strings.ToLower(short) {
	case "observe":
		return stageObserve
	case "orient":
		return stageOrient
	case "decide":
		return stageDecide
	case "act":
		return stageAct
	}
	return ""
}

func triggerEnum(short string) string {
	switch strings.ToLower(short) {
	case "user_tap":
		return triggerUserTap
	case "dwell":
		return triggerDwell
	case "keyboard":
		return triggerKeyboard
	case "programmatic":
		return triggerProgram
	case "defer":
		return triggerDefer
	case "recheck":
		return triggerRecheck
	case "archive":
		return triggerArchive
	case "mark_reviewed":
		return triggerMarkReviewed
	}
	return ""
}
