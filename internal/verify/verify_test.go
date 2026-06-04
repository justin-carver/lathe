package verify

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/devenjarvis/lathe/internal/store"
)

func TestSpawnVerifierMissingClaude(t *testing.T) {
	tutDir := t.TempDir()

	// Remove PATH so claude binary can't be found
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	err := SpawnVerifier("test-slug", tutDir)
	if err == nil {
		t.Error("SpawnVerifier() expected error when claude not in PATH, got nil")
	}
}

func writeTut(t *testing.T, dir string, status store.Status) {
	t.Helper()
	tut := &store.Tutorial{
		Slug:   "test-slug",
		Title:  "Test Slug",
		Status: status,
		Parts:  []string{"part-01.md"},
	}
	if err := store.WriteMetadata(dir, tut); err != nil {
		t.Fatal(err)
	}
}

func TestStartVerificationSetsVerifying(t *testing.T) {
	tutDir := t.TempDir()
	writeTut(t, tutDir, store.StatusUnverified)

	// PATH="" makes the spawn fail fast, but StartVerification writes
	// status=verifying BEFORE spawning, so we observe it regardless.
	origPath := os.Getenv("PATH")
	os.Setenv("PATH", "")
	defer os.Setenv("PATH", origPath)

	StartVerification("test-slug", tutDir) //nolint:errcheck

	got, err := store.ReadMetadata(tutDir)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if got.Status != store.StatusVerifying {
		t.Errorf("Status = %q, want %q", got.Status, store.StatusVerifying)
	}
}

func TestStartVerificationRejectsWhileVerifying(t *testing.T) {
	tutDir := t.TempDir()
	writeTut(t, tutDir, store.StatusVerifying)

	if err := StartVerification("test-slug", tutDir); err == nil {
		t.Error("StartVerification() should reject a tutorial already verifying")
	}
}

func TestStartVerificationRejectsWhileExtending(t *testing.T) {
	tutDir := t.TempDir()
	writeTut(t, tutDir, store.StatusExtending)

	if err := StartVerification("test-slug", tutDir); err == nil {
		t.Error("StartVerification() should reject a tutorial that is extending")
	}
}

func TestFinalizeVerifyFlipsStuckVerifyingToFailed(t *testing.T) {
	tutDir := t.TempDir()
	writeTut(t, tutDir, store.StatusVerifying)

	// Subprocess timed out without ever reporting a terminal status.
	finalizeVerify(tutDir, true, errors.New("signal: killed"))

	got, err := store.ReadMetadata(tutDir)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if got.Status != store.StatusFailed {
		t.Errorf("Status = %q, want %q", got.Status, store.StatusFailed)
	}

	vr, err := store.ReadVerifyResult(tutDir)
	if err != nil {
		t.Fatalf("ReadVerifyResult: %v", err)
	}
	if vr.Status != store.StatusFailed {
		t.Errorf("VerifyResult.Status = %q, want %q", vr.Status, store.StatusFailed)
	}
	if vr.Error == "" {
		t.Error("VerifyResult.Error should explain the timeout, got empty")
	}
}

func TestFinalizeVerifyLeavesReportedStatusAlone(t *testing.T) {
	tutDir := t.TempDir()
	// The skill already reported success before the goroutine ran.
	writeTut(t, tutDir, store.StatusVerified)

	finalizeVerify(tutDir, false, nil)

	got, err := store.ReadMetadata(tutDir)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if got.Status != store.StatusVerified {
		t.Errorf("Status = %q, want %q (must not override a reported status)", got.Status, store.StatusVerified)
	}
	// No fallback verify-result.json should be written when the skill reported.
	if _, err := os.Stat(filepath.Join(tutDir, "verify-result.json")); !os.IsNotExist(err) {
		t.Errorf("finalizeVerify wrote a verify-result.json over a reported status: %v", err)
	}
}
