package scene

import "math"

const defaultCellSize = 100.0 // meters

// SpatialGrid partitions scene objects into a 2D grid on the X,Z plane
// for fast spatial queries.
type SpatialGrid struct {
	CellSize float32
	cells    map[uint64][]int // cell key → object indices
}

// NewSpatialGrid creates a spatial grid and indexes the given objects.
func NewSpatialGrid(objects []Object, cellSize float32) *SpatialGrid {
	if cellSize <= 0 {
		cellSize = defaultCellSize
	}
	g := &SpatialGrid{
		CellSize: cellSize,
		cells:    make(map[uint64][]int, len(objects)/4),
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
			key := uint64(uint32(ix))<<32 | uint64(uint32(iz))
			if indices, ok := g.cells[key]; ok {
				result = append(result, indices...)
			}
		}
	}
	return result
}
