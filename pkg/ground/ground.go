package ground

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

type SurfaceType int

const (
	Roadbed  SurfaceType = iota
	Sidewalk
	Park
)

var surfaceColors = [3][3]uint8{
	Roadbed:  {45, 42, 40},
	Sidewalk: {70, 65, 60},
	Park:     {35, 50, 35},
}

var surfaceYOffsets = [3]float32{
	Roadbed:  0.00,
	Sidewalk: 0.01,
	Park:     0.02,
}

// Flatten creates a flat lit mesh from a surface polygon at the appropriate
// Y-offset for the surface type. Mirrors the roof section of building.Extrude.
func Flatten(r *renderer.Renderer, poly geojson.SurfacePolygon, surfType SurfaceType) (*mesh.Mesh, mgl32.Vec3, error) {
	outer := poly.Rings[0]
	n := len(outer)
	if n < 3 {
		return nil, mgl32.Vec3{}, fmt.Errorf("surface polygon has fewer than 3 vertices")
	}

	// Compute centroid of outer ring
	var cx, cz float32
	for _, p := range outer {
		cx += p.X
		cz += p.Z
	}
	cx /= float32(n)
	cz /= float32(n)

	color := surfaceColors[surfType]
	red, green, blue := color[0], color[1], color[2]

	// Center vertices and create LitVertex entries at Y=0 with upward normal
	centered := make([]geojson.Point2D, n)
	vertices := make([]renderer.LitVertex, n)
	for i, p := range outer {
		centered[i] = geojson.Point2D{X: p.X - cx, Z: p.Z - cz}
		vertices[i] = renderer.LitVertex{
			X: centered[i].X, Y: 0, Z: centered[i].Z,
			NX: 0, NY: 1, NZ: 0,
			R: red, G: green, B: blue, A: 255,
		}
	}

	triIndices := building.Triangulate(centered)
	if len(triIndices) == 0 {
		return nil, mgl32.Vec3{}, fmt.Errorf("triangulation produced no triangles")
	}

	// Reverse winding for correct front-face from above
	indices := make([]uint16, len(triIndices))
	for i := 0; i < len(triIndices); i += 3 {
		indices[i] = triIndices[i]
		indices[i+1] = triIndices[i+2]
		indices[i+2] = triIndices[i+1]
	}

	if len(vertices) > 65535 {
		return nil, mgl32.Vec3{}, fmt.Errorf("surface exceeds uint16 vertex limit (%d vertices)", len(vertices))
	}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, mgl32.Vec3{}, fmt.Errorf("failed to create vertex buffer: %w", err)
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, mgl32.Vec3{}, fmt.Errorf("failed to create index buffer: %w", err)
	}

	m := &mesh.Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}

	yOffset := surfaceYOffsets[surfType]
	pos := mgl32.Vec3{cx, yOffset, cz}
	return m, pos, nil
}
