package building

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// BuildingID is a unique identifier for a building within the registry.
type BuildingID uint32

// Building holds all data for a single building throughout its lifecycle.
type Building struct {
	ID        BuildingID
	BBL       string
	PLUTO     geojson.PLUTOData
	Footprint geojson.Footprint
	Color     Color
	Raw       *RawMesh
	Mesh      *mesh.Mesh
	Position  mgl32.Vec3
	Radius    float32
	CellKey   uint64
	Hidden    bool
}

// MergedCell holds a merged GPU mesh for a grid cell with per-building span tracking.
type MergedCell struct {
	Mesh  *mesh.Mesh
	CellX int32
	CellZ int32
	Spans []BuildingSpan
}

// Registry is the single owner of all building data and GPU resources.
type Registry struct {
	buildings []Building
	byBBL    map[string]BuildingID
	cells    map[uint64]*MergedCell
	rend     *renderer.Renderer
	cellSize float32
}

// NewRegistry creates an empty registry.
func NewRegistry(rend *renderer.Renderer, cellSize float32) *Registry {
	return &Registry{
		byBBL:    make(map[string]BuildingID),
		cells:    make(map[uint64]*MergedCell),
		rend:     rend,
		cellSize: cellSize,
	}
}

// Ingest processes enriched footprints: extrude, upload, register.
// Returns count of successfully processed buildings.
func (reg *Registry) Ingest(footprints []geojson.Footprint) int {
	count := 0
	for _, fp := range footprints {
		c := StyleColor(fp.PLUTO)
		raw, err := ExtrudeRaw(fp, c.R, c.G, c.B)
		if err != nil {
			continue
		}

		m, err := UploadMesh(reg.rend, raw)
		if err != nil {
			continue
		}

		id := BuildingID(len(reg.buildings))

		cx := int32(math.Floor(float64(raw.Position.X() / reg.cellSize)))
		cz := int32(math.Floor(float64(raw.Position.Z() / reg.cellSize)))
		cellKey := uint64(uint32(cx))<<32 | uint64(uint32(cz))

		reg.buildings = append(reg.buildings, Building{
			ID:        id,
			BBL:       fp.BBL,
			PLUTO:     fp.PLUTO,
			Footprint: fp,
			Color:     c,
			Raw:       raw,
			Mesh:      m,
			Position:  raw.Position,
			Radius:    raw.Radius,
			CellKey:   cellKey,
		})

		if fp.BBL != "" {
			reg.byBBL[fp.BBL] = id
		}

		count++
	}
	return count
}

// BuildCellMeshes merges per-cell raw meshes with span tracking.
// Returns the cell map for grid integration.
func (reg *Registry) BuildCellMeshes() map[uint64]*MergedCell {
	// Group visible buildings by cell
	cellBuildings := make(map[uint64][]int) // cellKey → building indices
	for i := range reg.buildings {
		if reg.buildings[i].Hidden {
			continue
		}
		key := reg.buildings[i].CellKey
		cellBuildings[key] = append(cellBuildings[key], i)
	}

	for key, indices := range cellBuildings {
		entries := make([]MergeEntry, len(indices))
		for j, idx := range indices {
			entries[j] = MergeEntry{
				ID:  reg.buildings[idx].ID,
				Raw: reg.buildings[idx].Raw,
			}
		}

		merged, spans, err := MergeMeshesWithSpans(reg.rend, entries)
		if err != nil {
			continue
		}

		cx := int32(uint32(key >> 32))
		cz := int32(uint32(key))
		reg.cells[key] = &MergedCell{
			Mesh:  merged,
			CellX: cx,
			CellZ: cz,
			Spans: spans,
		}
	}

	return reg.cells
}

// Get returns a building by ID.
func (reg *Registry) Get(id BuildingID) *Building {
	if int(id) >= len(reg.buildings) {
		return nil
	}
	return &reg.buildings[id]
}

// Lookup returns a building ID by BBL.
func (reg *Registry) Lookup(bbl string) (BuildingID, bool) {
	id, ok := reg.byBBL[bbl]
	return id, ok
}

// Count returns the total number of buildings.
func (reg *Registry) Count() int {
	return len(reg.buildings)
}

// Buildings returns the slice for iteration (read-only intent).
func (reg *Registry) Buildings() []Building {
	return reg.buildings
}

// ReplaceMesh releases the old GPU mesh, uploads a new one,
// and marks the cell for re-merge.
func (reg *Registry) ReplaceMesh(id BuildingID, raw *RawMesh) error {
	if int(id) >= len(reg.buildings) {
		return fmt.Errorf("building ID %d out of range", id)
	}
	b := &reg.buildings[id]

	newMesh, err := UploadMesh(reg.rend, raw)
	if err != nil {
		return fmt.Errorf("failed to upload replacement mesh: %w", err)
	}

	b.Mesh.Destroy(reg.rend)
	b.Raw = raw
	b.Mesh = newMesh
	b.Position = raw.Position
	b.Radius = raw.Radius

	return nil
}

// RebuildCellMesh re-merges a single cell after a building change.
// Returns the new MergedCell (or nil if no visible buildings remain).
func (reg *Registry) RebuildCellMesh(cellKey uint64) (*MergedCell, error) {
	// Collect visible buildings in this cell
	var entries []MergeEntry
	for i := range reg.buildings {
		if reg.buildings[i].CellKey == cellKey && !reg.buildings[i].Hidden {
			entries = append(entries, MergeEntry{
				ID:  reg.buildings[i].ID,
				Raw: reg.buildings[i].Raw,
			})
		}
	}

	// Release old merged mesh if it exists
	if old, ok := reg.cells[cellKey]; ok {
		old.Mesh.Destroy(reg.rend)
		delete(reg.cells, cellKey)
	}

	if len(entries) == 0 {
		return nil, nil
	}

	merged, spans, err := MergeMeshesWithSpans(reg.rend, entries)
	if err != nil {
		return nil, fmt.Errorf("failed to rebuild cell mesh: %w", err)
	}

	cx := int32(uint32(cellKey >> 32))
	cz := int32(uint32(cellKey))
	cell := &MergedCell{
		Mesh:  merged,
		CellX: cx,
		CellZ: cz,
		Spans: spans,
	}
	reg.cells[cellKey] = cell

	return cell, nil
}

// Destroy releases all GPU resources (individual + merged meshes).
func (reg *Registry) Destroy() {
	for i := range reg.buildings {
		if reg.buildings[i].Mesh != nil {
			reg.buildings[i].Mesh.Destroy(reg.rend)
		}
	}
	for _, cell := range reg.cells {
		if cell.Mesh != nil {
			cell.Mesh.Destroy(reg.rend)
		}
	}
}
