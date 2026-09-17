package secrets

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Well-known secrets paths / env (single source of truth — do not hardcode elsewhere).
const (
	EnvKEK    = "OSG_SECRETS_KEK"
	FileKEK   = "secrets.kek"
	FileStore = "secrets.enc.json"
	kekBytes  = 32
)

// Source identifies where the active KEK came from.
type Source string

const (
	SourceEnv  Source = "env"
	SourceFile Source = "file"
	SourceNone Source = "none"
)

// Status describes KEK durability for doctor /gateway info.
// Pinned is true when OSG_SECRETS_KEK is set (survives empty data-dir recreate
// as long as the same env is supplied). File-backed KEK survives volume recreate
// but is lost if the volume is deleted without a pinned env.
type Status struct {
	Source Source `json:"source"`
	Pinned bool   `json:"pinned"`
	Path   string `json:"path,omitempty"`
}

// Inspect reports KEK status without creating files.
// getenv may be nil (defaults to os.Getenv).
func Inspect(dir string, getenv func(string) string) Status {
	if getenv == nil {
		getenv = os.Getenv
	}
	if strings.TrimSpace(getenv(EnvKEK)) != "" {
		return Status{Source: SourceEnv, Pinned: true}
	}
	path := filepath.Join(dir, FileKEK)
	if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Size() >= int64(kekBytes) {
		return Status{Source: SourceFile, Pinned: false, Path: path}
	}
	return Status{Source: SourceNone, Pinned: false, Path: path}
}

// DeriveKEK normalizes arbitrary material to a 32-byte AES key via SHA-256.
func DeriveKEK(material []byte) []byte {
	sum := sha256.Sum256(material)
	out := make([]byte, kekBytes)
	copy(out, sum[:])
	return out
}

// ParseEnvKEK decodes OSG_SECRETS_KEK values.
// Accepts raw passphrase, standard base64, or hex (≥16 bytes decoded).
func ParseEnvKEK(v string) ([]byte, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, fmt.Errorf("secrets: empty %s", EnvKEK)
	}
	if b, err := base64.StdEncoding.DecodeString(v); err == nil && len(b) >= 16 {
		return DeriveKEK(b), nil
	}
	if b, err := hex.DecodeString(v); err == nil && len(b) >= 16 {
		return DeriveKEK(b), nil
	}
	return DeriveKEK([]byte(v)), nil
}

// Warning returns an operator-facing hint when KEK is not pinned via env.
func (s Status) Warning() string {
	switch s.Source {
	case SourceEnv:
		return ""
	case SourceFile:
		return fmt.Sprintf("secrets KEK is file-backed (%s); set %s so recreating the data volume without the file still decrypts", s.Path, EnvKEK)
	default:
		return fmt.Sprintf("secrets KEK not pinned; set %s (or rely on persistent %s in the gateway data volume)", EnvKEK, FileKEK)
	}
}
