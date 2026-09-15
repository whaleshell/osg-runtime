package proxy_test

import (
	"encoding/base64"
	"errors"
	"net/http"
	"testing"

	"github.com/zorneth/osg-runtime/env"
	"github.com/zorneth/osg-runtime/proxy"
)

func TestRewriteHeaderQueryPathBasic(t *testing.T) {
	secrets := proxy.SecretStore{
		"API_KEY": "secret-value",
		"PASS":    "p@ss",
	}
	req, _ := http.NewRequest(http.MethodGet, "http://api.example/bot"+env.PlaceholderPrefix+"API_KEY/x?token="+env.PlaceholderPrefix+"API_KEY", nil)
	req.Header.Set("Authorization", "Bearer "+env.PlaceholderPrefix+"API_KEY")
	basic := base64.StdEncoding.EncodeToString([]byte("user:"+env.PlaceholderPrefix+"PASS"))
	req.Header.Set("X-Basic", "Basic "+basic)

	if err := proxy.RewriteHTTPRequest(req, secrets); err != nil {
		t.Fatal(err)
	}
	if req.URL.Path != "/botsecret-value/x" && req.URL.Path != "botsecret-value/x" {
		// Path may keep leading slash from URL parser
		if req.URL.Path != "/botsecret-value/x" {
			t.Fatalf("path=%q", req.URL.Path)
		}
	}
	if req.URL.Query().Get("token") != "secret-value" {
		t.Fatalf("query=%q", req.URL.RawQuery)
	}
	if req.Header.Get("Authorization") != "Bearer secret-value" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
	decoded, _ := base64.StdEncoding.DecodeString(stringsTrimBasic(req.Header.Get("X-Basic")))
	if string(decoded) != "user:p@ss" {
		t.Fatalf("basic=%q", decoded)
	}
}

func TestRewriteFailClosed(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://x/", nil)
	req.Header.Set("X-Key", env.PlaceholderPrefix+"MISSING")
	err := proxy.RewriteHTTPRequest(req, proxy.SecretStore{})
	if err == nil {
		t.Fatal("expected unresolved error")
	}
}

func TestRewriteLegacyPlaceholderAlias(t *testing.T) {
	legacy := "openshell:resolve:env:API_KEY"
	secrets := proxy.SecretStore{"API_KEY": "secret-value"}
	req, _ := http.NewRequest(http.MethodGet, "http://api.example/", nil)
	req.Header.Set("Authorization", "Bearer "+legacy)
	if err := proxy.RewriteHTTPRequest(req, secrets); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "Bearer secret-value" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
}

func TestCredentialEndpointMismatch(t *testing.T) {
	secrets := proxy.SecretStore{
		"GITHUB_TOKEN": "gh-secret",
		"OPENAI_API_KEY": "oai-secret",
	}
	req, _ := http.NewRequest(http.MethodGet, "http://api.openai.com/v1", nil)
	req.Header.Set("Authorization", "Bearer "+env.PlaceholderPrefix+"GITHUB_TOKEN")
	used := proxy.PlaceholderKeysInRequest(req)
	if len(used) != 1 || used[0] != "GITHUB_TOKEN" {
		t.Fatalf("used=%v", used)
	}
	// OpenAI endpoint only binds OPENAI_API_KEY
	_, err := proxy.SecretsForEndpoint(secrets, []string{"OPENAI_API_KEY"}, used)
	if err == nil || !errors.Is(err, proxy.ErrCredentialEndpointMismatch) {
		t.Fatalf("err=%v", err)
	}
	// GitHub-bound endpoint allows GITHUB_TOKEN
	rew, err := proxy.SecretsForEndpoint(secrets, []string{"GITHUB_TOKEN", "GH_TOKEN"}, used)
	if err != nil {
		t.Fatal(err)
	}
	if err := proxy.RewriteHTTPRequest(req, rew); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("Authorization") != "Bearer gh-secret" {
		t.Fatalf("auth=%q", req.Header.Get("Authorization"))
	}
	// Empty binding rejects any placeholder
	_, err = proxy.SecretsForEndpoint(secrets, nil, used)
	if err == nil {
		t.Fatal("expected mismatch on empty binding")
	}
}

func TestCredentialKeysSurviveYAML(t *testing.T) {
	// covered in osg-core; smoke here via FilterSecrets empty semantics
	if len(proxy.FilterSecrets(proxy.SecretStore{"A": "1"}, nil)) != 0 {
		t.Fatal("empty bound keys must yield empty store")
	}
}

func stringsTrimBasic(v string) string {
	const p = "Basic "
	if len(v) > len(p) {
		return v[len(p):]
	}
	return v
}
