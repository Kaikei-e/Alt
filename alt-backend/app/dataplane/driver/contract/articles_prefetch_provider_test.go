//go:build contract

package contract

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"alt/config"
	"alt/domain"
	"alt/gen/proto/alt/articles/v2/articlesv2connect"
	"alt/orchestrator/connect/v2/articles"
)

// This file mounts the *real* ArticleService Connect handler for the one
// procedure alt-butterfly-facade's pact names on it —
// BatchPrefetchArticleContent — rather than a hand-written JSON stub like the
// FeedService routes in provider_test.go.
//
// The difference is the whole point of CLAUDE.md rule 7. A stub route proves
// that somebody wrote a stub route; it stays green whether or not the shipped
// handler implements the procedure at all, which is exactly the
// silent-fallback shape ADR-000928 is about. Mounting the real handler makes
// this verification fail with connect's "unimplemented" until the procedure
// exists, and fail again the day its wire shape drifts from what the BFF
// records.
//
// The other alt-butterfly-facade interactions keep their stubs: retrofitting
// them is a separate change with its own risk, and doing it here would bury
// this one under a large diff.

// pactPrefetchUser is the caller the real handler sees. Every ArticleService
// procedure starts with middleware.GetUserContext, so a verification that did
// not inject one would only ever prove the handler can return 401.
func pactPrefetchUser() *domain.UserContext {
	return &domain.UserContext{
		UserID:    uuid.MustParse("3f2b1c8e-5a4d-4e6f-9b0a-1c2d3e4f5a6b"),
		Email:     "pact-verifier@example.com",
		Role:      domain.UserRoleUser,
		TenantID:  uuid.MustParse("7a8b9c0d-1e2f-4a3b-8c5d-6e7f8a9b0c1d"),
		SessionID: "pact-session",
		LoginAt:   time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
	}
}

// withPactUserContext stands in for the auth interceptor. It authenticates on
// the presence of the header the BFF forwards rather than on a signature: the
// pact pins that the BFF sends X-Alt-Backend-Token, and JWT verification has
// its own unit tests. Rejecting the header's absence keeps the substitution
// from quietly turning an authenticated surface into an open one.
func withPactUserContext(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Alt-Backend-Token") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		ctx := domain.SetUserContext(r.Context(), pactPrefetchUser())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// mountArticleServiceProcedures registers the real ArticleService handler for
// the procedures the alt-butterfly-facade pact exercises.
func mountArticleServiceProcedures(mux *http.ServeMux) {
	handler := articles.NewHandler(
		newPactArticleDeps(),
		&config.Config{},
		slog.Default(),
	)

	_, connectHandler := articlesv2connect.NewArticleServiceHandler(handler)
	mux.Handle(
		"/alt.articles.v2.ArticleService/BatchPrefetchArticleContent",
		withPactUserContext(connectHandler),
	)
}
