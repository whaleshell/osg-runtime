// Package audit emits structured security / lifecycle events (JSONL → OCSF).
package audit

import "log/slog"

// Emitter writes one audit event.
type Emitter interface {
	Emit(event map[string]any)
}

// SlogEmitter is a simple slog-backed emitter for MVP.
type SlogEmitter struct {
	Log *slog.Logger
}

// Emit logs the event at Info level.
func (e SlogEmitter) Emit(event map[string]any) {
	if e.Log == nil {
		return
	}
	e.Log.Info("audit", "event", event)
}
