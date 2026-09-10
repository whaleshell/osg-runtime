// Package harden applies Landlock, seccomp, and privilege drop inside the guest.
//
// Linux-only implementation will use //go:build linux; this stub builds everywhere.
package harden

import (
	"context"
	"fmt"

	"github.com/lkmavi/osg-core"
	"github.com/lkmavi/osg-core/policy"
)

// Mode controls fail-closed vs loud best-effort.
type Mode string

const (
	ModeBestEffort Mode = "best_effort"
	ModeRequired   Mode = "required"
)

// Appliers run before exec of the agent process.
type Applier interface {
	Apply(ctx context.Context, doc policy.Document, mode Mode) error
}

// Stub does nothing yet (P4).
type Stub struct{}

// Apply returns not implemented.
func (Stub) Apply(_ context.Context, _ policy.Document, _ Mode) error {
	return fmt.Errorf("harden: %w", core.ErrNotImplemented)
}
