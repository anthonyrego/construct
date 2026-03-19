package hydrant

import (
	"fmt"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// Hydrant represents a single fire hydrant.
type Hydrant struct {
	Position geojson.Point2D
}

// System holds all hydrants and the shared mesh.
type System struct {
	Hydrants []Hydrant
	Mesh     *mesh.Mesh
}

// NewFromMapData creates hydrants from map data doodad items.
func NewFromMapData(r *renderer.Renderer, items []mapdata.DoodadItem) (*System, error) {
	hydrants := make([]Hydrant, 0, len(items))
	for _, d := range items {
		if !d.Visible {
			continue
		}
		hydrants = append(hydrants, Hydrant{
			Position: geojson.Point2D{X: d.Position[0], Z: d.Position[1]},
		})
	}

	m, err := newMesh(r)
	if err != nil {
		return nil, err
	}
	return &System{Hydrants: hydrants, Mesh: m}, nil
}

// newMesh builds a combined body + bonnet unit mesh at real-world scale.
// Body: red box from Y=0 to Y=0.7, half-width 0.15
// Bonnet: darker/wider box from Y=0.7 to Y=0.85, half-width 0.2
func newMesh(r *renderer.Renderer) (*mesh.Mesh, error) {
	const (
		bodyR, bodyG, bodyB    uint8 = 180, 40, 30
		capR, capG, capB       uint8 = 140, 30, 25
	)

	var vertices []renderer.LitVertex
	var indices []uint16

	// Helper to add a box (6 faces) with given bounds and color
	addBox := func(hw, yBot, yTop float32, cr, cg, cb uint8) {
		// Front face (Z+)
		base := uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yBot, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		// Back face (Z-)
		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: hw, Y: yBot, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yBot, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		// Right face (X+)
		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: hw, Y: yBot, Z: hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: -hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: -hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		// Left face (X-)
		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yBot, Z: -hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yBot, Z: hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: -hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		// Top face (Y+)
		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yTop, Z: hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: -hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: -hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		// Bottom face (Y-)
		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yBot, Z: -hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: -hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yBot, Z: hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)
	}

	// Body: Y=0 to Y=0.7, half-width 0.15
	addBox(0.15, 0, 0.7, bodyR, bodyG, bodyB)
	// Bonnet/cap: Y=0.7 to Y=0.85, half-width 0.2
	addBox(0.2, 0.7, 0.85, capR, capG, capB)

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, fmt.Errorf("hydrant vertex buffer: %w", err)
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, fmt.Errorf("hydrant index buffer: %w", err)
	}

	return &mesh.Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, nil
}

// Destroy releases the shared mesh GPU buffers.
func (s *System) Destroy(r *renderer.Renderer) {
	if s != nil && s.Mesh != nil {
		s.Mesh.Destroy(r)
	}
}
