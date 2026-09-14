// Package env maps host environment variables into sandbox guests and
// resolves credential placeholders during egress rewrite.
package env

import (
	"os"
	"strings"
)

// PlaceholderPrefix is the guest-visible credential marker written into sandbox env.
const PlaceholderPrefix = "osg:resolve:env:"

// LegacyPlaceholderPrefix is accepted on rewrite only (OpenShell-compatible alias).
const LegacyPlaceholderPrefix = "openshell:resolve:env:"

// rewritePrefixes are accepted when the proxy rewrites requests (canonical first).
var rewritePrefixes = []string{
	PlaceholderPrefix,
	LegacyPlaceholderPrefix,
}

// DefaultAllowlist is forwarded into the sandbox when present on the host.
// Credential-like keys are injected as placeholders (see FromHostForGuest).
var DefaultAllowlist = []string{
	"TERM",
	"LANG",
	"LC_ALL",
	"LC_CTYPE",
	"COLORTERM",
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"OPENROUTER_API_KEY",
	"GOOGLE_API_KEY",
	"GEMINI_API_KEY",
	"GROQ_API_KEY",
	"CURSOR_API_KEY",
	"DISPLAY",
}

// passthroughKeys keep real host values in the guest (not secrets).
var passthroughKeys = map[string]struct{}{
	"TERM": {}, "LANG": {}, "LC_ALL": {}, "LC_CTYPE": {}, "COLORTERM": {}, "DISPLAY": {},
}

// IsPassthrough reports keys that should never be placeholder-rewritten.
func IsPassthrough(key string) bool {
	_, ok := passthroughKeys[key]
	return ok
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

// FromHost builds allowlisted KEY=VAL from the current process environment (real values).
// Prefer FromHostForGuest for sandbox create when using credential placeholders.
func FromHost(extraKeys ...string) []string {
	keys := MergeAllowlists(DefaultAllowlist, extraKeys...)
	return Filter(os.Environ(), keys...)
}

// FromHostForGuest injects placeholders for credential keys and real values for passthrough keys.
// Only keys present on the host are emitted.
func FromHostForGuest(extraKeys ...string) []string {
	keys := MergeAllowlists(DefaultAllowlist, extraKeys...)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		v, ok := os.LookupEnv(k)
		if !ok {
			continue
		}
		if IsPassthrough(k) {
			out = append(out, k+"="+v)
			continue
		}
		out = append(out, k+"="+PlaceholderPrefix+k)
	}
	return out
}

// SecretsFromHost returns real KEY=VAL for credential keys present on the host (proxy sidecar).
func SecretsFromHost(extraKeys ...string) []string {
	keys := MergeAllowlists(DefaultAllowlist, extraKeys...)
	var secretKeys []string
	for _, k := range keys {
		if IsPassthrough(k) {
			continue
		}
		secretKeys = append(secretKeys, k)
	}
	return Filter(os.Environ(), secretKeys...)
}

// PlaceholderFor returns osg:resolve:env:KEY.
func PlaceholderFor(key string) string {
	return PlaceholderPrefix + key
}

// ContainsPlaceholder reports whether s embeds a credential marker.
func ContainsPlaceholder(s string) bool {
	for _, p := range rewritePrefixes {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// ParsePlaceholder extracts KEY from a full placeholder token.
func ParsePlaceholder(token string) (key string, ok bool) {
	token = strings.TrimSpace(token)
	for _, p := range rewritePrefixes {
		if !strings.HasPrefix(token, p) {
			continue
		}
		key = strings.TrimPrefix(token, p)
		if key == "" || strings.ContainsAny(key, ":/ \t") {
			return "", false
		}
		return key, true
	}
	return "", false
}

// IndexPlaceholder returns the earliest marker start index and prefix length in s.
// If none, idx is -1.
func IndexPlaceholder(s string) (idx, prefixLen int) {
	best := -1
	plen := 0
	for _, p := range rewritePrefixes {
		i := strings.Index(s, p)
		if i < 0 {
			continue
		}
		if best < 0 || i < best {
			best = i
			plen = len(p)
		}
	}
	return best, plen
}
