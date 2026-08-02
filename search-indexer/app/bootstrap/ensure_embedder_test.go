package bootstrap

import (
	"context"
	"errors"
	"testing"

	"search-indexer/driver"
	"search-indexer/logger"
)

type fakeEmbedderSettings struct {
	live       map[string]driver.EmbedderSpec
	getErr     error
	updateErr  error
	updateCall int
	gotPayload map[string]*driver.EmbedderSpec
}

func (f *fakeEmbedderSettings) GetEmbedders(context.Context) (map[string]driver.EmbedderSpec, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.live, nil
}

func (f *fakeEmbedderSettings) UpdateEmbedders(_ context.Context, payload map[string]*driver.EmbedderSpec) (int64, error) {
	f.updateCall++
	f.gotPayload = payload
	if f.updateErr != nil {
		return 0, f.updateErr
	}
	return 42, nil
}

func testDesiredSpec() driver.EmbedderSpec {
	return driver.EmbedderSpec{
		Source:           "ollama",
		Model:            "bge-m3",
		URL:              "http://knowledge-embedder-local:11434/api/embed",
		Dimensions:       1024,
		DocumentTemplate: embedderDocumentTemplate,
	}
}

// TestEnsureEmbedderSettings_PatchesWhenAbsent covers the first deploy on an
// index that has no embedder at all: the desired definition must be sent.
func TestEnsureEmbedderSettings_PatchesWhenAbsent(t *testing.T) {
	logger.Init()
	f := &fakeEmbedderSettings{live: map[string]driver.EmbedderSpec{}}

	if err := ensureEmbedderSettings(context.Background(), f, "bge-m3", testDesiredSpec()); err != nil {
		t.Fatalf("ensureEmbedderSettings: %v", err)
	}
	if f.updateCall != 1 {
		t.Fatalf("UpdateEmbedders calls = %d, want 1", f.updateCall)
	}
	got, ok := f.gotPayload["bge-m3"]
	if !ok || got == nil {
		t.Fatalf("payload missing desired embedder: %#v", f.gotPayload)
	}
	if *got != testDesiredSpec() {
		t.Fatalf("payload spec = %#v, want %#v", *got, testDesiredSpec())
	}
}

// TestEnsureEmbedderSettings_NoPatchWhenIdentical is the idempotence guard.
// Every settings write makes Meilisearch re-embed the whole index, so a
// restart that changes nothing must issue no PATCH at all.
func TestEnsureEmbedderSettings_NoPatchWhenIdentical(t *testing.T) {
	logger.Init()
	f := &fakeEmbedderSettings{live: map[string]driver.EmbedderSpec{"bge-m3": testDesiredSpec()}}

	if err := ensureEmbedderSettings(context.Background(), f, "bge-m3", testDesiredSpec()); err != nil {
		t.Fatalf("ensureEmbedderSettings: %v", err)
	}
	if f.updateCall != 0 {
		t.Fatalf("UpdateEmbedders calls = %d, want 0 for identical settings", f.updateCall)
	}
}

// TestEnsureEmbedderSettings_PatchesWhenFieldDiffers walks each owned field so
// a drift in any one of them converges instead of being silently tolerated.
func TestEnsureEmbedderSettings_PatchesWhenFieldDiffers(t *testing.T) {
	logger.Init()
	cases := []struct {
		name string
		live driver.EmbedderSpec
	}{
		{"model", driver.EmbedderSpec{Source: "ollama", Model: "qwen3-embedding:0.6b", URL: "http://knowledge-embedder-local:11434/api/embed", Dimensions: 1024, DocumentTemplate: embedderDocumentTemplate}},
		{"url", driver.EmbedderSpec{Source: "ollama", Model: "bge-m3", URL: "http://news-creator-backend:11435/api/embed", Dimensions: 1024, DocumentTemplate: embedderDocumentTemplate}},
		{"dimensions", driver.EmbedderSpec{Source: "ollama", Model: "bge-m3", URL: "http://knowledge-embedder-local:11434/api/embed", Dimensions: 768, DocumentTemplate: embedderDocumentTemplate}},
		{"document template", driver.EmbedderSpec{Source: "ollama", Model: "bge-m3", URL: "http://knowledge-embedder-local:11434/api/embed", Dimensions: 1024, DocumentTemplate: "{{doc.title}}"}},
		{"source", driver.EmbedderSpec{Source: "rest", Model: "bge-m3", URL: "http://knowledge-embedder-local:11434/api/embed", Dimensions: 1024, DocumentTemplate: embedderDocumentTemplate}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeEmbedderSettings{live: map[string]driver.EmbedderSpec{"bge-m3": c.live}}
			if err := ensureEmbedderSettings(context.Background(), f, "bge-m3", testDesiredSpec()); err != nil {
				t.Fatalf("ensureEmbedderSettings: %v", err)
			}
			if f.updateCall != 1 {
				t.Fatalf("UpdateEmbedders calls = %d, want 1 when %s differs", f.updateCall, c.name)
			}
			if got := f.gotPayload["bge-m3"]; got == nil || *got != testDesiredSpec() {
				t.Fatalf("payload spec = %#v, want %#v", got, testDesiredSpec())
			}
		})
	}
}

