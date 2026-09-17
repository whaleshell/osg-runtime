package idp

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

func verifyRS256(signingInput, sigB64 string, pub *rsa.PublicKey) ([]byte, error) {
	sig, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, fmt.Errorf("oidc: sig decode: %w", err)
	}
	sum := sha256.Sum256([]byte(signingInput))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return nil, fmt.Errorf("oidc: signature: %w", err)
	}
	payload, err := base64.RawURLEncoding.DecodeString(stringsSplitPayload(signingInput))
	if err != nil {
		return nil, fmt.Errorf("oidc: payload decode: %w", err)
	}
	return payload, nil
}

func stringsSplitPayload(signingInput string) string {
	for i := 0; i < len(signingInput); i++ {
		if signingInput[i] == '.' {
			return signingInput[i+1:]
		}
	}
	return ""
}
