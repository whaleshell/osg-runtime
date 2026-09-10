package proxy_test

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/lkmavi/osg-core/engine"
	"github.com/lkmavi/osg-core/policy"
	"github.com/lkmavi/osg-runtime/proxy"
)

func TestCONNECTAllowDeny(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	go func() {
		for {
			c, err := backend.Accept()
			if err != nil {
				return
			}
			_, _ = c.Write([]byte("hello"))
			_ = c.Close()
		}
	}()
	_, backendPort, _ := net.SplitHostPort(backend.Addr().String())

	doc := policy.Document{
		Version: 1,
		Network: &policy.Network{
			Default: "deny",
			Allow: []policy.AllowRule{
				{ID: "local", Host: "127.0.0.1", Port: mustAtoi(backendPort)},
			},
		},
	}
	var eng engine.Allowlist
	if err := eng.Apply(doc); err != nil {
		t.Fatal(err)
	}
	srv := proxy.NewServer(&eng, io.Discard)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx, ln) }()

	proxyAddr := ln.Addr().String()

	t.Run("allow", func(t *testing.T) {
		body, status := rawCONNECT(t, proxyAddr, net.JoinHostPort("127.0.0.1", backendPort))
		if status != 200 {
			t.Fatalf("status=%d body=%q", status, body)
		}
		if !strings.Contains(body, "hello") {
			t.Fatalf("tunnel body=%q", body)
		}
	})

	t.Run("deny", func(t *testing.T) {
		body, status := rawCONNECT(t, proxyAddr, "example.com:443")
		if status != 403 {
			t.Fatalf("status=%d body=%q", status, body)
		}
	})
}

func rawCONNECT(t *testing.T, proxyAddr, target string) (string, int) {
	t.Helper()
	c, err := net.DialTimeout("tcp", proxyAddr, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(3 * time.Second))
	_, err = c.Write([]byte("CONNECT " + target + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(c)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodConnect})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		return string(b), resp.StatusCode
	}
	buf := make([]byte, 64)
	n, _ := br.Read(buf)
	return string(buf[:n]), resp.StatusCode
}

func mustAtoi(s string) int {
	var n int
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}
