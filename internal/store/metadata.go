package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type Status string

const (
	StatusVerifying Status = "verifying"
	StatusVerified  Status = "verified"
	StatusFailed    Status = "failed"
	StatusExtending Status = "extending"
)

type Tutorial struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Topic       string    `json:"topic"`
	Created     time.Time `json:"created"`
	Status      Status    `json:"status"`
	Parts       []string  `json:"parts,omitempty"`
	PendingPart string    `json:"pending_part,omitempty"`
}

func (t *Tutorial) IsSeries() bool {
	return len(t.Parts) > 1
}

type VerifyResult struct {
	Status     Status `json:"status"`
	Part       string `json:"part,omitempty"`
	FailedStep int    `json:"failed_step,omitempty"`
	Error      string `json:"error,omitempty"`
	CheckedAt  string `json:"checked_at,omitempty"`
}

func ReadMetadata(tutorialDir string) (*Tutorial, error) {
	data, err := os.ReadFile(filepath.Join(tutorialDir, "metadata.json"))
	if err != nil {
		return nil, err
	}
	var t Tutorial
	return &t, json.Unmarshal(data, &t)
}

func WriteMetadata(tutorialDir string, t *Tutorial) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tutorialDir, "metadata.json"), data, 0644)
}
