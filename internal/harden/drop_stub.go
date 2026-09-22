//go:build !linux

package harden

import "fmt"

func dropPrivileges() error {
	return fmt.Errorf("privilege drop only available on linux")
}
