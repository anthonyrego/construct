package pipeline

import (
	"fmt"
	"os"

	"github.com/anthonyrego/construct/pkg/asset"
	"github.com/anthonyrego/construct/pkg/mapdata"
)

// Stage defines a single step in the asset pipeline.
type Stage interface {
	Name() string
	Run(ctx StageContext) error
}

// StageContext provides data to a pipeline stage.
type StageContext struct {
	BBL      string
	AssetDir string
	Manifest *asset.Manifest
	Building *mapdata.BuildingData
}

// Runner executes pipeline stages for buildings.
type Runner struct {
	Stages   []Stage
	AssetDir string // base directory (e.g., "data/assets")
	Store    *mapdata.Store
}

// NewRunner creates a pipeline runner with the standard stages.
func NewRunner(assetDir string, store *mapdata.Store) *Runner {
	return &Runner{
		Stages: []Stage{
			&stubStage{name: "reference"},
			&sculptStage{scadLibDir: "data/scad"},
			&stubStage{name: "uv"},
			&stubStage{name: "texture"},
			&bakeStage{},
		},
		AssetDir: assetDir,
		Store:    store,
	}
}

// RunBuilding processes a single building through all pending stages.
func (r *Runner) RunBuilding(bbl string) error {
	dir := asset.AssetDir(r.AssetDir, bbl)

	// Load or create manifest
	m, err := asset.LoadManifest(dir)
	if err != nil {
		m = asset.NewManifest(bbl)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create asset dir: %w", err)
		}
	}

	// Find building data
	var building *mapdata.BuildingData
	if r.Store != nil {
		bd, _ := r.Store.FindBuildingByBBL(bbl)
		building = bd
	}

	ctx := StageContext{
		BBL:      bbl,
		AssetDir: dir,
		Manifest: m,
		Building: building,
	}

	for _, stage := range r.Stages {
		if m.IsStageComplete(stage.Name()) {
			fmt.Printf("  [%s] already complete, skipping\n", stage.Name())
			continue
		}

		fmt.Printf("  [%s] running...\n", stage.Name())
		m.SetStage(stage.Name(), "running")
		if err := asset.SaveManifest(dir, m); err != nil {
			return fmt.Errorf("save manifest: %w", err)
		}

		if err := stage.Run(ctx); err != nil {
			m.Stages[stage.Name()] = asset.StageStatus{
				Status: "failed",
				Error:  err.Error(),
			}
			asset.SaveManifest(dir, m)
			return fmt.Errorf("stage %s: %w", stage.Name(), err)
		}

		m.SetStage(stage.Name(), "complete")
		if err := asset.SaveManifest(dir, m); err != nil {
			return fmt.Errorf("save manifest: %w", err)
		}
		fmt.Printf("  [%s] complete\n", stage.Name())
	}

	return nil
}

// RunStage runs a specific stage for a building (even if already complete).
func (r *Runner) RunStage(bbl, stageName string) error {
	dir := asset.AssetDir(r.AssetDir, bbl)

	m, err := asset.LoadManifest(dir)
	if err != nil {
		m = asset.NewManifest(bbl)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create asset dir: %w", err)
		}
	}

	var building *mapdata.BuildingData
	if r.Store != nil {
		bd, _ := r.Store.FindBuildingByBBL(bbl)
		building = bd
	}

	ctx := StageContext{
		BBL:      bbl,
		AssetDir: dir,
		Manifest: m,
		Building: building,
	}

	for _, stage := range r.Stages {
		if stage.Name() != stageName {
			continue
		}
		fmt.Printf("  [%s] running...\n", stage.Name())
		if err := stage.Run(ctx); err != nil {
			return fmt.Errorf("stage %s: %w", stage.Name(), err)
		}
		m.SetStage(stage.Name(), "complete")
		return asset.SaveManifest(dir, m)
	}

	return fmt.Errorf("unknown stage: %s", stageName)
}

// stubStage is a placeholder for AI-driven stages.
type stubStage struct {
	name string
}

func (s *stubStage) Name() string { return s.name }
func (s *stubStage) Run(ctx StageContext) error {
	fmt.Printf("    stage '%s' is a stub — AI integration not yet implemented\n", s.name)
	return nil
}

// bakeStage validates that mesh and texture files exist.
type bakeStage struct{}

func (s *bakeStage) Name() string { return "bake" }
func (s *bakeStage) Run(ctx StageContext) error {
	// Check that mesh file exists
	meshPath := ctx.AssetDir + "/mesh.bin"
	if _, err := os.Stat(meshPath); err != nil {
		return fmt.Errorf("mesh file not found: %s", meshPath)
	}

	// Check that texture file exists
	texPath := ctx.AssetDir + "/texture.png"
	if _, err := os.Stat(texPath); err != nil {
		return fmt.Errorf("texture file not found: %s", texPath)
	}

	// Validate mesh is loadable
	verts, indices, err := asset.LoadMesh(ctx.AssetDir)
	if err != nil {
		return fmt.Errorf("invalid mesh: %w", err)
	}

	fmt.Printf("    bake: %d vertices, %d indices\n", len(verts), len(indices))
	return nil
}
