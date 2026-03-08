package building

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// Extrude creates a lit mesh from a 2D footprint polygon + height.
// Vertices are relative to the footprint centroid.
// Returns the mesh and centroid position (for scene.Object.Position).
func Extrude(r *renderer.Renderer, fp geojson.Footprint, red, green, blue uint8) (*mesh.Mesh, mgl32.Vec3, error) {
	outer := fp.Rings[0]
	n := len(outer)
	if n < 3 {
		return nil, mgl32.Vec3{}, fmt.Errorf("footprint has fewer than 3 vertices")
	}

	// Compute centroid of outer ring
	var cx, cz float32
	for _, p := range outer {
		cx += p.X
		cz += p.Z
	}
	cx /= float32(n)
	cz /= float32(n)

	height := fp.Height

	// Pre-allocate: walls = 4*n verts, 6*n indices; roof = n verts, 3*(n-2) indices
	vertices := make([]renderer.LitVertex, 0, 5*n)
	indices := make([]uint16, 0, 9*n-6)

	// --- Walls ---
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a := outer[i]
		b := outer[j]

		// Positions relative to centroid
		ax, az := a.X-cx, a.Z-cz
		bx, bz := b.X-cx, b.Z-cz

		// Outward normal for CCW ring
		dx := bx - ax
		dz := bz - az
		nx := dz
		nz := -dx
		length := float32(math.Sqrt(float64(nx*nx + nz*nz)))
		if length > 0 {
			nx /= length
			nz /= length
		}

		base := uint16(len(vertices))

		// bottom-left, bottom-right, top-right, top-left
		vertices = append(vertices,
			renderer.LitVertex{X: ax, Y: 0, Z: az, NX: nx, NY: 0, NZ: nz, R: red, G: green, B: blue, A: 255},
			renderer.LitVertex{X: bx, Y: 0, Z: bz, NX: nx, NY: 0, NZ: nz, R: red, G: green, B: blue, A: 255},
			renderer.LitVertex{X: bx, Y: height, Z: bz, NX: nx, NY: 0, NZ: nz, R: red, G: green, B: blue, A: 255},
			renderer.LitVertex{X: ax, Y: height, Z: az, NX: nx, NY: 0, NZ: nz, R: red, G: green, B: blue, A: 255},
		)

		indices = append(indices,
			base, base+2, base+1,
			base, base+3, base+2,
		)
	}

	// --- Roof ---
	roofBase := uint16(len(vertices))

	// Create centered points for triangulation
	centered := make([]geojson.Point2D, n)
	for i, p := range outer {
		centered[i] = geojson.Point2D{X: p.X - cx, Z: p.Z - cz}
	}

	for _, p := range centered {
		vertices = append(vertices, renderer.LitVertex{
			X: p.X, Y: height, Z: p.Z,
			NX: 0, NY: 1, NZ: 0,
			R: red, G: green, B: blue, A: 255,
		})
	}

	roofIndices := Triangulate(centered)
	for i := 0; i < len(roofIndices); i += 3 {
		// Reverse winding for correct front-face from above
		indices = append(indices, roofBase+roofIndices[i], roofBase+roofIndices[i+2], roofBase+roofIndices[i+1])
	}

	// Check uint16 vertex limit
	if len(vertices) > 65535 {
		return nil, mgl32.Vec3{}, fmt.Errorf("building exceeds uint16 vertex limit (%d vertices)", len(vertices))
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

	pos := mgl32.Vec3{cx, 0, cz}
	return m, pos, nil
}
