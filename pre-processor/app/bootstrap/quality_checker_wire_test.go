package bootstrap

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"pre-processor/config"
	"pre-processor/driver"
	qualitychecker "pre-processor/quality-checker"
)

func TestProductionTLSFactoryWire_QualityChecker(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath, caPath, serverCert, caPool := writeThrowawayPKIWithServer(t, dir, "news-creator")

	var serverHit atomic.Bool
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/generate" && r.Method == http.MethodPost {
			serverHit.Store(true)
			if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
				http.Error(w, "missing peer client certificate", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"done":true,"response":"<score>8</score>"}`))
			return
		}
		http.NotFound(w, r)
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
	}
	server.StartTLS()
	defer server.Close()

	t.Setenv("MTLS_ENFORCE", "true")
	t.Setenv("MTLS_CERT_FILE", certPath)
	t.Setenv("MTLS_KEY_FILE", keyPath)
	t.Setenv("MTLS_CA_FILE", caPath)
	t.Setenv("NEWS_CREATOR_MTLS_SERVER_NAME", "news-creator")

	cfg := &config.Config{
		NewsCreator: config.NewsCreatorConfig{
			Host:    server.URL,
			Timeout: 5 * time.Second,
		},
		QualityChecker: config.QualityCheckerConfig{
			APIPath: "/api/generate",
			Timeout: 5 * time.Second,
		},
	}

	newsHTTPClient, err := buildNewsHTTPClient(cfg, slog.Default())
	require.NoError(t, err)
	qualitychecker.Configure(cfg, newsHTTPClient)
	t.Cleanup(func() {
		qualitychecker.Configure(&config.Config{
			NewsCreator: config.NewsCreatorConfig{
				Host: "http://news-creator:11434",
			},
		}, &http.Client{})
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-123",
		Content:         "Article content for quality check.",
		SummaryJapanese: "テスト要約です。",
	}

	err = qualitychecker.JudgeArticleQuality(ctx, nil, nil, article)
	require.NoError(t, err)
	assert.True(t, serverHit.Load())
}
