package usecase

import (
	"context"

	"auth-hub/internal/domain"
)

type IntrospectToken struct {
	Verifier domain.TokenVerifier
}

func NewIntrospectToken(v domain.TokenVerifier) *IntrospectToken {
	return &IntrospectToken{Verifier: v}
}

func (uc *IntrospectToken) Execute(ctx context.Context, token string) (*domain.IntrospectedToken, error) {
	if token == "" {
		return &domain.IntrospectedToken{Active: false}, nil
	}
	return uc.Verifier.IntrospectBackendToken(token)
}
