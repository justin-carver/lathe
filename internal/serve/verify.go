package serve

import (
	"net/http"

	"github.com/devenjarvis/lathe/internal/store"
	"github.com/devenjarvis/lathe/internal/verify"
)

// handleVerify triggers on-demand verification for a tutorial. Mirrors
// handleExtend/handleDelete: safeTutorialPath guards traversal, the conflict
// case returns 409, and a 202 is returned once status is committed to
// verifying (verify.StartVerification writes it before spawning).
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	tutDir, ok := s.safeTutorialPath(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}

	tut, err := store.ReadMetadata(tutDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if tut.Status == store.StatusExtending || tut.Status == store.StatusVerifying {
		http.Error(w, "conflict: tutorial is already extending or verifying", http.StatusConflict)
		return
	}

	// StartVerification commits status=verifying before spawning. A spawn
	// failure leaves metadata committed; we return 202 regardless so the UI
	// can show the in-flight badge (the background goroutine flips to failed
	// if the subprocess exits badly).
	verify.StartVerification(slug, tutDir) //nolint:errcheck

	tut, err = store.ReadMetadata(tutDir)
	if err != nil || tut.Status != store.StatusVerifying {
		http.Error(w, "verify failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
