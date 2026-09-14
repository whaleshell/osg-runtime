package middleware

import (
	"encoding/base64"
	"strings"
)

func decodeSegment(seg string) ([]byte, error) {
	seg = strings.TrimRight(seg, "=")
	// JWT uses base64url without padding
	if b, err := base64.RawURLEncoding.DecodeString(seg); err == nil {
		return b, nil
	}
	return base64.URLEncoding.DecodeString(seg)
}
