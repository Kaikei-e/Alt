package di

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"

	"rag-orchestrator/internal/adapter/rag_augur"
	"rag-orchestrator/internal/infra/config"
)

func TestSameCanonicalOrigin(t *testing.T) {
	tests := []struct {
		name       string
		target     string
		configured string
		expected   bool
	}{
		{
			name:       "exact match",
			target:     "http://localhost:11434",
			configured: "http://localhost:11434",
			expected:   true,
		},
		{
			name:       "trailing slash difference",
			target:     "http://localhost:11434/",
			configured: "http://localhost:11434",
			expected:   true,
		},
		{
			name:       "path difference ignored (origin comparison)",
			target:     "http://localhost:11434/api/embed",
			configured: "http://localhost:11434/other",
			expected:   true,
		},
		{
			name:       "default http port 80 match",
			target:     "http://example.internal:80",
			configured: "http://example.internal",
			expected:   true,
		},
		{
			name:       "default https port 443 match",
			target:     "https://example.internal:443",
			configured: "https://example.internal",
			expected:   true,
		},
		{
			name:       "different host",
			target:     "http://malicious.internal:11434",
			configured: "http://localhost:11434",
			expected:   false,
		},
		{
			name:       "different scheme",
			target:     "https://localhost:11434",
			configured: "http://localhost:11434",
			expected:   false,
		},
		{
			name:       "different port",
			target:     "http://localhost:11435",
			configured: "http://localhost:11434",
			expected:   false,
		},
		{
			name:       "invalid url target",
			target:     "://invalid",
			configured: "http://localhost:11434",
			expected:   false,
		},
		{
			name:       "invalid url configured",
			target:     "http://localhost:11434",
			configured: "://invalid",
			expected:   false,
		},
		{
			name:       "empty strings",
			target:     "",
			configured: "",
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sameCanonicalOrigin(tt.target, tt.configured)
			if got != tt.expected {
				t.Errorf("sameCanonicalOrigin(%q, %q) = %v; want %v", tt.target, tt.configured, got, tt.expected)
			}
		})
	}
}

func TestEmbedderFactory_InferenceCredentialScoping(t *testing.T) {
	cfg := &config.Config{
		Embedder: config.EmbedderConfig{
			URL:            "http://proxy.internal:11434",
			Model:          "bge-m3",
			Timeout:        10,
			InferenceToken: "secret-bearer-token",
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	embedderFactory := newEmbedderFactory(cfg.Embedder, logger)
	build := func(url string) *rag_augur.OllamaEmbedder {
		t.Helper()
		embedder, ok := embedderFactory(url, "bge-m3", 10).(*rag_augur.OllamaEmbedder)
		if !ok {
			t.Fatalf("factory must build an *rag_augur.OllamaEmbedder")
		}
		return embedder
	}

	// 1. Target with matching origin receives token
	sameOriginEmbedder := build("http://proxy.internal:11434/api/embed")
	if sameOriginEmbedder.InferenceToken != "secret-bearer-token" {
		t.Fatalf("expected matching origin to receive token, got %q", sameOriginEmbedder.InferenceToken)
	}

	// 2. Foreign target receives EMPTY token
	foreignEmbedder := build("http://foreign-service.internal:11434/api/embed")
	if foreignEmbedder.InferenceToken != "" {
		t.Fatalf("expected foreign target to have empty token, got %q", foreignEmbedder.InferenceToken)
	}

	// 3. Different port receives EMPTY token
	diffPortEmbedder := build("http://proxy.internal:8080/api/embed")
	if diffPortEmbedder.InferenceToken != "" {
		t.Fatalf("expected different port to have empty token, got %q", diffPortEmbedder.InferenceToken)
	}
}

// search-indexer serves /v1/search only behind RequireAndVerifyClientCert on
// :9443, so the client has to carry the leaf whatever MTLS_ENFORCE says. The
// pooled client it used to share only does that when MTLS_ENFORCE=true.
func TestNewSearchIndexerClient_RequiresClientCertMaterial(t *testing.T) {
	t.Setenv("MTLS_ENFORCE", "")

	client, err := newSearchIndexerClient(config.SearchConfig{
		IndexerURL: "https://search-indexer:9443",
		Timeout:    5,
	})

	if err == nil {
		t.Fatalf("missing client cert material must be a startup error, got client %+v", client)
	}
}

func TestLogInferenceAuth(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config.InferenceAuthConfig
		wantMsg string
		notMsg  string
	}{
		{
			name:    "disabled",
			cfg:     config.InferenceAuthConfig{Enabled: false},
			wantMsg: "inference_auth_disabled",
			notMsg:  "inference_auth_enabled",
		},
		{
			name:    "enabled",
			cfg:     config.InferenceAuthConfig{Enabled: true, Token: "secret-bearer-token-value"},
			wantMsg: "inference_auth_enabled",
			notMsg:  "inference_auth_disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logInferenceAuth(tt.cfg, slog.New(slog.NewJSONHandler(&buf, nil)))

			out := buf.String()
			if got := strings.Count(out, `"msg":"`+tt.wantMsg+`"`); got != 1 {
				t.Fatalf("want exactly one %s record, got %d in %q", tt.wantMsg, got, out)
			}
			if strings.Contains(out, tt.notMsg) {
				t.Fatalf("unexpected %s record in %q", tt.notMsg, out)
			}
			if strings.Contains(out, "secret-bearer-token-value") {
				t.Fatalf("the token must never reach the log: %q", out)
			}
		})
	}
}

