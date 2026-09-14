package env_test

import (
	"os"
	"strings"
	"testing"

	"github.com/zorneth/osg-runtime/env"
)

func TestFromHostForGuestPlaceholders(t *testing.T) {
	t.Setenv("TERM", "xterm")
	t.Setenv("ANTHROPIC_API_KEY", "sk-real")
	got := env.FromHostForGuest("ANTHROPIC_API_KEY")
	var term, key string
	for _, e := range got {
		k, v, _ := strings.Cut(e, "=")
		switch k {
		case "TERM":
			term = v
		case "ANTHROPIC_API_KEY":
			key = v
		}
	}
	if term != "xterm" {
		t.Fatalf("TERM=%q", term)
	}
	if key != env.PlaceholderPrefix+"ANTHROPIC_API_KEY" {
		t.Fatalf("key=%q", key)
	}
	secrets := env.SecretsFromHost("ANTHROPIC_API_KEY")
	found := false
	for _, e := range secrets {
		if e == "ANTHROPIC_API_KEY=sk-real" {
			found = true
		}
	}
	if !found {
		t.Fatalf("secrets=%v environ has %v", secrets, os.Getenv("ANTHROPIC_API_KEY"))
	}
}
