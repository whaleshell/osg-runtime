package proxy

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zorneth/osg-core/engine"
	"github.com/zorneth/osg-core/policy"
)

func TestWatchPolicyReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")
	body1 := []byte("version: 1\nnetwork:\n  default: deny\n  allow:\n    - id: a\n      host: a.example.com\n      port: 443\n")
	if err := os.WriteFile(path, body1, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := policy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var eng engine.Allowlist
	if err := eng.Apply(doc); err != nil {
		t.Fatal(err)
	}
	srv := NewServer(&eng, os.Stderr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go srv.WatchPolicy(ctx, path, 50*time.Millisecond)

	body2 := []byte("version: 1\nnetwork:\n  default: deny\n  allow:\n    - id: b\n      host: b.example.com\n      port: 443\n")
	time.Sleep(80 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(body2); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		dec, err := eng.Decide(context.Background(), engine.EgressRequest{Host: "b.example.com", Port: 443})
		if err == nil && dec.Allow {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("policy did not reload to allow b.example.com")
}
