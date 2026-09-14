package proxy

import (
	"testing"

	"github.com/zorneth/osg-core/policy"
)

func TestMCPMatchHTTP(t *testing.T) {
	rule := policy.AllowRule{
		Host: "mcp.example.com", Port: 443, Protocol: "mcp", TLS: "terminate",
		Rules: []policy.L7Rule{
			{Allow: &policy.L7Allow{Method: "tools/list"}},
			{Allow: &policy.L7Allow{Method: "tools/call", Tool: "safe_*"}},
		},
	}
	ok, _ := rule.MatchHTTP("tools/list", "/")
	if !ok {
		t.Fatal("tools/list")
	}
	ok, _ = rule.MatchHTTP("tools/call", "/\x00safe_read")
	if !ok {
		t.Fatal("tools/call safe_read")
	}
	ok, _ = rule.MatchHTTP("tools/call", "/\x00evil")
	if ok {
		t.Fatal("evil tool should deny")
	}
}
