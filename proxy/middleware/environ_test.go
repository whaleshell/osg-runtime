package middleware_test

import (
	"testing"

	"github.com/zorneth/osg-runtime/proxy/middleware"
)

func TestFromEnviron(t *testing.T) {
	if p := middleware.FromEnviron([]string{}); p != nil {
		t.Fatal("expected nil")
	}
	p := middleware.FromEnviron([]string{
		"OSG_MIDDLEWARE_JWT_AUD=osg",
		"OSG_MIDDLEWARE_REMOTE_URL=http://127.0.0.1:9/mw",
	})
	if p == nil || len(p.Stages) != 2 {
		t.Fatalf("stages=%v", p)
	}
}
