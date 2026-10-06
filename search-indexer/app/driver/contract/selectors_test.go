package contract

import "strings"

type consumerSelector struct {
	Consumer           string
	MainBranch         bool
	DeployedOrReleased bool
}

var searchIndexerConsumers = []string{"rag-orchestrator", "alt-backend", "acolyte-orchestrator"}

func lockstepConsumers(raw string) map[string]bool {
	result := make(map[string]bool)
	for _, item := range strings.Split(raw, ",") {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			result[trimmed] = true
		}
	}
	return result
}

// The deploy pipeline names consumers it rolls in the same release; their
// deployed pact is about to be replaced and the release gate pins their new version.
func consumerSelectors(consumers []string, lockstep map[string]bool) []consumerSelector {
	var selectors []consumerSelector
	for _, c := range consumers {
		selectors = append(selectors, consumerSelector{
			Consumer:   c,
			MainBranch: true,
		})
		if !lockstep[c] {
			selectors = append(selectors, consumerSelector{
				Consumer:           c,
				DeployedOrReleased: true,
			})
		}
	}
	return selectors
}
