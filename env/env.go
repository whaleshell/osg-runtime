// Package env filters host environment into sandbox-safe KEY=VAL pairs.
package env

import (
	"os"
	"strings"
)

// DefaultAllowlist is forwarded into the sandbox when present on the host.
// No secrets beyond explicit API keys; never pass through full environ.
var DefaultAllowlist = []string{
	"TERM",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"COLORTERM",
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"CURSOR_API_KEY",
	"DISPLAY", // only meaningful with display profile later
}

// Filter keeps only allowlisted keys from environ (KEY=VAL entries).
func Filter(environ []string, keys ...string) []string {
	if len(keys) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			allowed[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(allowed))
	for _, entry := range environ {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		if _, ok := allowed[key]; ok {
			out = append(out, entry)
		}
	}
	return out
}

// MergeAllowlists returns base plus extra keys (deduped, stable order).
func MergeAllowlists(base []string, extra ...string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]string, 0, len(base)+len(extra))
	for _, k := range append(append([]string{}, base...), extra...) {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// FromHost builds allowlisted KEY=VAL from the current process environment.
func FromHost(extraKeys ...string) []string {
	keys := MergeAllowlists(DefaultAllowlist, extraKeys...)
	return Filter(os.Environ(), keys...)
}
