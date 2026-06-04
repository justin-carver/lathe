package cmd

import (
	"testing"

	"github.com/devenjarvis/lathe/internal/store"
)

func TestParseTools(t *testing.T) {
	got := parseTools([]string{"zig:0.13.0", "go:1.22", "make"})
	want := []store.Tool{
		{Name: "zig", Version: "0.13.0"},
		{Name: "go", Version: "1.22"},
		{Name: "make", Version: ""}, // no ":" → name only, empty version
	}
	if len(got) != len(want) {
		t.Fatalf("parseTools = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseTools[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestParseToolsSplitsOnFirstColon(t *testing.T) {
	// A version that itself contains ":" must survive — only the first ":" splits.
	got := parseTools([]string{"docker:image:tag"})
	if len(got) != 1 || got[0] != (store.Tool{Name: "docker", Version: "image:tag"}) {
		t.Errorf("parseTools = %v, want [{docker image:tag}]", got)
	}
}
