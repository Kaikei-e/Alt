package knowledge_loop_projector

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// seedActTargets materialises the JSON bytes the projector writes into
// knowledge_loop_entries.act_targets. Downstream chain:
//
//	JSONB column → alt-backend decodeActTargets → loopv1.ActTarget
//	                                            → FE mapActTargetTypeFromProto
//	                                            → "recap" string in actTargets[]
//
// The entire chain is deterministic, so testing the JSON shape end-to-end is
// sufficient — we don't need to spin up a DB to verify the projector's
// behaviour.

func TestSeedActTargets_NoRecapID_ReturnsNil(t *testing.T) {
	t.Parallel()
	out := seedActTargets(SurfaceScoreInputs{})
	require.Nil(t, out, "no act_targets should be seeded when no recap snapshot resolved")
}

func TestSeedActTargets_RecapID_WritesRecapTarget(t *testing.T) {
	t.Parallel()
	out := seedActTargets(SurfaceScoreInputs{
		RecapTopicSnapshotID: "11111111-1111-4111-8111-111111111111",
	})
	require.NotEmpty(t, out)

	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	require.Equal(t, "recap", raw[0]["target_type"])
	require.Equal(t, "11111111-1111-4111-8111-111111111111", raw[0]["target_ref"])
	require.Equal(t, "/recap/topic/11111111-1111-4111-8111-111111111111", raw[0]["route"])
}

// TestSeedActTargets_RouteIsAbsolutePath documents the contract the FE relies
// on: the route MUST start with "/" and MUST NOT contain ":". Together with
// the FE-side allowlist this prevents javascript: schemes / open-redirect
// vectors even if a future resolver leaks an attacker-controlled string into
// RecapTopicSnapshotID. The resolver itself UUID-validates the input; this
// test is the projector's belt-and-suspenders.
func TestSeedActTargets_RouteIsAbsolutePath(t *testing.T) {
	t.Parallel()
	out := seedActTargets(SurfaceScoreInputs{
		RecapTopicSnapshotID: "22222222-2222-4222-8222-222222222222",
	})
	var raw []map[string]string
	require.NoError(t, json.Unmarshal(out, &raw))
	require.Len(t, raw, 1)
	route := raw[0]["route"]
	require.True(t, strings.HasPrefix(route, "/"), "route must be a server-relative path")
	require.False(t, strings.Contains(route, ":"), "route must not contain a scheme separator")
}
