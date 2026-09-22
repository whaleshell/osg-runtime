//go:build !linux

package harden

import (
	"fmt"

	"github.com/whaleshell/whaleshell-core/policy"
)

func landlockABI() (int, error) {
	return 0, fmt.Errorf("landlock only available on linux")
}

func applyLandlock(_ policy.Document) error {
	return fmt.Errorf("landlock only available on linux")
}
