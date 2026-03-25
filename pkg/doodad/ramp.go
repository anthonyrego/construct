package doodad

import (
	"fmt"
	"math"

	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// RampCurbHeight is used by the ramp mesh builder. Set before calling New.
var RampCurbHeight float32 = 0.14

// RampConfig defines the ramp doodad type.
var RampConfig = TypeConfig{
	BuildMesh:     buildRampMesh,
	BuildInstance: buildRampInstance,
	CullCenter:    rampCenter,
}

func buildRampInstance(d mapdata.DoodadItem) (Instance, bool) {
	if !d.Visible {
		return Instance{}, false
	}
	return Instance{
		ID:       d.ID,
		X:        d.Position[0],
		Z:        d.Position[1],
		ScaleX:   d.Width,
		ScaleY:   1,
		ScaleZ:   d.Length,
		Rotation: d.AngleDeg * math.Pi / 180,
		TransY:   0.07,
	}, true
}

func rampCenter(_ Instance) (float32, float32) {
	return 0.1, 2
}

// buildRampMesh creates a unit ramp mesh with flanking flare wings.
func buildRampMesh(r *renderer.Renderer) (*mesh.Mesh, error) {
	ch := RampCurbHeight

	ny := float32(1.0)
	nz := ch
	nLen := float32(math.Sqrt(float64(ny*ny + nz*nz)))
	ny /= nLen
	nz /= nLen

	const cr, cg, cb uint8 = 70, 65, 60
	wr, wg, wb := uint8(int(cr)*7/10), uint8(int(cg)*7/10), uint8(int(cb)*7/10)

	vertices := []renderer.LitVertex{
		// Ramp surface (B, C, F, E)
		{X: -0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: 0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: 0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: -0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		// Left flare (A, E, B)
		{X: -1.0, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: -0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: -0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		// Right flare (C, F, D)
		{X: 0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: 0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		{X: 1.0, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},
		// Left outer wall (A, A-bottom, E)
		{X: -1.0, Y: ch, Z: 0, NX: -1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},
		{X: -1.0, Y: 0, Z: 0, NX: -1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},
		{X: -0.5, Y: 0, Z: 1, NX: -1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},
		// Right outer wall (D, F, D-bottom)
		{X: 1.0, Y: ch, Z: 0, NX: 1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},
		{X: 0.5, Y: 0, Z: 1, NX: 1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},
		{X: 1.0, Y: 0, Z: 0, NX: 1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},
	}

	indices := []uint16{
		0, 2, 1, 0, 3, 2, // ramp surface
		4, 5, 6, // left flare
		7, 8, 9, // right flare
		10, 11, 12, // left wall
		13, 14, 15, // right wall
	}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, fmt.Errorf("ramp vertex buffer: %w", err)
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, fmt.Errorf("ramp index buffer: %w", err)
	}

	return &mesh.Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, nil
}
