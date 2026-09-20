package rag_gateway

import (
	"context"
	"net/http"
)

// BearerAuthRequestEditor returns a RequestEditorFn that sets the Authorization
// header to "Bearer <token>". When token is empty, no header is set.
func BearerAuthRequestEditor(token string) RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return nil
	}
}

// WithBearerToken returns a ClientOption that injects "Authorization: Bearer <token>"
// into every outbound HTTP request to rag-orchestrator.
func WithBearerToken(token string) ClientOption {
	return WithRequestEditorFn(BearerAuthRequestEditor(token))
}
