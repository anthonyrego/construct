package tree

import (
	"fmt"
	"math"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// Tree represents a single street tree with computed visual dimensions.
type Tree struct {
	Position geojson.Point2D
	Height   float32 // meters, computed from diameter
	Spread   float32 // canopy width in meters, computed from diameter
}

// System holds all trees and the shared unit mesh.
type System struct {
	Trees []Tree
	Mesh  *mesh.Mesh
}

// NewFromMapData creates trees from map data doodad items.
func NewFromMapData(r *renderer.Renderer, items []mapdata.DoodadItem) (*System, error) {
	trees := make([]Tree, 0, len(items))
	for _, d := range items {
		if !d.Visible {
			continue
		}
		var diameter float32 = 10 // default DBH in inches
		if v, ok := d.Properties["diameter"]; ok {
			switch dv := v.(type) {
			case float64:
				diameter = float32(dv)
			case float32:
				diameter = dv
			}
		}
		if diameter < 1 {
			diameter = 1
		}

		height := float32(4.0 + float64(diameter)*0.25)
		spread := float32(1.5 + float64(diameter)*0.12)

		trees = append(trees, Tree{
			Position: geojson.Point2D{X: d.Position[0], Z: d.Position[1]},
			Height:   height,
			Spread:   spread,
		})
	}

	m, err := newMesh(r)
	if err != nil {
		return nil, err
	}
	return &System{Trees: trees, Mesh: m}, nil
}

// newMesh builds a combined trunk + canopy unit mesh.
// Trunk: brown box from Y=0 to Y=0.4, width 0.15
// Canopy: green 8-sided cone from Y=0.35 to Y=1.0, base radius 0.5
func newMesh(r *renderer.Renderer) (*mesh.Mesh, error) {
	const (
		trunkR, trunkG, trunkB uint8 = 80, 55, 30
		leafR, leafG, leafB   uint8 = 40, 75, 30
	)

	var vertices []renderer.LitVertex
	var indices []uint16

	// --- Trunk (box from Y=0 to Y=0.4, half-width 0.075) ---
	const hw = 0.075 // half-width
	const th = 0.4   // trunk height

	// Front face (Z+)
	base := uint16(len(vertices))
	vertices = append(vertices,
		renderer.LitVertex{X: -hw, Y: 0, Z: hw, NX: 0, NY: 0, NZ: 1, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: hw, Y: 0, Z: hw, NX: 0, NY: 0, NZ: 1, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: hw, Y: th, Z: hw, NX: 0, NY: 0, NZ: 1, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: -hw, Y: th, Z: hw, NX: 0, NY: 0, NZ: 1, R: trunkR, G: trunkG, B: trunkB, A: 255},
	)
	indices = append(indices, base, base+1, base+2, base, base+2, base+3)

	// Back face (Z-)
	base = uint16(len(vertices))
	vertices = append(vertices,
		renderer.LitVertex{X: hw, Y: 0, Z: -hw, NX: 0, NY: 0, NZ: -1, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: -hw, Y: 0, Z: -hw, NX: 0, NY: 0, NZ: -1, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: -hw, Y: th, Z: -hw, NX: 0, NY: 0, NZ: -1, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: hw, Y: th, Z: -hw, NX: 0, NY: 0, NZ: -1, R: trunkR, G: trunkG, B: trunkB, A: 255},
	)
	indices = append(indices, base, base+1, base+2, base, base+2, base+3)

	// Right face (X+)
	base = uint16(len(vertices))
	vertices = append(vertices,
		renderer.LitVertex{X: hw, Y: 0, Z: hw, NX: 1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: hw, Y: 0, Z: -hw, NX: 1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: hw, Y: th, Z: -hw, NX: 1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: hw, Y: th, Z: hw, NX: 1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
	)
	indices = append(indices, base, base+1, base+2, base, base+2, base+3)

	// Left face (X-)
	base = uint16(len(vertices))
	vertices = append(vertices,
		renderer.LitVertex{X: -hw, Y: 0, Z: -hw, NX: -1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: -hw, Y: 0, Z: hw, NX: -1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: -hw, Y: th, Z: hw, NX: -1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
		renderer.LitVertex{X: -hw, Y: th, Z: -hw, NX: -1, NY: 0, NZ: 0, R: trunkR, G: trunkG, B: trunkB, A: 255},
	)
	indices = append(indices, base, base+1, base+2, base, base+2, base+3)

	// --- Canopy (8-sided cone from Y=0.35 to Y=1.0, base radius 0.5) ---
	const (
		coneBase = 0.35
		coneTop  = 1.0
		coneRad  = 0.5
		sides    = 8
	)

	// Tip vertex index (shared by all side triangles)
	tipIdx := uint16(len(vertices))
	vertices = append(vertices, renderer.LitVertex{
		X: 0, Y: coneTop, Z: 0,
		NX: 0, NY: 1, NZ: 0,
		R: leafR, G: leafG, B: leafB, A: 255,
	})

	// Generate side faces — each triangle gets its own vertices for correct normals
	for i := 0; i < sides; i++ {
		a0 := 2 * math.Pi * float64(i) / sides
		a1 := 2 * math.Pi * float64(i+1) / sides

		x0 := float32(math.Sin(a0)) * coneRad
		z0 := float32(math.Cos(a0)) * coneRad
		x1 := float32(math.Sin(a1)) * coneRad
		z1 := float32(math.Cos(a1)) * coneRad

		// Face normal: average of the two edge directions crossed with up
		midA := (a0 + a1) / 2
		nx := float32(math.Sin(midA))
		nz := float32(math.Cos(midA))
		// Tilt normal upward for cone slope
		ny := float32(coneRad / (coneTop - coneBase))
		nLen := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
		nx /= nLen
		ny /= nLen
		nz /= nLen

		base = uint16(len(vertices))
		vertices = append(vertices,
			renderer.LitVertex{X: x0, Y: coneBase, Z: z0, NX: nx, NY: ny, NZ: nz, R: leafR, G: leafG, B: leafB, A: 255},
			renderer.LitVertex{X: x1, Y: coneBase, Z: z1, NX: nx, NY: ny, NZ: nz, R: leafR, G: leafG, B: leafB, A: 255},
			renderer.LitVertex{X: 0, Y: coneTop, Z: 0, NX: nx, NY: ny, NZ: nz, R: leafR, G: leafG, B: leafB, A: 255},
		)
		indices = append(indices, base, base+1, base+2)
	}
	// Remove the shared tip vertex since we're using per-face vertices
	_ = tipIdx

	// Bottom cap (octagon at Y=coneBase, normal pointing down)
	centerIdx := uint16(len(vertices))
	vertices = append(vertices, renderer.LitVertex{
		X: 0, Y: coneBase, Z: 0,
		NX: 0, NY: -1, NZ: 0,
		R: leafR, G: leafG, B: leafB, A: 255,
	})
	capStart := uint16(len(vertices))
	for i := 0; i < sides; i++ {
		a := 2 * math.Pi * float64(i) / sides
		x := float32(math.Sin(a)) * coneRad
		z := float32(math.Cos(a)) * coneRad
		vertices = append(vertices, renderer.LitVertex{
			X: x, Y: coneBase, Z: z,
			NX: 0, NY: -1, NZ: 0,
			R: leafR, G: leafG, B: leafB, A: 255,
		})
	}
	for i := 0; i < sides; i++ {
		next := (i + 1) % sides
		// Winding: center, next, current (CCW when viewed from below)
		indices = append(indices, centerIdx, capStart+uint16(next), capStart+uint16(i))
	}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, fmt.Errorf("tree vertex buffer: %w", err)
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, fmt.Errorf("tree index buffer: %w", err)
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
