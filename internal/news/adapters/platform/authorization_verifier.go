// Package platform adapts shared Platform primitives to News Application Ports.
package platform

import (
	"context"
	"encoding/json"
	"errors"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformauthorization "example.com/stock-analysis-flow/internal/platform/authorization"
)

var ErrAuthorizationFingerprintInvalid = errors.New("news: authorization fingerprint is invalid")

type AuthorizationFingerprintVerifier struct{}

func NewAuthorizationFingerprintVerifier() *AuthorizationFingerprintVerifier {
	return &AuthorizationFingerprintVerifier{}
}

func (v *AuthorizationFingerprintVerifier) VerifyNewsAuthorizationFingerprint(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
) error {
	if v == nil || ctx == nil {
		return ErrAuthorizationFingerprintInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if domain.ValidateNewsAuthorizationBinding(binding) != nil {
		return ErrAuthorizationFingerprintInvalid
	}
	authorizationRef, err := json.Marshal(binding.AuthorizationRef)
	if err != nil {
		return ErrAuthorizationFingerprintInvalid
	}
	policySnapshot, err := json.Marshal(binding.PolicySnapshot)
	if err != nil {
		return ErrAuthorizationFingerprintInvalid
	}
	if err := platformauthorization.VerifyFingerprint(authorizationRef, policySnapshot); err != nil {
		return errors.Join(ErrAuthorizationFingerprintInvalid, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

var _ app.NewsAuthorizationFingerprintVerifier = (*AuthorizationFingerprintVerifier)(nil)
