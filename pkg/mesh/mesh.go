package mesh

import (
	"github.com/Zyko0/go-sdl3/sdl"

	"github.com/anthonyrego/construct/pkg/renderer"
)

type Mesh struct {
	VertexBuffer *sdl.GPUBuffer
	IndexBuffer  *sdl.GPUBuffer
	IndexCount   uint32
}

func NewCube(r *renderer.Renderer, red, green, blue uint8) (*Mesh, error) {
	// Cube vertices with position and color
	// Each face has a slightly different shade for visibility
	vertices := []renderer.Vertex{
		// Front face (Z+)
		renderer.NewVertex(-0.5, -0.5, 0.5, red, green, blue, 255),
		renderer.NewVertex(0.5, -0.5, 0.5, red, green, blue, 255),
		renderer.NewVertex(0.5, 0.5, 0.5, red, green, blue, 255),
		renderer.NewVertex(-0.5, 0.5, 0.5, red, green, blue, 255),

		// Back face (Z-)
		renderer.NewVertex(0.5, -0.5, -0.5, uint8(float32(red)*0.8), uint8(float32(green)*0.8), uint8(float32(blue)*0.8), 255),
		renderer.NewVertex(-0.5, -0.5, -0.5, uint8(float32(red)*0.8), uint8(float32(green)*0.8), uint8(float32(blue)*0.8), 255),
		renderer.NewVertex(-0.5, 0.5, -0.5, uint8(float32(red)*0.8), uint8(float32(green)*0.8), uint8(float32(blue)*0.8), 255),
		renderer.NewVertex(0.5, 0.5, -0.5, uint8(float32(red)*0.8), uint8(float32(green)*0.8), uint8(float32(blue)*0.8), 255),

		// Top face (Y+)
		renderer.NewVertex(-0.5, 0.5, 0.5, uint8(float32(red)*0.9), uint8(float32(green)*0.9), uint8(float32(blue)*0.9), 255),
		renderer.NewVertex(0.5, 0.5, 0.5, uint8(float32(red)*0.9), uint8(float32(green)*0.9), uint8(float32(blue)*0.9), 255),
		renderer.NewVertex(0.5, 0.5, -0.5, uint8(float32(red)*0.9), uint8(float32(green)*0.9), uint8(float32(blue)*0.9), 255),
		renderer.NewVertex(-0.5, 0.5, -0.5, uint8(float32(red)*0.9), uint8(float32(green)*0.9), uint8(float32(blue)*0.9), 255),

		// Bottom face (Y-)
		renderer.NewVertex(-0.5, -0.5, -0.5, uint8(float32(red)*0.6), uint8(float32(green)*0.6), uint8(float32(blue)*0.6), 255),
		renderer.NewVertex(0.5, -0.5, -0.5, uint8(float32(red)*0.6), uint8(float32(green)*0.6), uint8(float32(blue)*0.6), 255),
		renderer.NewVertex(0.5, -0.5, 0.5, uint8(float32(red)*0.6), uint8(float32(green)*0.6), uint8(float32(blue)*0.6), 255),
		renderer.NewVertex(-0.5, -0.5, 0.5, uint8(float32(red)*0.6), uint8(float32(green)*0.6), uint8(float32(blue)*0.6), 255),

		// Right face (X+)
		renderer.NewVertex(0.5, -0.5, 0.5, uint8(float32(red)*0.85), uint8(float32(green)*0.85), uint8(float32(blue)*0.85), 255),
		renderer.NewVertex(0.5, -0.5, -0.5, uint8(float32(red)*0.85), uint8(float32(green)*0.85), uint8(float32(blue)*0.85), 255),
		renderer.NewVertex(0.5, 0.5, -0.5, uint8(float32(red)*0.85), uint8(float32(green)*0.85), uint8(float32(blue)*0.85), 255),
		renderer.NewVertex(0.5, 0.5, 0.5, uint8(float32(red)*0.85), uint8(float32(green)*0.85), uint8(float32(blue)*0.85), 255),

		// Left face (X-)
		renderer.NewVertex(-0.5, -0.5, -0.5, uint8(float32(red)*0.7), uint8(float32(green)*0.7), uint8(float32(blue)*0.7), 255),
		renderer.NewVertex(-0.5, -0.5, 0.5, uint8(float32(red)*0.7), uint8(float32(green)*0.7), uint8(float32(blue)*0.7), 255),
		renderer.NewVertex(-0.5, 0.5, 0.5, uint8(float32(red)*0.7), uint8(float32(green)*0.7), uint8(float32(blue)*0.7), 255),
		renderer.NewVertex(-0.5, 0.5, -0.5, uint8(float32(red)*0.7), uint8(float32(green)*0.7), uint8(float32(blue)*0.7), 255),
	}

	indices := []uint16{
		// Front
		0, 1, 2, 0, 2, 3,
		// Back
		4, 5, 6, 4, 6, 7,
		// Top
		8, 9, 10, 8, 10, 11,
		// Bottom
		12, 13, 14, 12, 14, 15,
		// Right
		16, 17, 18, 16, 18, 19,
		// Left
		20, 21, 22, 20, 22, 23,
	}

	vertexBuffer, err := r.CreateVertexBuffer(vertices)
	if err != nil {
		return nil, err
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, err
	}

	return &Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, nil
}

