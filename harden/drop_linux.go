//go:build linux

package harden

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// dropPrivileges sets no_new_privs and switches to OSG_UID/OSG_GID or "nobody".
func dropPrivileges() error {
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS: %w", err)
	}
	uid, gid, err := targetIDs()
	if err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		if err := syscall.Setgid(gid); err != nil {
			return fmt.Errorf("setgid %d: %w", gid, err)
		}
		if err := syscall.Setuid(uid); err != nil {
			return fmt.Errorf("setuid %d: %w", uid, err)
		}
	}
	return nil
}

func targetIDs() (uid, gid int, err error) {
	if v := os.Getenv("OSG_UID"); v != "" {
		uid, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, fmt.Errorf("OSG_UID: %w", err)
		}
	}
	if v := os.Getenv("OSG_GID"); v != "" {
		gid, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, fmt.Errorf("OSG_GID: %w", err)
		}
	}
	if uid != 0 || gid != 0 {
		if gid == 0 {
			gid = uid
		}
		if uid == 0 {
			uid = gid
		}
		return uid, gid, nil
	}
	u, err := user.Lookup("nobody")
	if err != nil {
		// distro fallback
		return 65534, 65534, nil
	}
	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}
