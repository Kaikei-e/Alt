package driver

import (
	"testing"
)

func TestHybridConfig_Enabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  *HybridConfig
		want bool
	}{
		{"nil", nil, false},
		{"empty embedder", &HybridConfig{Embedder: ""}, false},
		{"embedder set", &HybridConfig{Embedder: "qwen3", SemanticRatio: 0.5}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.cfg.Enabled(); got != c.want {
				t.Errorf("Enabled()=%v want %v", got, c.want)
			}
		})
	}
}

func TestHybridConfig_ToSDK_NilWhenDisabled(t *testing.T) {
	var c *HybridConfig
	if c.toSDK() != nil {
		t.Error("nil receiver should return nil SDK struct")
	}
	if (&HybridConfig{}).toSDK() != nil {
		t.Error("empty config should return nil SDK struct")
	}
}

func TestHybridConfig_ToSDK_ClampsSemanticRatio(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want float64
	}{
		{"negative", -1.0, 0.0},
		{"zero", 0.0, 0.0},
		{"mid", 0.5, 0.5},
		{"one", 1.0, 1.0},
		{"over", 1.5, 1.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := (&HybridConfig{Embedder: "qwen3", SemanticRatio: c.in}).toSDK()
			if got == nil {
				t.Fatal("expected SDK struct, got nil")
			}
			if got.SemanticRatio != c.want {
				t.Errorf("SemanticRatio=%v want %v", got.SemanticRatio, c.want)
			}
			if got.Embedder != "qwen3" {
				t.Errorf("Embedder=%q want qwen3", got.Embedder)
			}
		})
	}
}

func TestMeilisearchDriver_AppliesHybridToSearchRequest(t *testing.T) {
	d := &MeilisearchDriver{hybrid: &HybridConfig{Embedder: "qwen3", SemanticRatio: 0.5}}
	req := d.newBaseSearchRequest("", 10)
	if req.Hybrid == nil {
		t.Fatal("expected Hybrid to be attached when hybrid config is present")
	}
	if req.Hybrid.Embedder != "qwen3" {
		t.Errorf("Embedder=%q want qwen3", req.Hybrid.Embedder)
	}
	if req.Hybrid.SemanticRatio != 0.5 {
		t.Errorf("SemanticRatio=%v want 0.5", req.Hybrid.SemanticRatio)
	}
}

func TestMeilisearchDriver_NoHybridWhenUnconfigured(t *testing.T) {
	d := &MeilisearchDriver{}
	req := d.newBaseSearchRequest("", 10)
	if req.Hybrid != nil {
		t.Errorf("Hybrid should be nil when no config, got %+v", req.Hybrid)
	}
}
