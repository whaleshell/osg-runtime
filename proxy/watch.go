package proxy

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/zorneth/osg-core/policy"
)

// WatchPolicy polls path for content changes and calls Apply on the server.
// Interval defaults to 1s. Stops when ctx is done.
func (s *Server) WatchPolicy(ctx context.Context, path string, interval time.Duration) {
	if s == nil || strings.TrimSpace(path) == "" {
		return
	}
	if interval <= 0 {
		interval = time.Second
	}
	var lastMod time.Time
	var lastSize int64
	if fi, err := os.Stat(path); err == nil {
		lastMod = fi.ModTime()
		lastSize = fi.Size()
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fi, err := os.Stat(path)
			if err != nil {
				continue
			}
			if fi.ModTime().Equal(lastMod) && fi.Size() == lastSize {
				continue
			}
			lastMod = fi.ModTime()
			lastSize = fi.Size()
			doc, err := policy.Load(path)
			if err != nil {
				s.logAudit(auditEvent{Action: "error", Reason: "policy reload: " + err.Error(), Allow: false})
				continue
			}
			if err := s.Apply(ctx, doc); err != nil {
				s.logAudit(auditEvent{Action: "error", Reason: "policy apply: " + err.Error(), Allow: false})
				continue
			}
			s.logAudit(auditEvent{Action: "reload", Reason: fmt.Sprintf("policy reloaded from %s", path), Allow: true})
		}
	}
}
