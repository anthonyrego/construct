package scene

import (
	"math"

	"github.com/anthonyrego/construct/pkg/mesh"
)

const defaultCellSize = 100.0 // meters

// CellMesh is a merged mesh for all buildings in a grid cell.
type CellMesh struct {
	Mesh   *mesh.Mesh
	CellX  int32
	CellZ  int32
}

// SpatialGrid partitions scene objects into a 2D grid on the X,Z plane
// for fast spatial queries.
type SpatialGrid struct {
	CellSize    float32
	cells       map[uint64][]int // cell key → object indices
	CellMeshes  map[uint64]*CellMesh // merged meshes per cell
}

// NewSpatialGrid creates a spatial grid and indexes the given objects.
func NewSpatialGrid(objects []Object, cellSize float32) *SpatialGrid {
	if cellSize <= 0 {
		cellSize = defaultCellSize
	}
	g := &SpatialGrid{
		CellSize:   cellSize,
		cells:      make(map[uint64][]int, len(objects)/4),
		CellMeshes: make(map[uint64]*CellMesh),
	}
	for i, obj := range objects {
		key := g.cellKey(obj.Position.X(), obj.Position.Z())
		g.cells[key] = append(g.cells[key], i)
	}
	return g
}

func (g *SpatialGrid) cellKey(x, z float32) uint64 {
	cx := int32(math.Floor(float64(x / g.CellSize)))
	cz := int32(math.Floor(float64(z / g.CellSize)))
	return uint64(uint32(cx))<<32 | uint64(uint32(cz))
}

func cellKeyFromInts(cx, cz int32) uint64 {
	return uint64(uint32(cx))<<32 | uint64(uint32(cz))
}

// QueryRadius returns indices of objects in cells that overlap a circle
// of the given radius centered at (cx, cz). The caller should further
// cull these with frustum checks.
func (g *SpatialGrid) QueryRadius(cx, cz, radius float32) []int {
	r := radius + g.CellSize // pad by one cell to avoid edge misses
	minCX := int32(math.Floor(float64((cx - r) / g.CellSize)))
	maxCX := int32(math.Floor(float64((cx + r) / g.CellSize)))
	minCZ := int32(math.Floor(float64((cz - r) / g.CellSize)))
	maxCZ := int32(math.Floor(float64((cz + r) / g.CellSize)))

	var result []int
	for ix := minCX; ix <= maxCX; ix++ {
		for iz := minCZ; iz <= maxCZ; iz++ {
			key := cellKeyFromInts(ix, iz)
			if indices, ok := g.cells[key]; ok {
				result = append(result, indices...)
			}
		}
	}
	return result
}

// QueryCells returns cell keys and their center positions for cells
// within the given radius. Used for merged mesh rendering.
func (g *SpatialGrid) QueryCells(cx, cz, radius float32) []uint64 {
	r := radius + g.CellSize
	minCX := int32(math.Floor(float64((cx - r) / g.CellSize)))
	maxCX := int32(math.Floor(float64((cx + r) / g.CellSize)))
	minCZ := int32(math.Floor(float64((cz - r) / g.CellSize)))
	maxCZ := int32(math.Floor(float64((cz + r) / g.CellSize)))

	var result []uint64
	for ix := minCX; ix <= maxCX; ix++ {
		for iz := minCZ; iz <= maxCZ; iz++ {
			key := cellKeyFromInts(ix, iz)
			if _, ok := g.CellMeshes[key]; ok {
				result = append(result, key)
			}
		}
	}
	return result
}

// CellCenter returns the world-space center of a cell.
func (g *SpatialGrid) CellCenter(key uint64) (float32, float32) {
	cx := int32(uint32(key >> 32))
	cz := int32(uint32(key))
	x := (float32(cx) + 0.5) * g.CellSize
	z := (float32(cz) + 0.5) * g.CellSize
	return x, z
}

// CellDistSq returns the squared distance from a point to the nearest
// edge of a cell (0 if inside the cell).
func (g *SpatialGrid) CellDistSq(key uint64, px, pz float32) float32 {
	cx := int32(uint32(key >> 32))
	cz := int32(uint32(key))
	minX := float32(cx) * g.CellSize
	maxX := minX + g.CellSize
	minZ := float32(cz) * g.CellSize
	maxZ := minZ + g.CellSize

	var dx, dz float32
	if px < minX {
		dx = minX - px
	} else if px > maxX {
		dx = px - maxX
	}
	if pz < minZ {
		dz = minZ - pz
	} else if pz > maxZ {
		dz = pz - maxZ
	}
	return dx*dx + dz*dz
}
