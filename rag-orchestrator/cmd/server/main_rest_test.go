package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"rag-orchestrator/internal/di"
	"rag-orchestrator/internal/domain"
	"rag-orchestrator/internal/infra/config"
	peermw "rag-orchestrator/internal/middleware"
	"rag-orchestrator/internal/usecase"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSimilarityUC struct{}

func (s *stubSimilarityUC) Execute(_ context.Context, _ usecase.CandidateSimilarityInput) (*usecase.CandidateSimilarityOutput, error) {
	return &usecase.CandidateSimilarityOutput{
		SeedIndexed: true,
		Results: []domain.CandidateSimilarityItem{
			{ArticleID: "cand-1", Similarity: 0.88},
		},
	}, nil
}

func TestRESTServer_CandidateSimilarity_PeerIdentityAndAuthWiring(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	peerMW := peermw.NewPeerIdentityMiddleware([]string{"alt-backend", "alt-harvester"}, log)
	apiAuthMW := peermw.NewAPIAuthMiddleware("test-rag-api-token-minimum-24-characters", true, log)

	app := &di.ApplicationComponents{
		CandidateSimilarityUsecase: &stubSimilarityUC{},
	}
	e := newRESTEcho(app, &config.Config{}, peerMW, apiAuthMW, nil, log)

	makeReq := func() *http.Request {
		body := bytes.NewBufferString(`{"seed_article_id":"seed-1","candidate_article_ids":["cand-1"],"representation":"chunk_mean","user_id":"00000000-0000-0000-0000-000000000001"}`)
		req := httptest.NewRequest(http.MethodPost, "/internal/rag/candidate-similarity", body)
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	t.Run("missing client cert returns 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, makeReq())
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("disallowed peer CN returns 403", func(t *testing.T) {
		req := makeReq()
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "evil-peer"}}},
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("allowed peer without bearer token returns 401", func(t *testing.T) {
		req := makeReq()
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "alt-backend"}}},
		}
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("allowed peer with valid bearer token succeeds", func(t *testing.T) {
		req := makeReq()
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{{Subject: pkix.Name{CommonName: "alt-backend"}}},
		}
		req.Header.Set("Authorization", "Bearer test-rag-api-token-minimum-24-characters")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"seed_indexed":true`)
		assert.Contains(t, rec.Body.String(), `"cand-1"`)
	})
}
