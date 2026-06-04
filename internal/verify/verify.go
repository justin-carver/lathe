package verify

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/devenjarvis/lathe/internal/store"
)

//go:embed skills/lathe-verify.md
var verifySkillContent string

// verifyTimeout bounds a verification run. It is generous on purpose: a
// first-time run may download and install a toolchain before it can build.
const verifyTimeout = 20 * time.Minute

// StartVerification is the single entry point all three triggers (the CLI
// `lathe verify`, the `--verify` store flag, and the web button) funnel
// through. It conflict-guards against an in-flight extend/verify, commits
// status=verifying to metadata before spawning, then launches the detached
// verifier subprocess. Mirrors extend.SpawnExtender's status-before-spawn
// contract so the UI can show the in-flight badge even if spawn fails.
func StartVerification(slug, tutorialDir string) error {
	tut, err := store.ReadMetadata(tutorialDir)
	if err != nil {
		return fmt.Errorf("read metadata: %w", err)
	}

	if tut.Status == store.StatusExtending || tut.Status == store.StatusVerifying {
		return fmt.Errorf("already extending or verifying: status is %q", tut.Status)
	}

	tut.Status = store.StatusVerifying
	if err := store.WriteMetadata(tutorialDir, tut); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}

	return SpawnVerifier(slug, tutorialDir)
}

func SpawnVerifier(slug, tutorialDir string) error {
	tempDir, err := os.MkdirTemp("", "lathe-verify-"+slug+"-")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}

	// Write the embedded skill into the temp dir so claude can discover it
	skillDir := filepath.Join(tempDir, ".claude", "skills", "lathe-verify")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("create skill dir: %w", err)
	}
	skillPath := filepath.Join(skillDir, "lathe-verify.md")
	if err := os.WriteFile(skillPath, []byte(verifySkillContent), 0644); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("write skill: %w", err)
	}

	// Capture the subprocess's stdout+stderr in the tutorial dir so a failed
	// run leaves a legible trail. Best-effort: a missing log must not block.
	logFile, _ := os.Create(filepath.Join(tutorialDir, "verify.log"))

	prompt := fmt.Sprintf(
		"Use the /lathe-verify skill to verify the tutorial. "+
			"LATHE_TUTORIAL_DIR is set to %q.",
		tutorialDir,
	)

	ctx, cancel := context.WithTimeout(context.Background(), verifyTimeout)

	// cmd.Dir pins the working directory to the temp dir so every file the
	// verifier creates lands there, not in the user's repo. --add-dir grants
	// access only; it does not change the cwd. The tutorial dir is granted too
	// so the subprocess can read metadata.json and write its result + status.
	cmd := exec.CommandContext(ctx, "claude",
		"--add-dir", tempDir,
		"--add-dir", tutorialDir,
		"--dangerously-skip-permissions",
		"-p", prompt,
	)
	cmd.Dir = tempDir
	cmd.Env = append(os.Environ(), "LATHE_TUTORIAL_DIR="+tutorialDir)
	if logFile != nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		cancel()
		if logFile != nil {
			logFile.Close()
		}
		os.RemoveAll(tempDir)
		return fmt.Errorf("start verifier: %w", err)
	}

	// Detach: clean up temp dir when subprocess exits, don't block. The
	// goroutine also enforces the fallback so the badge never sticks at ⏳.
	go func() {
		defer cancel()
		defer os.RemoveAll(tempDir)
		if logFile != nil {
			defer logFile.Close()
		}
		waitErr := cmd.Wait()
		timedOut := ctx.Err() == context.DeadlineExceeded
		finalizeVerify(tutorialDir, timedOut, waitErr)
	}()

	return nil
}

// finalizeVerify is the post-Wait fallback. The verifier skill is responsible
// for writing the terminal status (verified/failed/skipped). If it never did —
// the subprocess crashed, timed out, or exited non-zero before reporting — the
// status is still "verifying", so we flip it to failed with a clear reason.
// Pure and side-effect-scoped to the tutorial dir, so it's unit-testable
// without a real subprocess.
func finalizeVerify(tutorialDir string, timedOut bool, waitErr error) {
	tut, err := store.ReadMetadata(tutorialDir)
	if err != nil {
		return
	}
	// The skill already reported a terminal status — nothing to override.
	if tut.Status != store.StatusVerifying {
		return
	}

	var reason string
	switch {
	case timedOut:
		reason = fmt.Sprintf("verification timed out after %s", verifyTimeout)
	case waitErr != nil:
		reason = fmt.Sprintf("verifier exited without reporting: %v", waitErr)
	default:
		reason = "verifier exited without reporting a result"
	}

	tut.Status = store.StatusFailed
	store.WriteMetadata(tutorialDir, tut)                     //nolint:errcheck
	store.WriteVerifyResult(tutorialDir, &store.VerifyResult{ //nolint:errcheck
		Status: store.StatusFailed,
		Error:  reason,
	})
}
