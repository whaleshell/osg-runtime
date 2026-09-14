// Package refresh defines managed credential refresh backends.
// Stub fails closed; MVP re-injects secrets via sandbox recreate / proxy env.
package refresh

import (
	"context"
	"errors"
	"fmt"
)

// Result is one refreshed secret.
type Result struct {
	Key   string
	Value string
}

// Refresher obtains updated credentials for allowlisted keys.
type Refresher interface {
	Refresh(ctx context.Context, keys []string) ([]Result, error)
}

// ErrNotImplemented is returned by Stub.
var ErrNotImplemented = errors.New("refresh: not implemented — see docs/CREDENTIALS.md")

// Stub fails closed until a vault/OIDC refresh RPC lands.
type Stub struct{}

// Refresh always fails closed.
func (Stub) Refresh(context.Context, []string) ([]Result, error) {
	return nil, fmt.Errorf("%w", ErrNotImplemented)
}

var _ Refresher = Stub{}
