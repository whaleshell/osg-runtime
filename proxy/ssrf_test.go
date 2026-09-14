package proxy_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/zorneth/osg-runtime/proxy"
)

func TestSSRFBlocksLoopback(t *testing.T) {
	_, err := proxy.ResolveAndFilter(context.Background(), "127.0.0.1", proxy.SSRFOptions{})
	if err == nil {
		t.Fatal("expected loopback block")
	}
}

func TestSSRFBlocksPrivateWithoutAllow(t *testing.T) {
	_, err := proxy.ResolveAndFilter(context.Background(), "10.1.2.3", proxy.SSRFOptions{})
	if err == nil {
		t.Fatal("expected private block")
	}
}

func TestSSRFAllowsPrivateWithCIDR(t *testing.T) {
	addrs, err := proxy.ResolveAndFilter(context.Background(), "10.1.2.3", proxy.SSRFOptions{
		AllowedIPs: []string{"10.0.0.0/8"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) != 1 || addrs[0] != netip.MustParseAddr("10.1.2.3") {
		t.Fatalf("addrs=%v", addrs)
	}
}

func TestSSRFAllowLoopbackOpt(t *testing.T) {
	addrs, err := proxy.ResolveAndFilter(context.Background(), "127.0.0.1", proxy.SSRFOptions{AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) != 1 {
		t.Fatalf("addrs=%v", addrs)
	}
}

func TestSSRFLinkLocalAlwaysBlocked(t *testing.T) {
	_, err := proxy.ResolveAndFilter(context.Background(), "169.254.169.254", proxy.SSRFOptions{
		AllowedIPs: []string{"169.254.0.0/16"},
	})
	if err == nil {
		t.Fatal("link-local must stay blocked")
	}
}
