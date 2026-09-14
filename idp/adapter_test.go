package idp

import (
	"context"
	"errors"
	"testing"
)

func TestStub(t *testing.T) {
	s := Stub{IssuerURL: "https://idp.example"}
	if s.Issuer() != "https://idp.example" {
		t.Fatalf("issuer=%q", s.Issuer())
	}
	_, err := s.Token(context.Background())
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("token err=%v", err)
	}
	_, err = s.Validate(context.Background(), "x")
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("validate err=%v", err)
	}
}
