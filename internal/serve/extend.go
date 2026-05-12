package serve

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/devenjarvis/lathe/internal/extend"
	"github.com/devenjarvis/lathe/internal/store"
)

const maxGuidanceBytes = 2 * 1024

func (s *Server) handleExtend(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	tutDir, ok := s.safeTutorialPath(slug)
	if !ok {
		http.NotFound(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxGuidanceBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "request body too large", http.StatusBadRequest)
		return
	}

	var payload struct {
		Guidance string `json:"guidance"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &payload); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
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

	if err := store.PromoteIndexToPart(tutDir); err != nil {
		http.Error(w, "promote failed", http.StatusInternalServerError)
		return
	}

	// SpawnExtender writes metadata to "extending" before attempting spawn.
	// A spawn failure leaves metadata committed; we return 202 regardless so
	// the UI can show the in-flight badge (the background goroutine flips to
	// "failed" if the subprocess exits badly).
	extend.SpawnExtender(slug, tutDir, payload.Guidance) //nolint:errcheck

	tut, err = store.ReadMetadata(tutDir)
	if err != nil || tut.Status != store.StatusExtending {
		http.Error(w, "extend failed", http.StatusInternalServerError)
		return
	}

	if tut.PendingPart != "" {
		w.Header().Set("Location", "/"+slug+"/"+tut.PendingPart)
	}
	w.WriteHeader(http.StatusAccepted)
}

