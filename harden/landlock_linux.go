//go:build linux

package harden

import (
	"fmt"
	"os"
	"strings"

	ll "github.com/landlock-lsm/go-landlock/landlock"
	"github.com/zorneth/osg-core/policy"
	"golang.org/x/sys/unix"
)

const landlockCreateRulesetVersion = 1 << 0

func landlockABI() (int, error) {
	r1, _, errno := unix.Syscall(
		unix.SYS_LANDLOCK_CREATE_RULESET,
		0, 0, landlockCreateRulesetVersion,
	)
	if errno != 0 {
		return 0, errno
	}
	return int(r1), nil
}

func applyLandlock(doc policy.Document) error {
	reads := []string{"/usr", "/lib", "/lib64", "/bin", "/sbin", "/etc", "/proc", "/dev", "/sys", "/app", "/osg", "/tmp", "/var", "/home", "/run"}
	writes := []string{"/tmp", "/dev/null", "/dev/zero", "/dev/urandom", "/dev/tty", "/workspace", "/run", "/home", "/var/tmp"}
	if doc.Filesystem != nil {
		if len(doc.Filesystem.Read) > 0 {
			reads = append([]string{}, doc.Filesystem.Read...)
			reads = append(reads, "/proc", "/dev", "/osg", "/tmp")
		}
		if len(doc.Filesystem.Write) > 0 {
			writes = append([]string{}, doc.Filesystem.Write...)
		}
		if doc.Filesystem.IncludeWorkdir {
			writes = append(writes, "/workspace")
		}
	}
	if doc.Display != nil && strings.EqualFold(doc.Display.Mode, "novnc") {
		reads = append(reads, "/tmp/.X11-unix", "/usr/share", "/usr/lib",
			"/etc/chromium", "/usr/lib/chromium", "/usr/bin/chromium")
		writes = append(writes, "/tmp/.X11-unix", "/tmp/osg-display", "/home", "/run/user")
	}
	// Always allow X11 socket dir when present (gui boot / best_effort).
	reads = append(reads, "/tmp/.X11-unix")
	writes = append(writes, "/tmp/.X11-unix", "/tmp/osg-display")
	reads = unique(reads)
	writes = unique(writes)

	cfg := ll.V5.BestEffort()
	if doc.HardenMode() == "required" {
		cfg = ll.V5
	}
	rules := make([]ll.PathOpt, 0, 2)
	if ro := existing(reads); len(ro) > 0 {
		rules = append(rules, ll.RODirs(ro...))
	}
	if rw := existing(writes); len(rw) > 0 {
		rules = append(rules, ll.RWDirs(rw...))
	}
	if len(rules) == 0 {
		return fmt.Errorf("no filesystem paths to allow")
	}
	return cfg.RestrictPaths(rules...)
}

func existing(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
