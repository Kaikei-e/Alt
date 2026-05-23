package knowledge_loop_session_state

import "testing"

func TestLookupLensModeWeights_DefaultLensReturnsCanonicalThresholds(t *testing.T) {
	t.Parallel()
	got := LookupLensModeWeights(DefaultLensModeID)
	if got.MediumThreshold != 3 {
		t.Errorf("MediumThreshold = %d; want 3", got.MediumThreshold)
	}
	if got.HeavyThreshold != 7 {
		t.Errorf("HeavyThreshold = %d; want 7", got.HeavyThreshold)
	}
}

func TestLookupLensModeWeights_UnknownLensFallsBackToDefault(t *testing.T) {
	t.Parallel()
	def := LookupLensModeWeights(DefaultLensModeID)
	got := LookupLensModeWeights(LensModeID("does-not-exist"))
	if got != def {
		t.Errorf("unknown lens = %+v; want default %+v", got, def)
	}
}

func TestLensWeightsVersion_IsPinnedToOne(t *testing.T) {
	t.Parallel()
	// LensWeightsVersion is a load-bearing reproject signal — operators
	// schedule a full reproject when this bumps. The test pins the value
	// so a casual edit cannot silently change cohorts without going
	// through the runbook.
	if LensWeightsVersion != 1 {
		t.Errorf("LensWeightsVersion = %d; want 1 (bump only via reproject runbook)", LensWeightsVersion)
	}
}
