package proxy

import (
	"encoding/json"
	"time"
)

type auditEvent struct {
	TS     string `json:"ts"`
	Action string `json:"action"`
	Host   string `json:"host,omitempty"`
	Port   int    `json:"port,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	Binary string `json:"binary,omitempty"`
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
}

func (s *Server) logAudit(ev auditEvent) {
	if s.audit == nil {
		return
	}
	ev.TS = time.Now().UTC().Format(time.RFC3339Nano)
	enc := json.NewEncoder(s.audit)
	_ = enc.Encode(ev)
}
