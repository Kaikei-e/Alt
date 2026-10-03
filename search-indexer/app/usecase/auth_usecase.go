package usecase

import (
	"context"
	"errors"

	"search-indexer/port"
)

type AuthUsecase struct {
	authHub port.AuthHub
}

func NewAuthUsecase(authHub port.AuthHub) *AuthUsecase {
	return &AuthUsecase{authHub: authHub}
}

func (u *AuthUsecase) VerifyUserToken(ctx context.Context, token, expectedUserID string) error {
	info, err := u.authHub.IntrospectToken(ctx, token)
	if err != nil {
		return err
	}
	if !info.Active {
		return errors.New("token is inactive")
	}
	if info.Sub != expectedUserID {
		return errors.New("token subject does not match expected user ID")
	}
	return nil
}
