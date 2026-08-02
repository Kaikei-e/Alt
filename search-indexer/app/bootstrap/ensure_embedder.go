package bootstrap

import (
	"context"
	"fmt"
	"sort"

	"search-indexer/config"
	"search-indexer/driver"
	"search-indexer/logger"
)

// embedderSource is the Meilisearch embedder backend. Ollama is not
// configurable: the URL and model defaults below only make sense for it.
const embedderSource = "ollama"

// embedderDocumentTemplate is the Liquid template Meilisearch renders per
// document before handing the text to the embedder. It stays in code rather
// than in env because editing it re-embeds the whole index.
const embedderDocumentTemplate = "{{doc.title}}{% if doc.content %}\n{{doc.content | truncate: 2000}}{% endif %}"

// embedderSettingsManager narrows the driver surface the reconciliation needs,
// mirroring warmupSearcher/taskPruner: the unit test stays independent of the
// HTTP driver.
type embedderSettingsManager interface {
	GetEmbedders(ctx context.Context) (map[string]driver.EmbedderSpec, error)
	UpdateEmbedders(ctx context.Context, payload map[string]*driver.EmbedderSpec) (int64, error)
}

// desiredEmbedder is the embedder definition this service declares on the
// articles index.
func desiredEmbedder() driver.EmbedderSpec {
	return driver.EmbedderSpec{
		Source:           embedderSource,
		Model:            config.MeiliEmbedderModel,
		URL:              config.MeiliEmbedderURL,
		Dimensions:       config.MeiliEmbedderDimensions,
		DocumentTemplate: embedderDocumentTemplate,
	}
}

// ensureEmbedderSettings converges the index's embedder settings on the single
// definition named by MEILI_HYBRID_EMBEDDER, so the embedder every search
// request asks for is declared here rather than by an operator's out-of-band
// PATCH (the debt recorded in ADR-000951).
//
// It patches only on a real difference. Meilisearch re-embeds every document
// whenever the embedder settings change, so an unconditional write on each
// restart would put the index through a full re-embed for nothing.
//
// Embedders under any other name are removed in the same PATCH. This route
// merges by name: an embedder omitted from the payload survives and keeps
// consuming embedder calls and vector storage per document, so removal has to
// be requested explicitly with a JSON null.
func ensureEmbedderSettings(ctx context.Context, m embedderSettingsManager, name string, desired driver.EmbedderSpec) error {
	live, err := m.GetEmbedders(ctx)
	if err != nil {
		return fmt.Errorf("read embedder settings: %w", err)
	}

	payload := make(map[string]*driver.EmbedderSpec, len(live)+1)
	if current, ok := live[name]; !ok || current != desired {
		spec := desired
		payload[name] = &spec
	}
	var removed []string
	for liveName := range live {
		if liveName != name {
			payload[liveName] = nil
			removed = append(removed, liveName)
		}
	}
	sort.Strings(removed)

	if len(payload) == 0 {
		logger.Logger.InfoContext(ctx, "embedder settings already match",
			"embedder", name,
			"model", desired.Model,
		)
		return nil
	}

	taskUID, err := m.UpdateEmbedders(ctx, payload)
	if err != nil {
		return fmt.Errorf("update embedder settings: %w", err)
	}
	logger.Logger.InfoContext(ctx, "embedder settings patched, meilisearch will re-embed every document",
		"embedder", name,
		"model", desired.Model,
		"url", desired.URL,
		"dimensions", desired.Dimensions,
		"removed_embedders", removed,
		"task_uid", taskUID,
	)
	return nil
}
