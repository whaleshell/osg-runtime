package secrets

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalEncryptedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.PutProviderCredentials(ctx, "cursor", map[string]string{"CURSOR_API_KEY": "crsr_test"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProviderCredentials(ctx, "cursor", []string{"CURSOR_API_KEY", "MISSING"})
	if err != nil {
		t.Fatal(err)
	}
	if got["CURSOR_API_KEY"] != "crsr_test" {
		t.Fatalf("got %#v", got)
	}
	if _, ok := got["MISSING"]; ok {
		t.Fatal("missing should be omitted")
	}
	s2, err := OpenLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := s2.Get(ctx, ProviderKey("cursor", "CURSOR_API_KEY"))
	if err != nil || v != "crsr_test" {
		t.Fatalf("reopen: %q %v", v, err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, FileStore))
	if strings.Contains(string(raw), "crsr_test") {
		t.Fatal("plaintext leaked into secrets.enc.json")
	}
}
