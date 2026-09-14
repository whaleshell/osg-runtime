package middleware_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zorneth/osg-runtime/proxy/middleware"
)

func TestJWTStubAudience(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"aud":"osg"}`))
	token := "x." + payload + ".y"
	st := &middleware.JWTStub{Audience: "osg", Required: true}
	dec, err := st.Evaluate(context.Background(), middleware.Request{
		Headers: map[string]string{"Authorization": "Bearer " + token},
	})
	if err != nil || !dec.Allow {
		t.Fatalf("dec=%v err=%v", dec, err)
	}
	dec, _ = st.Evaluate(context.Background(), middleware.Request{})
	if dec.Allow {
		t.Fatal("expected deny without token")
	}
}

func TestRemoteStage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(middleware.Decision{Allow: false, Reason: "nope"})
	}))
	defer srv.Close()
	p := &middleware.Pipeline{Stages: []middleware.Stage{
		&middleware.RemoteStage{URL: srv.URL, FailClosed: true},
	}}
	dec, err := p.Run(context.Background(), middleware.Request{Host: "x", Method: "GET", Path: "/"})
	if err != nil || dec.Allow {
		t.Fatalf("dec=%v err=%v", dec, err)
	}
}
