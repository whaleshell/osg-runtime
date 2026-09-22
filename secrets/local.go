// Package secrets — LocalEncrypted persists provider credential values next to
// gateway state (OpenShell-like: values never appear in state.json plaintext).
package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// LocalEncrypted is an AES-GCM file-backed Store keyed by logical names
// (typically "provider/<name>/<ENV_KEY>").
type LocalEncrypted struct {
	mu   sync.Mutex
	dir  string
	aead cipher.AEAD
	data map[string]string // plaintext cache; disk holds ciphertext only
}

type diskBlob struct {
	Version int               `json:"version"`
	Entries map[string]string `json:"entries"` // key → base64(nonce|ciphertext)
}

// OpenLocal opens or creates an encrypted store under dir.
// KEK comes from WHALESHELL_SECRETS_KEK (raw, hex, or base64) or a generated secrets.kek file.
func OpenLocal(dir string) (*LocalEncrypted, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	kek, err := loadOrCreateKEK(dir, os.Getenv)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	s := &LocalEncrypted{
		dir:  dir,
		aead: aead,
		data: map[string]string{},
	}
	if err := s.loadLocked(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return s, nil
}

// loadOrCreateKEK resolves the KEK using Inspect order: env → file → generate file.
func loadOrCreateKEK(dir string, getenv func(string) string) ([]byte, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if v := strings.TrimSpace(getenv(EnvKEK)); v != "" {
		return ParseEnvKEK(v)
	}
	path := filepath.Join(dir, FileKEK)
	if b, err := os.ReadFile(path); err == nil && len(b) >= kekBytes {
		out := make([]byte, kekBytes)
		copy(out, b[:kekBytes])
		return out, nil
	}
	b := make([]byte, kekBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *LocalEncrypted) loadLocked() error {
	path := filepath.Join(s.dir, FileStore)
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var blob diskBlob
	if err := json.Unmarshal(raw, &blob); err != nil {
		return fmt.Errorf("secrets: parse: %w", err)
	}
	out := map[string]string{}
	for k, enc := range blob.Entries {
		pt, err := s.decrypt(enc)
		if err != nil {
			return fmt.Errorf("secrets: decrypt %s: %w", k, err)
		}
		out[k] = pt
	}
	s.data = out
	return nil
}

func (s *LocalEncrypted) flushLocked() error {
	blob := diskBlob{Version: 1, Entries: map[string]string{}}
	for k, v := range s.data {
		enc, err := s.encrypt(v)
		if err != nil {
			return err
		}
		blob.Entries[k] = enc
	}
	raw, err := json.MarshalIndent(blob, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(s.dir, FileStore)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *LocalEncrypted) encrypt(plain string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := s.aead.Seal(nonce, nonce, []byte(plain), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func (s *LocalEncrypted) decrypt(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	ns := s.aead.NonceSize()
	if len(raw) < ns {
		return "", fmt.Errorf("ciphertext too short")
	}
	pt, err := s.aead.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

// Get implements Store.
func (s *LocalEncrypted) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key = strings.TrimSpace(key)
	if key == "" {
		return "", fmt.Errorf("secrets: empty key")
	}
	v, ok := s.data[key]
	if !ok {
		return "", ErrNotFound{Key: key}
	}
	return v, nil
}

// Put stores a plaintext value under key.
func (s *LocalEncrypted) Put(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("secrets: empty key")
	}
	if value == "" {
		return fmt.Errorf("secrets: empty value for %q", key)
	}
	s.data[key] = value
	return s.flushLocked()
}

// Delete removes a key.
func (s *LocalEncrypted) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return s.flushLocked()
}

// DeletePrefix removes all keys with the given prefix.
func (s *LocalEncrypted) DeletePrefix(_ context.Context, prefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			delete(s.data, k)
		}
	}
	return s.flushLocked()
}

// ProviderKey builds the canonical store key for a provider credential.
func ProviderKey(providerName, envKey string) string {
	return "provider/" + providerName + "/" + envKey
}

// PutProviderCredentials writes all credential values for a provider instance.
func (s *LocalEncrypted) PutProviderCredentials(ctx context.Context, providerName string, creds map[string]string) error {
	for k, v := range creds {
		k = strings.TrimSpace(k)
		if k == "" || strings.TrimSpace(v) == "" {
			continue
		}
		if err := s.Put(ctx, ProviderKey(providerName, k), v); err != nil {
			return err
		}
	}
	return nil
}

// GetProviderCredentials returns env KEY→value for the listed env keys.
// Missing keys are skipped (not an error).
func (s *LocalEncrypted) GetProviderCredentials(ctx context.Context, providerName string, envKeys []string) (map[string]string, error) {
	out := map[string]string{}
	for _, k := range envKeys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		v, err := s.Get(ctx, ProviderKey(providerName, k))
		if err != nil {
			continue
		}
		if v != "" {
			out[k] = v
		}
	}
	return out, nil
}
