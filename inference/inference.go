// Package inference implements the privacy router (inference.local) surface.
package inference

import "context"

// Router rewrites / injects credentials for approved provider routes.
type Router interface {
	Handle(ctx context.Context, host string, body []byte) ([]byte, error)
}

// Local is a stub privacy router.
type Local struct{}

// Handle is not implemented yet (P5).
func (Local) Handle(_ context.Context, _ string, body []byte) ([]byte, error) {
	return body, nil
}
