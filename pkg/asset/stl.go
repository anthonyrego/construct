package asset

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/anthonyrego/construct/pkg/renderer"
)

// STLTriangle represents a single triangle from an STL file.
type STLTriangle struct {
	Normal   [3]float32
	Vertices [3][3]float32
}

// ParseSTL reads an STL file (binary or ASCII) and returns triangles.
func ParseSTL(path string) ([]STLTriangle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read STL: %w", err)
	}

	// Detect ASCII vs binary: ASCII starts with "solid"
	if len(data) > 5 && strings.HasPrefix(string(data[:5]), "solid") {
		// Could be ASCII, but binary STL can also start with "solid" in header.
		// Check if it contains "facet" which only appears in ASCII.
		if strings.Contains(string(data[:min(len(data), 1000)]), "facet") {
			return parseASCIISTL(string(data))
		}
	}

	return parseBinarySTL(data)
}

func parseBinarySTL(data []byte) ([]STLTriangle, error) {
	if len(data) < 84 {
		return nil, fmt.Errorf("binary STL too small: %d bytes", len(data))
	}

	triCount := binary.LittleEndian.Uint32(data[80:84])
	expected := 84 + triCount*50
	if uint32(len(data)) < expected {
		return nil, fmt.Errorf("binary STL truncated: expected %d bytes, got %d", expected, len(data))
	}

	triangles := make([]STLTriangle, triCount)
	for i := uint32(0); i < triCount; i++ {
		offset := 84 + i*50
		t := &triangles[i]
		t.Normal[0] = readF32(data[offset:])
		t.Normal[1] = readF32(data[offset+4:])
		t.Normal[2] = readF32(data[offset+8:])
		for v := 0; v < 3; v++ {
			vo := offset + 12 + uint32(v)*12
			t.Vertices[v][0] = readF32(data[vo:])
			t.Vertices[v][1] = readF32(data[vo+4:])
			t.Vertices[v][2] = readF32(data[vo+8:])
		}
	}
	return triangles, nil
}

func parseASCIISTL(data string) ([]STLTriangle, error) {
	var triangles []STLTriangle
	var current STLTriangle
	vertIdx := 0

	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "facet normal") {
			fmt.Sscanf(line, "facet normal %f %f %f",
				&current.Normal[0], &current.Normal[1], &current.Normal[2])
			vertIdx = 0
		} else if strings.HasPrefix(line, "vertex") {
			if vertIdx < 3 {
				fmt.Sscanf(line, "vertex %f %f %f",
					&current.Vertices[vertIdx][0],
					&current.Vertices[vertIdx][1],
					&current.Vertices[vertIdx][2])
				vertIdx++
			}
		} else if strings.HasPrefix(line, "endfacet") {
			triangles = append(triangles, current)
			current = STLTriangle{}
		}
	}
	return triangles, nil
}

func readF32(b []byte) float32 {
	bits := binary.LittleEndian.Uint32(b)
	return *(*float32)(unsafe.Pointer(&bits))
}

