package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// EmbedderSpec is the part of a Meilisearch embedder definition that
// search-indexer owns as code. Fields Meilisearch derives on its own
// (documentTemplateMaxBytes, pooling, distribution, binaryQuantized, ...) are
// deliberately absent: they are dropped when reading, so reconciliation never
// fights the engine over values this service never declared.
type EmbedderSpec struct {
	Source           string `json:"source"`
	Model            string `json:"model"`
	URL              string `json:"url"`
	Dimensions       int    `json:"dimensions"`
	DocumentTemplate string `json:"documentTemplate"`
}

// EmbedderSettingsDriver reads and writes /indexes/{uid}/settings/embedders.
//
// It speaks HTTP directly instead of going through meilisearch-go because
// removing an embedder requires sending JSON null under its name, and the
// SDK's UpdateEmbedders signature (map[string]Embedder) cannot express null.
// Meilisearch merges this route by embedder name, so without a null an
// obsolete embedder survives forever and keeps embedding every document.
type EmbedderSettingsDriver struct {
	baseURL   string
	apiKey    string
	indexName string
	client    *http.Client
}

func NewEmbedderSettingsDriver(baseURL, apiKey, indexName string, timeout time.Duration) *EmbedderSettingsDriver {
	return &EmbedderSettingsDriver{
		baseURL:   strings.TrimSuffix(baseURL, "/"),
		apiKey:    apiKey,
		indexName: indexName,
		client:    &http.Client{Timeout: timeout},
	}
}

func (d *EmbedderSettingsDriver) endpoint() string {
	return d.baseURL + "/indexes/" + d.indexName + "/settings/embedders"
}

func (d *EmbedderSettingsDriver) newRequest(ctx context.Context, method string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, d.endpoint(), body)
	if err != nil {
		return nil, err
	}
	if d.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+d.apiKey)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// GetEmbedders returns the embedders currently defined on the index, keyed by
// embedder name.
func (d *EmbedderSettingsDriver) GetEmbedders(ctx context.Context) (map[string]EmbedderSpec, error) {
	req, err := d.newRequest(ctx, http.MethodGet, nil)
	if err != nil {
		return nil, &DriverError{Op: "GetEmbedders", Err: err}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, &DriverError{Op: "GetEmbedders", Err: err}
	}
	defer resp.Body.Close()

	raw, err := readBounded(resp.Body)
	if err != nil {
		return nil, &DriverError{Op: "GetEmbedders", Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &DriverError{Op: "GetEmbedders", Err: statusError(resp.StatusCode, raw)}
	}

	embedders := make(map[string]EmbedderSpec)
	if err := json.Unmarshal(raw, &embedders); err != nil {
		return nil, &DriverError{Op: "GetEmbedders", Err: fmt.Errorf("decode embedders: %w", err)}
	}
	return embedders, nil
}

// UpdateEmbedders PATCHes the index's embedder settings and returns the UID of
// the task Meilisearch enqueued. A nil value removes that embedder.
//
// The task is not awaited: any change here makes Meilisearch re-embed every
// document in the index, which takes minutes, and startup must not block on it.
func (d *EmbedderSettingsDriver) UpdateEmbedders(ctx context.Context, payload map[string]*EmbedderSpec) (int64, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return 0, &DriverError{Op: "UpdateEmbedders", Err: err}
	}
	req, err := d.newRequest(ctx, http.MethodPatch, bytes.NewReader(encoded))
	if err != nil {
		return 0, &DriverError{Op: "UpdateEmbedders", Err: err}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, &DriverError{Op: "UpdateEmbedders", Err: err}
	}
	defer resp.Body.Close()

	raw, err := readBounded(resp.Body)
	if err != nil {
		return 0, &DriverError{Op: "UpdateEmbedders", Err: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, &DriverError{Op: "UpdateEmbedders", Err: statusError(resp.StatusCode, raw)}
	}

	var task struct {
		TaskUID int64 `json:"taskUid"`
	}
	if err := json.Unmarshal(raw, &task); err != nil {
		return 0, &DriverError{Op: "UpdateEmbedders", Err: fmt.Errorf("decode task: %w", err)}
	}
	return task.TaskUID, nil
}

// embedderResponseLimit bounds how much of a Meilisearch response is read, so
// a misrouted request cannot stream an unbounded body into memory.
const embedderResponseLimit = 1 << 20

func readBounded(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, embedderResponseLimit))
}

// statusError includes a truncated body so the Meilisearch error code reaches
// the logs without risking a multi-KB line.
func statusError(status int, body []byte) error {
	const maxDetail = 512
	if len(body) > maxDetail {
		body = body[:maxDetail]
	}
	return fmt.Errorf("meilisearch returned %d: %s", status, strings.TrimSpace(string(body)))
}
