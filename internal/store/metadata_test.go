package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devenjarvis/lathe/internal/store"
)

func TestWriteReadMetadata(t *testing.T) {
	dir := t.TempDir()
	original := &store.Tutorial{
		Slug:    "test-tutorial",
		Title:   "Test Tutorial",
		Topic:   "test tutorial",
		Created: time.Date(2026, 5, 3, 0, 0, 0, 0, time.UTC),
		Status:  store.StatusVerified,
	}

	if err := store.WriteMetadata(dir, original); err != nil {
		t.Fatalf("WriteMetadata() error = %v", err)
	}

	got, err := store.ReadMetadata(dir)
	if err != nil {
		t.Fatalf("ReadMetadata() error = %v", err)
	}
	if got.Slug != original.Slug {
		t.Errorf("Slug = %q, want %q", got.Slug, original.Slug)
	}
	if got.Status != original.Status {
		t.Errorf("Status = %q, want %q", got.Status, original.Status)
	}
}

func TestReadMetadataNotFound(t *testing.T) {
	_, err := store.ReadMetadata("/nonexistent/path/abc123")
	if err == nil {
		t.Error("ReadMetadata() expected error for missing file, got nil")
	}
}

func TestTutorialIsSeries(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
		want  bool
	}{
		{"zero parts", nil, false},
		{"one part", []string{"part-01.md"}, false},
		{"two parts", []string{"part-01.md", "part-02.md"}, true},
		{"three parts", []string{"part-01.md", "part-02.md", "part-03.md"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tut := &store.Tutorial{Parts: tc.parts}
			if got := tut.IsSeries(); got != tc.want {
				t.Errorf("IsSeries() = %v, want %v (parts=%v)", got, tc.want, tc.parts)
			}
		})
	}
}

func TestMetadataRoundTripPendingPart(t *testing.T) {
	dir := t.TempDir()
	tut := &store.Tutorial{
		Slug:        "test-tut",
		Title:       "Test Tutorial",
		Status:      store.StatusExtending,
		Parts:       []string{"part-01.md", "part-02.md", "part-03.md"},
		PendingPart: "part-04.md",
	}
	if err := store.WriteMetadata(dir, tut); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	got, err := store.ReadMetadata(dir)
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if got.PendingPart != "part-04.md" {
		t.Errorf("PendingPart = %q, want %q", got.PendingPart, "part-04.md")
	}
	if got.Status != store.StatusExtending {
		t.Errorf("Status = %q, want %q", got.Status, store.StatusExtending)
	}
}

func TestStatusExtendingValue(t *testing.T) {
	if store.StatusExtending != "extending" {
		t.Errorf("StatusExtending = %q, want %q", store.StatusExtending, "extending")
	}
}

func TestPendingPartOmittedWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	tut := &store.Tutorial{
		Slug:   "test-tut",
		Status: store.StatusVerified,
	}
	if err := store.WriteMetadata(dir, tut); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "metadata.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(data), "pending_part") {
		t.Error("pending_part should be omitted from JSON when empty")
	}
}
