package idp_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/whaleshell/whaleshell-runtime/idp"
)

func TestOIDCValidateRS256(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	n := base64.RawURLEncoding.EncodeToString(key.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01})

	var issuer string
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 issuer,
			"jwks_uri":               issuer + "/jwks",
			"authorization_endpoint": issuer + "/auth",
			"token_endpoint":         issuer + "/token",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig", "n": n, "e": e,
			}},
		})
	})
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	issuer = ts.URL

	tok := mustSignJWT(t, key, map[string]any{
		"iss": issuer,
		"sub": "user-1",
		"aud": "whaleshell",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	})

	v, err := idp.NewOIDC(idp.OIDCConfig{
		Issuer:            issuer,
		Audience:          "whaleshell",
		AllowInsecureHTTP: true,
		HTTPClient:        ts.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := v.Validate(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != "user-1" {
		t.Fatalf("sub=%q", claims.Subject)
	}
}

func mustSignJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT","kid":"k1"}`))
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	pl := base64.RawURLEncoding.EncodeToString(payload)
	input := hdr + "." + pl
	sum := sha256.Sum256([]byte(input))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return input + "." + base64.RawURLEncoding.EncodeToString(sig)
}
