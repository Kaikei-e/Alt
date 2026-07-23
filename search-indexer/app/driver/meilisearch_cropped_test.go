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

	// Real Meilisearch shape (production, 2026-07-23): AttributesToRetrieve
	// excludes "content", so the top-level hit never carries it -- only
	// _formatted.content does. _formatted also carries non-string values
	// (tags is an array, id/score originate from numbers), so unmarshaling
	// _formatted straight into map[string]string always errors out and the
	// getCropped fallback finds no top-level "content" either, yielding "".
	// Confirmed against a live Meilisearch query: _formatted keys were
	// [content, id, language, score, tags, title] and _formatted.content was
	// a valid 733-char snippet -- the driver was discarding it wholesale.
	t.Run("real Meilisearch shape with mixed-type _formatted values", func(t *testing.T) {
		hit := makeHit(t, map[string]any{
			"id":            42,
			"title":         "JD Vance meets EU leaders",
			"tags":          []string{"politics", "eu"},
			"user_id":       "user-1",
			"language":      "eng",
			"published_at":  1700000000,
			"_rankingScore": 0.87,
			"_formatted": map[string]any{
				"content":  "JD Vance met with several EU leaders today to discuss trade policy and security cooperation across the continent amid rising tensions.",
				"id":       42,
				"language": "eng",
				"score":    0.87,
				"tags":     []string{"politics", "eu"},
				"title":    "JD Vance meets EU leaders",
			},
		})

		want := "JD Vance met with several EU leaders today to discuss trade policy and security cooperation across the continent amid rising tensions."
		if got := d.getCropped(hit, "content"); got != want {
			t.Errorf("getCropped = %q, want %q", got, want)
		}
	})
}
