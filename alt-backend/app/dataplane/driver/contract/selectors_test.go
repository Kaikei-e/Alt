package contract

import "strings"

type consumerSelector struct {
	Consumer           string
	MainBranch         bool
	DeployedOrReleased bool
}

func lockstepConsumers(raw string) map[string]bool {
	result := make(map[string]bool)
	for _, item := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result[trimmed] = true
		}
	}
	return result
}

// The deploy pipeline names consumers it rolls in the same release; their
// deployed pact is about to be replaced and the release gate pins their new version.
func consumerSelectors(consumer string, lockstep map[string]bool) []consumerSelector {
	selectors := []consumerSelector{{Consumer: consumer, MainBranch: true}}
	if !lockstep[consumer] {
		selectors = append(selectors, consumerSelector{Consumer: consumer, DeployedOrReleased: true})
	}
	return selectors
}