func TestAugurGenerator_MTLSSuppressesRawToken(t *testing.T) {
	tests := []struct {
		url       string
		purpose   string
		token     string
		wantToken string
	}{
		{
			url:       "https://news-creator:9443",
			purpose:   "",
			token:     "raw-secret",
			wantToken: "", // canonical default News mTLS suppresses token
		},
		{
			url:       "https://news-service.internal/generate",
			purpose:   "news_mtls",
			token:     "raw-secret",
			wantToken: "", // explicitly configured purpose suppresses token
		},
		{
			url:       "https://news-proxy:9443",
			purpose:   "",
			token:     "raw-secret",
			wantToken: "raw-secret", // news proxy 9443 retains token if purpose is empty (defaults to authenticated_proxy)
		},
		{
			url:       "http://ollama-proxy:11434",
			purpose:   "",
			token:     "raw-secret",
			wantToken: "raw-secret", // standard proxy retains token
		},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			cfg := &config.AugurConfig{
				URL:             tt.url,
				EndpointPurpose: tt.purpose,
				InferenceToken:  tt.token,
				Model:           "test-model",
				Timeout:         10,
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
			generator := BuildOllamaGenerator(cfg, nil, logger)

			if generator.InferenceToken != tt.wantToken {
				t.Errorf("url %q: token = %q, want %q", tt.url, generator.InferenceToken, tt.wantToken)
			}
		})
	}
}

func TestBuildOllamaGenerator_ProductionFactoryTests(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	t.Run("missingtoken proxy failclosed", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for missing token proxy")
			}
		}()
		cfg := &config.AugurConfig{
			URL:             "http://proxy:11434",
			EndpointPurpose: "authenticated_proxy",
			InferenceToken:  "",
		}
		BuildOllamaGenerator(cfg, nil, logger)
	})

	t.Run("unknownpurpose failclosed", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Errorf("expected panic for unknown purpose")
			}
		}()
		cfg := &config.AugurConfig{
			URL:             "http://proxy:11434",
			EndpointPurpose: "invalid_purpose",
			InferenceToken:  "secret",
		}
		BuildOllamaGenerator(cfg, nil, logger)
	})

	t.Run("case/trailingSlash normalizedNewsnoBearer", func(t *testing.T) {
		cfg := &config.AugurConfig{
			URL:             "HTTPS://News-Creator:9443/",
			EndpointPurpose: "",
			InferenceToken:  "secret",
		}
		gen := BuildOllamaGenerator(cfg, nil, logger)
		if gen.InferenceToken != "" {
			t.Errorf("expected empty token for normalized news url, got %q", gen.InferenceToken)
		}
	})

	t.Run("genuine exportedInferenceToken inspectedokay", func(t *testing.T) {
		cfg := &config.AugurConfig{
			URL:             "https://news-proxy:9443",
			EndpointPurpose: "authenticated_proxy",
			InferenceToken:  "raw-secret",
		}
		gen := BuildOllamaGenerator(cfg, nil, logger)
		if gen.InferenceToken != "raw-secret" {
			t.Errorf("expected token for news-proxy, got %q", gen.InferenceToken)
		}
	})
}
