package proxy_test

import (
	"encoding/base64"
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

func stringsTrimBasic(v string) string {
	const p = "Basic "
	if len(v) > len(p) {
		return v[len(p):]
	}
	return v
}