func NewLitCube(r *renderer.Renderer, red, green, blue uint8) (*Mesh, error) {
	vertices := []renderer.LitVertex{
		// Front face (Z+) — normal (0, 0, 1)
		{X: -0.5, Y: -0.5, Z: 0.5, NX: 0, NY: 0, NZ: 1, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: -0.5, Z: 0.5, NX: 0, NY: 0, NZ: 1, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: 0.5, Z: 0.5, NX: 0, NY: 0, NZ: 1, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: 0.5, Z: 0.5, NX: 0, NY: 0, NZ: 1, R: red, G: green, B: blue, A: 255},

		// Back face (Z-) — normal (0, 0, -1)
		{X: 0.5, Y: -0.5, Z: -0.5, NX: 0, NY: 0, NZ: -1, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: -0.5, Z: -0.5, NX: 0, NY: 0, NZ: -1, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: 0.5, Z: -0.5, NX: 0, NY: 0, NZ: -1, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: 0.5, Z: -0.5, NX: 0, NY: 0, NZ: -1, R: red, G: green, B: blue, A: 255},

		// Top face (Y+) — normal (0, 1, 0)
		{X: -0.5, Y: 0.5, Z: 0.5, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: 0.5, Z: 0.5, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: 0.5, Z: -0.5, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: 0.5, Z: -0.5, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},

		// Bottom face (Y-) — normal (0, -1, 0)
		{X: -0.5, Y: -0.5, Z: -0.5, NX: 0, NY: -1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: -0.5, Z: -0.5, NX: 0, NY: -1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: -0.5, Z: 0.5, NX: 0, NY: -1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: -0.5, Z: 0.5, NX: 0, NY: -1, NZ: 0, R: red, G: green, B: blue, A: 255},

		// Right face (X+) — normal (1, 0, 0)
		{X: 0.5, Y: -0.5, Z: 0.5, NX: 1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: -0.5, Z: -0.5, NX: 1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: 0.5, Z: -0.5, NX: 1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: 0.5, Y: 0.5, Z: 0.5, NX: 1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},

		// Left face (X-) — normal (-1, 0, 0)
		{X: -0.5, Y: -0.5, Z: -0.5, NX: -1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: -0.5, Z: 0.5, NX: -1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: 0.5, Z: 0.5, NX: -1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: -0.5, Y: 0.5, Z: -0.5, NX: -1, NY: 0, NZ: 0, R: red, G: green, B: blue, A: 255},
	}

	indices := []uint16{
		0, 1, 2, 0, 2, 3, // Front
		4, 5, 6, 4, 6, 7, // Back
		8, 9, 10, 8, 10, 11, // Top
		12, 13, 14, 12, 14, 15, // Bottom
		16, 17, 18, 16, 18, 19, // Right
		20, 21, 22, 20, 22, 23, // Left
	}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, err
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, err
	}

	return &Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, nil
}

func NewGroundPlane(r *renderer.Renderer, size float32, red, green, blue uint8) (*Mesh, error) {
	vertices := []renderer.LitVertex{
		{X: -size, Y: 0, Z: size, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: size, Y: 0, Z: size, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: size, Y: 0, Z: -size, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
		{X: -size, Y: 0, Z: -size, NX: 0, NY: 1, NZ: 0, R: red, G: green, B: blue, A: 255},
	}

	indices := []uint16{0, 1, 2, 0, 2, 3}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, err
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, err
	}

	return &Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, nil
}

func (m *Mesh) Destroy(r *renderer.Renderer) {
	r.ReleaseBuffer(m.VertexBuffer)
	r.ReleaseBuffer(m.IndexBuffer)
}
