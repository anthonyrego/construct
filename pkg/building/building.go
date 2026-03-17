package building

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// RawMesh holds CPU-side vertex/index data before GPU upload.
type RawMesh struct {
	Vertices []renderer.LitVertex
	Indices  []uint16
	Position mgl32.Vec3 // Centroid position
	Radius   float32    // Bounding sphere radius
}

// ExtrudeRaw generates lit vertices and indices for a building footprint.
// Vertices are centroid-relative. No GPU resources are created.
func ExtrudeRaw(fp geojson.Footprint, red, green, blue uint8) (*RawMesh, error) {
	outer := fp.Rings[0]
	n := len(outer)
	if n < 3 {
		return nil, fmt.Errorf("footprint has fewer than 3 vertices")
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

	// Compute bounding sphere radius (max XZ distance from centroid + height)
	var maxDistSq float32
	for _, p := range outer {
		dx := p.X - cx
		dz := p.Z - cz
		distSq := dx*dx + dz*dz
		if distSq > maxDistSq {
			maxDistSq = distSq
		}
	}
	xzRadius := float32(math.Sqrt(float64(maxDistSq)))
	boundRadius := float32(math.Sqrt(float64(xzRadius*xzRadius + height*height)))

	// Pre-allocate: walls = 4*n verts, 6*n indices; roof = n verts, 3*(n-2) indices
	vertices := make([]renderer.LitVertex, 0, 5*n)
	indices := make([]uint16, 0, 9*n-6)

	// --- Walls ---
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a := outer[i]
		b := outer[j]

		ax, az := a.X-cx, a.Z-cz
		bx, bz := b.X-cx, b.Z-cz

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
		indices = append(indices, roofBase+roofIndices[i], roofBase+roofIndices[i+2], roofBase+roofIndices[i+1])
	}

	if len(vertices) > 65535 {
		return nil, fmt.Errorf("building exceeds uint16 vertex limit (%d vertices)", len(vertices))
	}

	return &RawMesh{
		Vertices: vertices,
		Indices:  indices,
		Position: mgl32.Vec3{cx, 0, cz},
		Radius:   boundRadius,
	}, nil
}

// UploadMesh uploads a RawMesh to the GPU.
func UploadMesh(r *renderer.Renderer, raw *RawMesh) (*mesh.Mesh, error) {
	vertexBuffer, err := r.CreateLitVertexBuffer(raw.Vertices)
	if err != nil {
		return nil, fmt.Errorf("failed to create vertex buffer: %w", err)
	}

	indexBuffer, err := r.CreateIndexBuffer(raw.Indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, fmt.Errorf("failed to create index buffer: %w", err)
	}

	return &mesh.Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(raw.Indices)),
	}, nil
}

// Extrude creates a lit mesh from a 2D footprint polygon + height.
// Returns the mesh, centroid position, and bounding sphere radius.
func Extrude(r *renderer.Renderer, fp geojson.Footprint, red, green, blue uint8) (*mesh.Mesh, mgl32.Vec3, float32, error) {
	raw, err := ExtrudeRaw(fp, red, green, blue)
	if err != nil {
		return nil, mgl32.Vec3{}, 0, err
	}

	m, err := UploadMesh(r, raw)
	if err != nil {
		return nil, mgl32.Vec3{}, 0, err
	}

	return m, raw.Position, raw.Radius, nil
}

// MergeEntry pairs a building ID with its raw mesh for span-tracked merging.
type MergeEntry struct {
	ID  BuildingID
	Raw *RawMesh
}

// BuildingSpan records a building's index range within a merged mesh.
type BuildingSpan struct {
	BuildingID  BuildingID
	IndexOffset uint32
	IndexCount  uint32
}

// MergeMeshesWithSpans combines raw meshes into a single GPU mesh and records
// per-building index ranges. Vertices are transformed to world space.
func MergeMeshesWithSpans(r *renderer.Renderer, entries []MergeEntry) (*mesh.Mesh, []BuildingSpan, error) {
	var totalVerts, totalIndices int
	for _, e := range entries {
		totalVerts += len(e.Raw.Vertices)
		totalIndices += len(e.Raw.Indices)
	}
	if totalVerts == 0 {
		return nil, nil, fmt.Errorf("no vertices to merge")
	}

	vertices := make([]renderer.LitVertex, 0, totalVerts)
	indices := make([]uint32, 0, totalIndices)
	spans := make([]BuildingSpan, 0, len(entries))

	for _, e := range entries {
		baseVertex := uint32(len(vertices))
		indexOffset := uint32(len(indices))

		cx, cz := e.Raw.Position.X(), e.Raw.Position.Z()
		for _, v := range e.Raw.Vertices {
			v.X += cx
			v.Z += cz
			vertices = append(vertices, v)
		}

		for _, idx := range e.Raw.Indices {
			indices = append(indices, baseVertex+uint32(idx))
		}

		spans = append(spans, BuildingSpan{
			BuildingID:  e.ID,
			IndexOffset: indexOffset,
			IndexCount:  uint32(len(e.Raw.Indices)),
		})
	}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create merged vertex buffer: %w", err)
	}

	indexBuffer, err := r.CreateIndexBuffer32(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, nil, fmt.Errorf("failed to create merged index buffer: %w", err)
	}

	return &mesh.Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, spans, nil
}
