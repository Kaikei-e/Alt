package driver

import (
	"encoding/json"
	"testing"

	"github.com/meilisearch/meilisearch-go"
)

func makeHit(t *testing.T, m map[string]any) meilisearch.Hit {
	t.Helper()
	hit := meilisearch.Hit{}
	for k, v := range m {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %s: %v", k, err)
		}
		hit[k] = b
	}
	return hit
}

func TestGetCropped_PrefersFormattedThenFallbackToRaw(t *testing.T) {
	d := &MeilisearchDriver{}

	t.Run("formatted present", func(t *testing.T) {
		hit := makeHit(t, map[string]any{
			"content":    "this is the full original content payload",
			"_formatted": map[string]string{"content": "this is the crop"},
		})
		if got := d.getCropped(hit, "content"); got != "this is the crop" {
			t.Errorf("getCropped = %q, want %q", got, "this is the crop")
		}
	})

	t.Run("formatted absent falls back to raw", func(t *testing.T) {
		hit := makeHit(t, map[string]any{
			"content": "raw value",
		})
		if got := d.getCropped(hit, "content"); got != "raw value" {
			t.Errorf("getCropped = %q, want %q", got, "raw value")
		}
	})

	t.Run("both absent returns empty", func(t *testing.T) {
		hit := makeHit(t, map[string]any{})
		if got := d.getCropped(hit, "content"); got != "" {
			t.Errorf("getCropped = %q, want empty", got)
		}
	})
}
