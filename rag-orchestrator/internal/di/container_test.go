package di

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"rag-orchestrator/internal/adapter/rag_augur"
	"rag-orchestrator/internal/infra/config"
	"rag-orchestrator/internal/infra/httpclient"
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

	// Factory logic as instantiated in DI
	embedderFactory := func(url string, model string, timeout int) *rag_augur.OllamaEmbedder {
		token := ""
		if sameCanonicalOrigin(url, cfg.Embedder.URL) {
			token = cfg.Embedder.InferenceToken
		}
		return rag_augur.NewOllamaEmbedder(url, model, timeout, logger, token, httpclient.NewPooledClient(time.Duration(timeout)*time.Second))
	}

	// 1. Target with matching origin receives token
	sameOriginEmbedder := embedderFactory("http://proxy.internal:11434/api/embed", "bge-m3", 10)
	if sameOriginEmbedder.InferenceToken != "secret-bearer-token" {
		t.Fatalf("expected matching origin to receive token, got %q", sameOriginEmbedder.InferenceToken)
	}

	// 2. Foreign target receives EMPTY token
	foreignEmbedder := embedderFactory("http://foreign-service.internal:11434/api/embed", "bge-m3", 10)
	if foreignEmbedder.InferenceToken != "" {
		t.Fatalf("expected foreign target to have empty token, got %q", foreignEmbedder.InferenceToken)
	}

	// 3. Different port receives EMPTY token
	diffPortEmbedder := embedderFactory("http://proxy.internal:8080/api/embed", "bge-m3", 10)
	if diffPortEmbedder.InferenceToken != "" {
		t.Fatalf("expected different port to have empty token, got %q", diffPortEmbedder.InferenceToken)
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

func stringsContainsAny(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && (s != "" && func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	}()))
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
