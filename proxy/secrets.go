package proxy

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/zorneth/osg-runtime/env"
)

// SecretStore maps env key to secret value for placeholder rewrite.
type SecretStore map[string]string

// FilterSecrets returns a copy of secrets limited to allowed keys.
// Empty allowed means no filter (all keys).
func FilterSecrets(secrets SecretStore, allowed []string) SecretStore {
	if len(allowed) == 0 || secrets == nil {
		return secrets
	}
	out := make(SecretStore, len(allowed))
	for _, k := range allowed {
		if v, ok := secrets[k]; ok {
			out[k] = v
		}
	}
	return out
}

// LoadSecretsFromEnviron builds a store from KEY=VAL entries (skips passthrough keys).
func LoadSecretsFromEnviron(environ []string) SecretStore {
	out := make(SecretStore)
	for _, entry := range environ {
		k, v, ok := strings.Cut(entry, "=")
		if !ok || k == "" || v == "" {
			continue
		}
		if env.IsPassthrough(k) {
			continue
		}
		out[k] = v
	}
	return out
}

// ContainsPlaceholder reports whether s has a reserved credential marker.
func ContainsPlaceholder(s string) bool {
	return env.ContainsPlaceholder(s)
}

// RewriteHTTPRequest resolves placeholders in path, query, and headers (incl. Basic).
// Fail-closed: unresolved markers return an error (do not send upstream).
func RewriteHTTPRequest(req *http.Request, secrets SecretStore) error {
	if req == nil {
		return nil
	}
	if req.URL != nil {
		path, err := rewritePath(req.URL.EscapedPath(), secrets)
		if err != nil {
			return err
		}
		rawQuery, err := rewriteQuery(req.URL.RawQuery, secrets)
		if err != nil {
			return err
		}
		req.URL.Path = path
		req.URL.RawPath = ""
		req.URL.RawQuery = rawQuery
		if req.URL.Scheme == "" && req.RequestURI != "" {
			// absolute-form already handled via URL fields
		}
	}
	for k, vv := range req.Header {
		for i, v := range vv {
			nv, err := rewriteHeaderValue(v, secrets)
			if err != nil {
				return fmt.Errorf("header %s: %w", k, err)
			}
			if nv != v {
				vv[i] = nv
			}
		}
		req.Header[k] = vv
	}
	return nil
}

// RewriteText resolves all placeholders in an arbitrary string (WS text frames).
func RewriteText(text string, secrets SecretStore) (string, error) {
	return rewriteText(text, secrets)
}

func rewriteHeaderValue(value string, secrets SecretStore) (string, error) {
	trimmed := strings.TrimSpace(value)
	// Basic auth: placeholder lives inside base64 — check before ContainsPlaceholder short-circuit.
	if strings.HasPrefix(strings.ToLower(trimmed), "basic ") {
		enc := strings.TrimSpace(trimmed[6:])
		raw, err := base64.StdEncoding.DecodeString(enc)
		if err != nil {
			if ContainsPlaceholder(trimmed) {
				return "", fmt.Errorf("basic auth decode: %w", err)
			}
			return value, nil
		}
		decoded := string(raw)
		if ContainsPlaceholder(decoded) {
			rewritten, err := rewriteText(decoded, secrets)
			if err != nil {
				return "", err
			}
			return "Basic " + base64.StdEncoding.EncodeToString([]byte(rewritten)), nil
		}
		return value, nil
	}
	if !ContainsPlaceholder(trimmed) {
		return value, nil
	}
	// Exact placeholder
	if secret, ok := resolveExact(trimmed, secrets); ok {
		return secret, nil
	}
	// Prefixed: Bearer osg:resolve:env:KEY
	if i := strings.IndexFunc(trimmed, func(r rune) bool { return r == ' ' || r == '\t' }); i > 0 {
		prefix := trimmed[:i]
		cand := strings.TrimSpace(trimmed[i:])
		if secret, ok := resolveExact(cand, secrets); ok {
			return prefix + " " + secret, nil
		}
		if ContainsPlaceholder(cand) {
			return "", fmt.Errorf("unresolved placeholder in header")
		}
	}
	if ContainsPlaceholder(trimmed) {
		return rewriteText(trimmed, secrets)
	}
	return value, nil
}

func resolveExact(token string, secrets SecretStore) (string, bool) {
	key, ok := env.ParsePlaceholder(token)
	if !ok {
		return "", false
	}
	secret, found := secrets[key]
	return secret, found && secret != ""
}

func rewriteText(text string, secrets SecretStore) (string, error) {
	if !ContainsPlaceholder(text) {
		return text, nil
	}
	var b strings.Builder
	b.Grow(len(text))
	rest := text
	for {
		i, prefixLen := env.IndexPlaceholder(rest)
		if i < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:i])
		rest = rest[i:]
		if prefixLen > len(rest) {
			return "", fmt.Errorf("unresolved placeholder")
		}
		j := prefixLen
		for j < len(rest) {
			c := rest[j]
			if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
				j++
				continue
			}
			break
		}
		if j == prefixLen {
			return "", fmt.Errorf("unresolved placeholder")
		}
		key := rest[prefixLen:j]
		secret, ok := secrets[key]
		if !ok || secret == "" {
			return "", fmt.Errorf("unresolved placeholder %s%s", env.PlaceholderPrefix, key)
		}
		b.WriteString(secret)
		rest = rest[j:]
	}
	return b.String(), nil
}

func rewritePath(escapedPath string, secrets SecretStore) (string, error) {
	if escapedPath == "" || !ContainsPlaceholder(escapedPath) {
		// Also check decoded form
		decoded, err := url.PathUnescape(escapedPath)
		if err != nil || !ContainsPlaceholder(decoded) {
			return escapedPath, nil
		}
		rewritten, err := rewriteText(decoded, secrets)
		if err != nil {
			return "", err
		}
		return rewritten, nil
	}
	decoded, err := url.PathUnescape(escapedPath)
	if err != nil {
		decoded = escapedPath
	}
	return rewriteText(decoded, secrets)
}

func rewriteQuery(raw string, secrets SecretStore) (string, error) {
	if raw == "" || !ContainsPlaceholder(raw) {
		return raw, nil
	}
	q, err := url.ParseQuery(raw)
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}
	changed := false
	for k, vv := range q {
		for i, v := range vv {
			nv, err := rewriteText(v, secrets)
			if err != nil {
				return "", fmt.Errorf("query %s: %w", k, err)
			}
			if nv != v {
				vv[i] = nv
				changed = true
			}
		}
		q[k] = vv
	}
	if !changed {
		return raw, nil
	}
	return q.Encode(), nil
}
