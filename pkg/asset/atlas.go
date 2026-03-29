package asset

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/anthonyrego/construct/pkg/renderer"
)

// AtlasEntry describes one building's placement within a texture atlas.
type AtlasEntry struct {
	BBL    string  `json:"bbl"`
	UOff   float32 `json:"uOff"`   // UV offset X in atlas [0,1]
	VOff   float32 `json:"vOff"`   // UV offset Y in atlas [0,1]
	UScale float32 `json:"uScale"` // UV scale X
	VScale float32 `json:"vScale"` // UV scale Y
}

// AtlasManifest describes a per-cell texture atlas.
type AtlasManifest struct {
	CellKey string       `json:"cellKey"`
	Width   int          `json:"width"`
	Height  int          `json:"height"`
	Entries []AtlasEntry `json:"entries"`
}

// LoadAtlasManifest reads an atlas manifest from disk.
func LoadAtlasManifest(path string) (*AtlasManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m AtlasManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse atlas manifest: %w", err)
	}
	return &m, nil
}

// SaveAtlasManifest writes an atlas manifest to disk.
func SaveAtlasManifest(path string, m *AtlasManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// PackAtlas packs building textures into a single atlas image.
// Uses simple row-based packing. The (0,0) pixel is reserved as white
// so untextured buildings (UV=0,0) sample white.
func PackAtlas(textures map[string]image.Image, atlasSize int) (*AtlasManifest, *image.NRGBA, error) {
	atlas := image.NewNRGBA(image.Rect(0, 0, atlasSize, atlasSize))

	// Fill with white (so UV=0,0 samples white for untextured buildings)
	draw.Draw(atlas, atlas.Bounds(), &image.Uniform{color.NRGBA{255, 255, 255, 255}}, image.Point{}, draw.Src)

	manifest := &AtlasManifest{
		Width:  atlasSize,
		Height: atlasSize,
	}

	// Simple row-based packing
	curX, curY, rowHeight := 1, 1, 0 // start at (1,1) to reserve (0,0) white pixel

	for bbl, tex := range textures {
		bounds := tex.Bounds()
		tw := bounds.Dx()
		th := bounds.Dy()

		// Move to next row if needed
		if curX+tw > atlasSize {
			curX = 0
			curY += rowHeight + 1 // 1px gap between rows
			rowHeight = 0
		}

		// Check if it fits vertically
		if curY+th > atlasSize {
			return nil, nil, fmt.Errorf("atlas overflow: %s doesn't fit in %dx%d", bbl, atlasSize, atlasSize)
		}

		// Copy texture into atlas
		dstRect := image.Rect(curX, curY, curX+tw, curY+th)
		draw.Draw(atlas, dstRect, tex, bounds.Min, draw.Src)

		// Record UV mapping
		manifest.Entries = append(manifest.Entries, AtlasEntry{
			BBL:    bbl,
			UOff:   float32(curX) / float32(atlasSize),
			VOff:   float32(curY) / float32(atlasSize),
			UScale: float32(tw) / float32(atlasSize),
			VScale: float32(th) / float32(atlasSize),
		})

		curX += tw + 1 // 1px gap between textures
		if th > rowHeight {
			rowHeight = th
		}
	}

	return manifest, atlas, nil
}

// SaveAtlasPNG writes the atlas image to disk.
func SaveAtlasPNG(path string, img *image.NRGBA) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// RemapUVs transforms vertex UVs from [0,1] range to an atlas sub-region.
func RemapUVs(vertices []renderer.LitVertex, entry AtlasEntry) {
	for i := range vertices {
		vertices[i].U = entry.UOff + vertices[i].U*entry.UScale
		vertices[i].V = entry.VOff + vertices[i].V*entry.VScale
	}
}
