package driver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEmbedderSettingsDriver_GetEmbedders(t *testing.T) {
	var gotPath, gotAuth, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth, gotMethod = r.URL.Path, r.Header.Get("Authorization"), r.Method
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"bge-m3":{"source":"ollama","model":"bge-m3","url":"http://embedder:11434/api/embed","dimensions":1024,"documentTemplate":"{{doc.title}}","documentTemplateMaxBytes":400}}`)
	}))
	defer srv.Close()

	d := NewEmbedderSettingsDriver(srv.URL, "master-key", "articles", time.Second)
	got, err := d.GetEmbedders(context.Background())
	if err != nil {
		t.Fatalf("GetEmbedders: %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Errorf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/indexes/articles/settings/embedders" {
		t.Errorf("path = %s", gotPath)
	}
	if gotAuth != "Bearer master-key" {
		t.Errorf("authorization header = %q", gotAuth)
	}
	want := EmbedderSpec{
		Source:           "ollama",
		Model:            "bge-m3",
		URL:              "http://embedder:11434/api/embed",
		Dimensions:       1024,
		DocumentTemplate: "{{doc.title}}",
	}
	// documentTemplateMaxBytes is deliberately dropped: fields this service
	// does not declare must not participate in the settings diff.
	if got["bge-m3"] != want {
		t.Fatalf("spec = %#v, want %#v", got["bge-m3"], want)
	}
}

// TestEmbedderSettingsDriver_UpdateEmbeddersSendsNullForRemoval is the reason
// this driver bypasses meilisearch-go: its UpdateEmbedders takes a
// map[string]Embedder, which cannot express the JSON null that removes an
// embedder from the index.
func TestEmbedderSettingsDriver_UpdateEmbeddersSendsNullForRemoval(t *testing.T) {
	var body map[string]json.RawMessage
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("request body is not a JSON object: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"taskUid":77,"status":"enqueued"}`)
	}))
	defer srv.Close()

	d := NewEmbedderSettingsDriver(srv.URL, "master-key", "articles", time.Second)
	spec := EmbedderSpec{Source: "ollama", Model: "bge-m3", URL: "http://embedder:11434/api/embed", Dimensions: 1024, DocumentTemplate: "{{doc.title}}"}
	taskUID, err := d.UpdateEmbedders(context.Background(), map[string]*EmbedderSpec{
		"bge-m3": &spec,
		"qwen3":  nil,
	})
	if err != nil {
		t.Fatalf("UpdateEmbedders: %v", err)
	}

	if gotMethod != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", gotMethod)
	}
	if taskUID != 77 {
		t.Errorf("taskUID = %d, want 77", taskUID)
	}
	if string(body["qwen3"]) != "null" {
		t.Errorf("qwen3 entry = %s, want null", body["qwen3"])
	}
	var sent EmbedderSpec
	if err := json.Unmarshal(body["bge-m3"], &sent); err != nil {
		t.Fatalf("bge-m3 entry: %v", err)
	}
	if sent != spec {
		t.Errorf("bge-m3 entry = %#v, want %#v", sent, spec)
	}
}

func TestEmbedderSettingsDriver_ErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":"missing_authorization_header"}`)
	}))
	defer srv.Close()

	d := NewEmbedderSettingsDriver(srv.URL, "", "articles", time.Second)
	if _, err := d.GetEmbedders(context.Background()); err == nil {
		t.Error("GetEmbedders: expected an error on 401")
	}
	if _, err := d.UpdateEmbedders(context.Background(), map[string]*EmbedderSpec{"qwen3": nil}); err == nil {
		t.Error("UpdateEmbedders: expected an error on 401")
	}
}
