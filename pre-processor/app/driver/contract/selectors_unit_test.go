package contract

import (
	"reflect"
	"testing"
)

func TestLockstepConsumers(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want map[string]bool
	}{
		{name: "unset", raw: "", want: map[string]bool{}},
		{name: "one consumer", raw: "alt-backend", want: map[string]bool{"alt-backend": true}},
		{
			name: "whitespace and a trailing comma",
			raw:  " alt-backend , search-indexer ,",
			want: map[string]bool{"alt-backend": true, "search-indexer": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lockstepConsumers(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lockstepConsumers(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestConsumerSelectors(t *testing.T) {
	mainOnly := []consumerSelector{{Consumer: "alt-backend", MainBranch: true}}
	mainAndDeployed := []consumerSelector{
		{Consumer: "alt-backend", MainBranch: true},
		{Consumer: "alt-backend", DeployedOrReleased: true},
	}

	tests := []struct {
		name     string
		lockstep map[string]bool
		want     []consumerSelector
	}{
		{name: "no lockstep keeps the deployed pact", lockstep: lockstepConsumers(""), want: mainAndDeployed},
		{name: "a lockstep consumer drops its deployed pact", lockstep: lockstepConsumers("alt-backend"), want: mainOnly},
		{name: "another consumer's lockstep changes nothing", lockstep: lockstepConsumers("search-indexer"), want: mainAndDeployed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := consumerSelectors("alt-backend", tt.lockstep); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("consumerSelectors = %v, want %v", got, tt.want)
			}
		})
	}
}
