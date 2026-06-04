package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/devenjarvis/lathe/internal/store"
)

func TestVerifyCommandFlipsMetadataToVerifying(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("PATH", "") // prevent claude from being found

	tutDir := filepath.Join(homeDir, ".lathe", "tutorials", "test-slug")
	if err := os.MkdirAll(tutDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tutDir, "part-01.md"), []byte("# Part 1"), 0644); err != nil {
		t.Fatal(err)
	}
	tut := &store.Tutorial{
		Slug:   "test-slug",
		Title:  "Test Slug",
		Status: store.StatusUnverified,
		Parts:  []string{"part-01.md"},
	}
	if err := store.WriteMetadata(tutDir, tut); err != nil {
		t.Fatal(err)
	}

	// PATH="" means SpawnVerifier fails fast, but the metadata flip to
	// verifying happens before spawn.
	verifyCmd.RunE(verifyCmd, []string{"test-slug"}) //nolint:errcheck

	got, err := store.ReadMetadata(tutDir)
	if err != nil {
		t.Fatalf("ReadMetadata after verify: %v", err)
	}
	if got.Status != store.StatusVerifying {
		t.Errorf("Status = %q, want %q", got.Status, store.StatusVerifying)
	}
}

func TestVerifyCommandRejectsBadSlug(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, slug := range []string{"", ".", "..", "foo/bar", `foo\bar`} {
		if err := verifyCmd.RunE(verifyCmd, []string{slug}); err == nil {
			t.Errorf("verify %q should be rejected as invalid", slug)
		}
	}
}
