// Package idp defines control-plane identity adapters (OIDC / mTLS).
// Stub fails closed until a real backend is wired into osg-gateway.
package idp

import (
	"context"
	"errors"
	"fmt"
)

// Claims is a minimal identity assertion after token validation.
type Claims struct {
	Subject string
	Issuer  string
	Email   string
	Raw     map[string]any
}

// Adapter obtains and validates identity tokens for gateway / agent auth.
type Adapter interface {
	Issuer() string
	Token(ctx context.Context) (string, error)
	Validate(ctx context.Context, token string) (Claims, error)
}

// ErrNotImplemented is returned by Stub for Token/Validate.
var ErrNotImplemented = errors.New("idp: adapter not implemented — see docs/ECOSYSTEM.md")

// Stub is a no-op Adapter placeholder until a real OIDC/mTLS backend lands.
type Stub struct {
	IssuerURL string
}

// Issuer returns IssuerURL or "stub".
func (s Stub) Issuer() string {
	if s.IssuerURL != "" {
		return s.IssuerURL
	}
	return "stub"
}

// Token always fails closed.
func (s Stub) Token(context.Context) (string, error) {
	return "", fmt.Errorf("%w", ErrNotImplemented)
}

// Validate always fails closed.
func (s Stub) Validate(context.Context, string) (Claims, error) {
	return Claims{}, fmt.Errorf("%w", ErrNotImplemented)
}

var _ Adapter = Stub{}
