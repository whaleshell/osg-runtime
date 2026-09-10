package env

import (
	"reflect"
	"testing"
)

func TestFilter(t *testing.T) {
	in := []string{
		"TERM=xterm",
		"SECRET=nope",
		"ANTHROPIC_API_KEY=sk-test",
		"BAD",
		"LANG=C",
	}
	got := Filter(in, DefaultAllowlist...)
	want := []string{"TERM=xterm", "ANTHROPIC_API_KEY=sk-test", "LANG=C"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestMergeAllowlists(t *testing.T) {
	got := MergeAllowlists([]string{"TERM", "LANG"}, "LANG", "CURSOR_API_KEY", "")
	want := []string{"TERM", "LANG", "CURSOR_API_KEY"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
