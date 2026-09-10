// Package proxy implements mandatory egress (CONNECT and later L7).
package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/lkmavi/osg-core"
	"github.com/lkmavi/osg-core/engine"
	"github.com/lkmavi/osg-core/policy"
)

// EgressProxy applies policy and serves egress for a sandbox network.
type EgressProxy interface {
	Apply(ctx context.Context, doc policy.Document) error
	Close(ctx context.Context) error
}

// Server is a default-deny HTTP CONNECT proxy backed by engine.PolicyEngine.
type Server struct {
	mu     sync.RWMutex
	eng    engine.PolicyEngine
	audit  io.Writer
	server *http.Server
}

// NewServer builds a CONNECT proxy. audit defaults to stderr.
func NewServer(eng engine.PolicyEngine, audit io.Writer) *Server {
	if eng == nil {
		eng = &engine.Allowlist{}
	}
	if audit == nil {
		audit = os.Stderr
	}
	return &Server{eng: eng, audit: audit}
}

// Apply reloads policy (hot-reload safe).
func (s *Server) Apply(_ context.Context, doc policy.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.eng == nil {
		return fmt.Errorf("proxy: %w", core.ErrNotImplemented)
	}
	return s.eng.Apply(doc)
}

// Close shuts down the HTTP server if Serve was used.
func (s *Server) Close(ctx context.Context) error {
	s.mu.RLock()
	srv := s.server
	s.mu.RUnlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}

// Handler returns the HTTP handler (CONNECT + /healthz).
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.serveHTTP)
}

// ListenAndServe listens on addr until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("proxy listen: %w", err)
	}
	return s.Serve(ctx, ln)
}

// Serve serves CONNECT on ln until ctx is cancelled.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.mu.Lock()
	s.server = srv
	s.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if err == http.ErrServerClosed {
			return ctx.Err()
		}
		return err
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" && r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
		return
	}
	if r.Method != http.MethodConnect {
		http.Error(w, "CONNECT only", http.StatusMethodNotAllowed)
		s.logAudit(auditEvent{
			Action: "reject",
			Host:   r.Host,
			Reason: "method " + r.Method,
			Allow:  false,
		})
		return
	}

	host, portStr, err := net.SplitHostPort(r.Host)
	if err != nil {
		// CONNECT host without port — assume 443
		host = r.Host
		portStr = "443"
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		http.Error(w, "bad port", http.StatusBadRequest)
		s.logAudit(auditEvent{Action: "reject", Host: r.Host, Reason: "bad port", Allow: false})
		return
	}

	s.mu.RLock()
	eng := s.eng
	s.mu.RUnlock()
	if eng == nil {
		http.Error(w, "proxy not configured", http.StatusServiceUnavailable)
		return
	}
	dec, err := eng.Decide(r.Context(), engine.EgressRequest{Host: host, Port: port})
	if err != nil {
		http.Error(w, "policy error", http.StatusInternalServerError)
		s.logAudit(auditEvent{Action: "error", Host: host, Port: port, Reason: err.Error(), Allow: false})
		return
	}
	if !dec.Allow {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("osg-proxy: denied\n"))
		s.logAudit(auditEvent{Action: "deny", Host: host, Port: port, Reason: dec.Reason, Allow: false})
		return
	}

	dest := net.JoinHostPort(host, portStr)
	backend, err := net.DialTimeout("tcp", dest, 15*time.Second)
	if err != nil {
		http.Error(w, "dial failed", http.StatusBadGateway)
		s.logAudit(auditEvent{Action: "dial_error", Host: host, Port: port, Reason: err.Error(), Allow: true})
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = backend.Close()
		http.Error(w, "hijack unsupported", http.StatusInternalServerError)
		return
	}
	clientConn, bufrw, err := hj.Hijack()
	if err != nil {
		_ = backend.Close()
		return
	}
	_, _ = bufrw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
	_ = bufrw.Flush()
	s.logAudit(auditEvent{Action: "allow", Host: host, Port: port, Reason: dec.Reason, Allow: true})

	go tunnel(backend, clientConn)
}

func tunnel(a, b net.Conn) {
	defer a.Close()
	defer b.Close()
	errCh := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(a, b)
		errCh <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(b, a)
		errCh <- struct{}{}
	}()
	<-errCh
}

// CONNECT is kept as a thin Apply-only adapter for sandbox.Manager.
type CONNECT struct {
	*Server
}

// NewCONNECT wraps an engine in a Server-backed EgressProxy.
func NewCONNECT(eng engine.PolicyEngine) *CONNECT {
	return &CONNECT{Server: NewServer(eng, nil)}
}
