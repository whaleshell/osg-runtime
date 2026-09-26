// Package relayclient is the supervisor side of the gateway relay (OpenShell
// ConnectSupervisor): it keeps an outbound control stream to the gateway and,
// for every "open" request, dials the local sandbox SSH socket and a fresh
// outbound data stream, then pipes them. The sandbox never listens on TCP.
package relayclient

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/whaleshell/whaleshell-core/relayproto"
)

// Config configures Run.
type Config struct {
	GatewayURL string
	Sandbox    string
	// Token is the sandbox-scoped supervisor token (never a user token).
	Token string
	// SSHSocket is the sandbox sshd Unix socket (TargetSSH).
	SSHSocket string
	TLSConfig *tls.Config
	Log       *slog.Logger
	// DialTarget overrides how targets are dialed (tests).
	DialTarget func(ctx context.Context, target string) (net.Conn, error)
	MinBackoff time.Duration
	MaxBackoff time.Duration
	// OnConnected is called after each successful control handshake (tests).
	OnConnected func()
}

func (c *Config) defaults() {
	if c.Log == nil {
		c.Log = slog.Default()
	}
	if c.MinBackoff <= 0 {
		c.MinBackoff = time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 30 * time.Second
	}
	if c.DialTarget == nil {
		sock := c.SSHSocket
		c.DialTarget = func(ctx context.Context, target string) (net.Conn, error) {
			if target != relayproto.TargetSSH {
				return nil, fmt.Errorf("relay target %q not permitted", target)
			}
			var d net.Dialer
			return d.DialContext(ctx, "unix", sock)
		}
	}
}

// Run keeps the supervisor session alive until ctx is done.
func Run(ctx context.Context, cfg Config) error {
	cfg.defaults()
	if strings.TrimSpace(cfg.GatewayURL) == "" || strings.TrimSpace(cfg.Sandbox) == "" || strings.TrimSpace(cfg.Token) == "" {
		return errors.New("relayclient: gateway url, sandbox and token are required")
	}
	backoff := cfg.MinBackoff
	for {
		started := time.Now()
		err := session(ctx, cfg)
		if ctx.Err() != nil {
			return nil
		}
		if time.Since(started) > 2*relayproto.KeepaliveTimeout {
			backoff = cfg.MinBackoff
		}
		attrs := []any{slog.String("op", "supervisor.reconnect"), slog.String("sandbox", cfg.Sandbox), slog.Duration("backoff", backoff)}
		if err != nil {
			attrs = append(attrs, slog.String("error", err.Error()))
		}
		var se *relayproto.StatusError
		if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
			cfg.Log.Error("supervisor token rejected by gateway", attrs...)
		} else {
			cfg.Log.Warn("supervisor session ended", attrs...)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > cfg.MaxBackoff {
			backoff = cfg.MaxBackoff
		}
	}
}

func (cfg *Config) header() http.Header {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+cfg.Token)
	return h
}

func session(ctx context.Context, cfg Config) error {
	path := relayproto.PathSupervisorConnect + "?sandbox=" + url.QueryEscape(cfg.Sandbox)
	conn, err := relayproto.Dial(ctx, cfg.GatewayURL, path, relayproto.DialOptions{Header: cfg.header(), TLSConfig: cfg.TLSConfig})
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	mw := relayproto.NewMessageWriter(conn)
	if err := mw.Write(relayproto.Message{Type: relayproto.MsgHello, Sandbox: cfg.Sandbox}); err != nil {
		return err
	}
	cfg.Log.Info("supervisor connected", slog.String("op", "supervisor.connect"), slog.String("sandbox", cfg.Sandbox))
	if cfg.OnConnected != nil {
		cfg.OnConnected()
	}
	mr := relayproto.NewMessageReader(conn)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(relayproto.KeepaliveTimeout))
		m, err := mr.Read()
		if err != nil {
			return err
		}
		switch m.Type {
		case relayproto.MsgPing:
			if err := mw.Write(relayproto.Message{Type: relayproto.MsgPong}); err != nil {
				return err
			}
		case relayproto.MsgOpen:
			if m.Channel == "" {
				continue
			}
			go openChannel(ctx, cfg, m)
		}
	}
}

func openChannel(ctx context.Context, cfg Config, m relayproto.Message) {
	log := cfg.Log.With(slog.String("sandbox", cfg.Sandbox), slog.String("channel", m.Channel))
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	target, err := cfg.DialTarget(dctx, m.Target)
	if err != nil {
		log.Warn("relay target dial failed", slog.String("op", "supervisor.open"), slog.String("target", m.Target), slog.String("error", err.Error()))
		return
	}
	data, err := relayproto.Dial(dctx, cfg.GatewayURL, relayproto.PathSupervisorRelay+url.PathEscape(m.Channel),
		relayproto.DialOptions{Header: cfg.header(), TLSConfig: cfg.TLSConfig})
	if err != nil {
		_ = target.Close()
		log.Warn("relay data stream failed", slog.String("op", "supervisor.open"), slog.String("error", err.Error()))
		return
	}
	log.Debug("relay channel open", slog.String("op", "supervisor.open"), slog.String("target", m.Target))
	relayproto.Pipe(data, target)
}
