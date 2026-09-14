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

	"github.com/zorneth/osg-runtime/proxy"
)

func TestDialViaUpstreamProxy(t *testing.T) {
	// Fake corp proxy: accepts CONNECT and tunnels to a backend.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	go func() {
		c, err := backend.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = c.Write([]byte("upstream-ok"))
	}()
	_, bport, _ := net.SplitHostPort(backend.Addr().String())

	pln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pln.Close()
	go func() {
		c, err := pln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		req, err := http.ReadRequest(br)
		if err != nil || req.Method != http.MethodConnect {
			return
		}
		up, err := net.DialTimeout("tcp", req.Host, time.Second)
		if err != nil {
			_, _ = c.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
			return
		}
		defer up.Close()
		_, _ = c.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
		go func() { _, _ = io.Copy(up, br) }()
		_, _ = io.Copy(c, up)
	}()

	t.Setenv("HTTP_PROXY", "http://"+pln.Addr().String())
	t.Setenv("HTTPS_PROXY", "http://"+pln.Addr().String())
	t.Setenv("NO_PROXY", "")

	conn, err := proxy.DialSSRF(context.Background(), "127.0.0.1", bport, proxy.SSRFOptions{AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 32)
	n, _ := conn.Read(buf)
	if !strings.Contains(string(buf[:n]), "upstream-ok") {
		t.Fatalf("got %q", buf[:n])
	}
}