// ConvertSTLToMeshBin converts an STL file to the engine's mesh.bin format.
// swapYZ swaps Y and Z axes (OpenSCAD is Z-up, engine is Y-up).
// r, g, b set the vertex color for the entire mesh.
func ConvertSTLToMeshBin(stlPath, meshBinPath string, r, g, b uint8, swapYZ bool) error {
	triangles, err := ParseSTL(stlPath)
	if err != nil {
		return err
	}

	if len(triangles) == 0 {
		return fmt.Errorf("STL has no triangles")
	}

	// Build vertex list with deduplication
	type vertKey struct {
		x, y, z int32 // quantized to 0.001mm precision
	}

	vertMap := make(map[vertKey]uint32)
	var vertices [][3]float32
	var indices []uint32
	// Track face normals per vertex for smooth normal computation
	var vertexNormals []([3]float32)

	quantize := func(v float32) int32 {
		return int32(v * 10000)
	}

	for _, tri := range triangles {
		// Compute face normal from vertices (more reliable than STL normals)
		v0 := tri.Vertices[0]
		v1 := tri.Vertices[1]
		v2 := tri.Vertices[2]

		if swapYZ {
			v0[1], v0[2] = v0[2], v0[1]
			v1[1], v1[2] = v1[2], v1[1]
			v2[1], v2[2] = v2[2], v2[1]
		}

		// Edge vectors
		e1 := [3]float32{v1[0] - v0[0], v1[1] - v0[1], v1[2] - v0[2]}
		e2 := [3]float32{v2[0] - v0[0], v2[1] - v0[1], v2[2] - v0[2]}
		// Cross product
		fn := [3]float32{
			e1[1]*e2[2] - e1[2]*e2[1],
			e1[2]*e2[0] - e1[0]*e2[2],
			e1[0]*e2[1] - e1[1]*e2[0],
		}
		fnLen := float32(math.Sqrt(float64(fn[0]*fn[0] + fn[1]*fn[1] + fn[2]*fn[2])))
		if fnLen > 1e-8 {
			fn[0] /= fnLen
			fn[1] /= fnLen
			fn[2] /= fnLen
		}

		verts := [3][3]float32{v0, v1, v2}
		for _, v := range verts {
			key := vertKey{quantize(v[0]), quantize(v[1]), quantize(v[2])}
			idx, exists := vertMap[key]
			if !exists {
				idx = uint32(len(vertices))
				vertMap[key] = idx
				vertices = append(vertices, v)
				vertexNormals = append(vertexNormals, [3]float32{})
			}
			// Accumulate face normal
			vertexNormals[idx][0] += fn[0]
			vertexNormals[idx][1] += fn[1]
			vertexNormals[idx][2] += fn[2]
			indices = append(indices, idx)
		}
	}

	// Compute centroid
	var cx, cy, cz float32
	for _, v := range vertices {
		cx += v[0]
		cy += v[1]
		cz += v[2]
	}
	n := float32(len(vertices))
	cx /= n
	cy /= n
	cz /= n

	// Build LitVertex array
	litVerts := make([]renderer.LitVertex, len(vertices))
	for i, v := range vertices {
		// Normalize the accumulated normal
		nx := vertexNormals[i][0]
		ny := vertexNormals[i][1]
		nz := vertexNormals[i][2]
		nLen := float32(math.Sqrt(float64(nx*nx + ny*ny + nz*nz)))
		if nLen > 1e-8 {
			nx /= nLen
			ny /= nLen
			nz /= nLen
		}

		litVerts[i] = renderer.LitVertex{
			X:  v[0] - cx,
			Y:  v[1] - cy,
			Z:  v[2] - cz,
			NX: nx,
			NY: ny,
			NZ: nz,
			R:  r,
			G:  g,
			B:  b,
			A:  255,
			// U, V default to 0 (no UV until unwrap stage)
		}
	}

	// Write mesh.bin
	return WriteMeshBin(meshBinPath, litVerts, indices)
}

// WriteMeshBin writes vertices and indices in the engine's binary mesh format.
func WriteMeshBin(path string, vertices []renderer.LitVertex, indices []uint32) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	vertexCount := uint32(len(vertices))
	indexCount := uint32(len(indices))
	vertexSize := uint32(unsafe.Sizeof(renderer.LitVertex{}))

	buf := make([]byte, 8+vertexCount*vertexSize+indexCount*4)

	// Header
	binary.LittleEndian.PutUint32(buf[0:4], vertexCount)
	binary.LittleEndian.PutUint32(buf[4:8], indexCount)

	// Vertices (raw copy)
	offset := uint32(8)
	for _, v := range vertices {
		vBytes := (*[36]byte)(unsafe.Pointer(&v))
		copy(buf[offset:offset+vertexSize], vBytes[:])
		offset += vertexSize
	}

	// Indices
	for _, idx := range indices {
		binary.LittleEndian.PutUint32(buf[offset:offset+4], idx)
		offset += 4
	}

	return os.WriteFile(path, buf, 0o644)
}
