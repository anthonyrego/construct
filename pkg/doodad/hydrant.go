package doodad

import (
	"fmt"

	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// HydrantConfig defines the hydrant doodad type.
var HydrantConfig = TypeConfig{
	BuildMesh:     buildHydrantMesh,
	BuildInstance: buildHydrantInstance,
	CullCenter:    hydrantCenter,
}

func buildHydrantInstance(d mapdata.DoodadItem) (Instance, bool) {
	if !d.Visible {
		return Instance{}, false
	}
	return Instance{
		ID: d.ID,
		X:  d.Position[0],
		Z:  d.Position[1],
	}, true
}

func hydrantCenter(_ Instance) (float32, float32) {
	return 0.4, 1.5
}

// buildHydrantMesh creates a body + bonnet unit mesh.
// Body: red box from Y=0 to Y=0.7, half-width 0.15
// Bonnet: darker/wider box from Y=0.7 to Y=0.85, half-width 0.2
func buildHydrantMesh(r *renderer.Renderer) (*mesh.Mesh, error) {
	const (
		bodyR, bodyG, bodyB uint8 = 180, 40, 30
		capR, capG, capB    uint8 = 140, 30, 25
	)

	var vertices []renderer.LitVertex
	var indices []uint16

	addBox := func(hw, yBot, yTop float32, cr, cg, cb uint8) {
		base := uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yBot, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: hw, NX: 0, NY: 0, NZ: 1, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: hw, Y: yBot, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yBot, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: -hw, NX: 0, NY: 0, NZ: -1, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: hw, Y: yBot, Z: hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: -hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: -hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: hw, NX: 1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yBot, Z: -hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yBot, Z: hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: -hw, NX: -1, NY: 0, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yTop, Z: hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yTop, Z: -hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yTop, Z: -hw, NX: 0, NY: 1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)

		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: -hw, Y: yBot, Z: -hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: -hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: hw, Y: yBot, Z: hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
			renderer.LitVertex{X: -hw, Y: yBot, Z: hw, NX: 0, NY: -1, NZ: 0, R: cr, G: cg, B: cb, A: 255},
		)
		indices = append(indices, base, base+1, base+2, base, base+2, base+3)
	}

	addBox(0.15, 0, 0.7, bodyR, bodyG, bodyB)
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
