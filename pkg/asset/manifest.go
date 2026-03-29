package asset

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StageStatus tracks the state of a pipeline stage.
type StageStatus struct {
	Status  string    `json:"status"` // pending, running, complete, failed
	Updated time.Time `json:"updated,omitempty"`
	Error   string    `json:"error,omitempty"`
}

// Manifest tracks the pipeline state for a single building asset.
type Manifest struct {
	BBL     string                  `json:"bbl"`
	Version int                     `json:"version"`
	Stages  map[string]StageStatus  `json:"stages"`
}

// NewManifest creates a manifest with all stages set to pending.
func NewManifest(bbl string) *Manifest {
	return &Manifest{
		BBL:     bbl,
		Version: 1,
		Stages: map[string]StageStatus{
			"reference": {Status: "pending"},
			"sculpt":    {Status: "pending"},
			"uv":        {Status: "pending"},
			"texture":   {Status: "pending"},
			"bake":      {Status: "pending"},
		},
	}
}

// IsStageComplete returns true if the named stage has status "complete".
func (m *Manifest) IsStageComplete(stage string) bool {
	s, ok := m.Stages[stage]
	return ok && s.Status == "complete"
}

// SetStage updates a stage's status and timestamp.
func (m *Manifest) SetStage(stage, status string) {
	m.Stages[stage] = StageStatus{
		Status:  status,
		Updated: time.Now(),
	}
}

// LoadManifest reads a manifest from the given asset directory.
func LoadManifest(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return &m, nil
}

// SaveManifest writes a manifest to the given asset directory.
func SaveManifest(dir string, m *Manifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644)
}

// AssetDir returns the asset directory path for a building BBL.
func AssetDir(baseDir, bbl string) string {
	return filepath.Join(baseDir, "buildings", bbl)
}
