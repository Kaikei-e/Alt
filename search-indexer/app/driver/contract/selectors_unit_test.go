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
		{
			name: "unset/empty",
			raw:  "",
			want: map[string]bool{},
		},
		{
			name: "rag-orchestrator",
			raw:  "rag-orchestrator",
			want: map[string]bool{
				"rag-orchestrator": true,
			},
		},
		{
			name: "rag-orchestrator and acolyte-orchestrator with whitespace and comma",
			raw:  " rag-orchestrator , acolyte-orchestrator ,",
			want: map[string]bool{
				"rag-orchestrator":     true,
				"acolyte-orchestrator": true,
			},
		},
		{
			name: "news-creator",
			raw:  "news-creator",
			want: map[string]bool{
				"news-creator": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lockstepConsumers(tt.raw)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lockstepConsumers(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestConsumerSelectors(t *testing.T) {
	allSix := []consumerSelector{
		{Consumer: "rag-orchestrator", MainBranch: true},
		{Consumer: "rag-orchestrator", DeployedOrReleased: true},
		{Consumer: "alt-backend", MainBranch: true},
		{Consumer: "alt-backend", DeployedOrReleased: true},
		{Consumer: "acolyte-orchestrator", MainBranch: true},
		{Consumer: "acolyte-orchestrator", DeployedOrReleased: true},
	}

	tests := []struct {
		name     string
		lockstep map[string]bool
		want     []consumerSelector
	}{
		{
			name:     "unset/empty",
			lockstep: lockstepConsumers(""),
			want:     allSix,
		},
		{
			name:     "rag-orchestrator",
			lockstep: lockstepConsumers("rag-orchestrator"),
			want: []consumerSelector{
				{Consumer: "rag-orchestrator", MainBranch: true},
				{Consumer: "alt-backend", MainBranch: true},
				{Consumer: "alt-backend", DeployedOrReleased: true},
				{Consumer: "acolyte-orchestrator", MainBranch: true},
				{Consumer: "acolyte-orchestrator", DeployedOrReleased: true},
			},
		},
		{
			name:     "rag-orchestrator and acolyte-orchestrator with whitespace and comma",
			lockstep: lockstepConsumers(" rag-orchestrator , acolyte-orchestrator ,"),
			want: []consumerSelector{
				{Consumer: "rag-orchestrator", MainBranch: true},
				{Consumer: "alt-backend", MainBranch: true},
				{Consumer: "alt-backend", DeployedOrReleased: true},
				{Consumer: "acolyte-orchestrator", MainBranch: true},
			},
		},
		{
			name:     "unknown name news-creator",
			lockstep: lockstepConsumers("news-creator"),
			want:     allSix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consumerSelectors(searchIndexerConsumers, tt.lockstep)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("consumerSelectors() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
