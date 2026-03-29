package asset

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"unsafe"

	"github.com/anthonyrego/construct/pkg/renderer"
)

// LoadMesh reads a mesh file from the asset directory and returns LitVertex + index data.
// Currently supports a simple binary format. glTF support will be added when the
// qmuntal/gltf dependency is integrated.
func LoadMesh(dir string) ([]renderer.LitVertex, []uint32, error) {
	path := filepath.Join(dir, "mesh.bin")

	// Check for binary mesh format first
	if _, err := os.Stat(path); err == nil {
		return loadBinaryMesh(path)
	}

	return nil, nil, fmt.Errorf("no mesh file found in %s", dir)
}

// loadBinaryMesh reads a simple binary mesh format:
// Header: vertexCount(uint32) + indexCount(uint32)
// Vertices: vertexCount * sizeof(LitVertex)
// Indices: indexCount * uint32
func loadBinaryMesh(path string) ([]renderer.LitVertex, []uint32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read mesh: %w", err)
	}

	if len(data) < 8 {
		return nil, nil, fmt.Errorf("mesh file too small")
	}

	vertexCount := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	indexCount := uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16 | uint32(data[7])<<24

	vertexSize := uint32(36) // sizeof(LitVertex) = 36 bytes
	expectedSize := 8 + vertexCount*vertexSize + indexCount*4
	if uint32(len(data)) < expectedSize {
		return nil, nil, fmt.Errorf("mesh file truncated: expected %d bytes, got %d", expectedSize, len(data))
	}

	vertices := make([]renderer.LitVertex, vertexCount)
	offset := uint32(8)
	for i := uint32(0); i < vertexCount; i++ {
		v := &vertices[i]
		b := data[offset:]
		v.X = readFloat32(b[0:4])
		v.Y = readFloat32(b[4:8])
		v.Z = readFloat32(b[8:12])
		v.NX = readFloat32(b[12:16])
		v.NY = readFloat32(b[16:20])
		v.NZ = readFloat32(b[20:24])
		v.R = b[24]
		v.G = b[25]
		v.B = b[26]
		v.A = b[27]
		v.U = readFloat32(b[28:32])
		v.V = readFloat32(b[32:36])
		offset += vertexSize
	}

	indices := make([]uint32, indexCount)
	for i := uint32(0); i < indexCount; i++ {
		b := data[offset:]
		indices[i] = uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		offset += 4
	}

	return vertices, indices, nil
}

func readFloat32(b []byte) float32 {
	bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	return *(*float32)(unsafe.Pointer(&bits))
}

// LoadTexture reads a PNG texture from the asset directory and returns RGBA pixel data.
func LoadTexture(dir string) (width, height uint32, pixels []byte, err error) {
	path := filepath.Join(dir, "texture.png")
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("open texture: %w", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("decode texture: %w", err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	rgba := make([]byte, w*h*4)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			idx := (y*w + x) * 4
			rgba[idx+0] = uint8(r >> 8)
			rgba[idx+1] = uint8(g >> 8)
			rgba[idx+2] = uint8(b >> 8)
			rgba[idx+3] = uint8(a >> 8)
		}
	}

	// If the image is already NRGBA, use it directly for better performance
	if nrgba, ok := img.(*image.NRGBA); ok {
		return uint32(w), uint32(h), nrgba.Pix, nil
	}

	return uint32(w), uint32(h), rgba, nil
}
