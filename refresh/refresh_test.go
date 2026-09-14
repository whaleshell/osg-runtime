package refresh

import (
	"context"
	"errors"
	"testing"
)

func TestStub(t *testing.T) {
	_, err := Stub{}.Refresh(context.Background(), []string{"TOKEN"})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err=%v", err)
	}
}