// TestEnsureEmbedderSettings_RemovesUnknownEmbedders pins the cutover from the
// out-of-band qwen3 embedder: Meilisearch merges the embedders PATCH by name,
// so an embedder left out of the payload survives and keeps embedding every
// document. Removal has to be requested explicitly (nil -> JSON null).
func TestEnsureEmbedderSettings_RemovesUnknownEmbedders(t *testing.T) {
	logger.Init()
	f := &fakeEmbedderSettings{live: map[string]driver.EmbedderSpec{
		"bge-m3": testDesiredSpec(),
		"qwen3":  {Source: "ollama", Model: "qwen3-embedding:0.6b", URL: "http://knowledge-embedder-local:11434/api/embed", Dimensions: 1024},
	}}

	if err := ensureEmbedderSettings(context.Background(), f, "bge-m3", testDesiredSpec()); err != nil {
		t.Fatalf("ensureEmbedderSettings: %v", err)
	}
	if f.updateCall != 1 {
		t.Fatalf("UpdateEmbedders calls = %d, want 1", f.updateCall)
	}
	if len(f.gotPayload) != 1 {
		t.Fatalf("payload = %#v, want only the removal entry", f.gotPayload)
	}
	spec, ok := f.gotPayload["qwen3"]
	if !ok {
		t.Fatalf("payload = %#v, want a qwen3 entry", f.gotPayload)
	}
	if spec != nil {
		t.Fatalf("qwen3 payload = %#v, want nil so it marshals to JSON null", spec)
	}
}

// TestEnsureEmbedderSettings_GetFailureAborts keeps a transient read failure
// from being mistaken for "no embedders configured", which would otherwise
// re-embed the entire index on every flaky startup.
func TestEnsureEmbedderSettings_GetFailureAborts(t *testing.T) {
	logger.Init()
	f := &fakeEmbedderSettings{getErr: errors.New("meilisearch unreachable")}

	err := ensureEmbedderSettings(context.Background(), f, "bge-m3", testDesiredSpec())
	if err == nil {
		t.Fatal("expected an error when the embedder settings cannot be read")
	}
	if f.updateCall != 0 {
		t.Fatalf("UpdateEmbedders calls = %d, want 0 after a read failure", f.updateCall)
	}
}

// TestEnsureEmbedderSettings_UpdateFailurePropagates makes a failed write a
// startup error: hybrid search asks for an embedder by name, so a missing or
// stale definition breaks every query rather than degrading it.
func TestEnsureEmbedderSettings_UpdateFailurePropagates(t *testing.T) {
	logger.Init()
	f := &fakeEmbedderSettings{live: map[string]driver.EmbedderSpec{}, updateErr: errors.New("meilisearch rejected the patch")}

	if err := ensureEmbedderSettings(context.Background(), f, "bge-m3", testDesiredSpec()); err == nil {
		t.Fatal("expected the update failure to propagate")
	}
}

// TestDesiredEmbedder_DefaultsToBgeM3 pins the shipped defaults so the
// declared embedder matches the model knowledge-embedder-local keeps resident.
func TestDesiredEmbedder_DefaultsToBgeM3(t *testing.T) {
	got := desiredEmbedder()
	want := driver.EmbedderSpec{
		Source:           "ollama",
		Model:            "bge-m3",
		URL:              "http://knowledge-embedder-local:11434/api/embed",
		Dimensions:       1024,
		DocumentTemplate: "{{doc.title}}{% if doc.content %}\n{{doc.content | truncate: 2000}}{% endif %}",
	}
	if got != want {
		t.Fatalf("desiredEmbedder() = %#v, want %#v", got, want)
	}
}
