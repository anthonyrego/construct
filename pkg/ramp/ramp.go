package ramp

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// Ramp represents a single pedestrian ramp from NYC open data.
type Ramp struct {
	Position geojson.Point2D
	Width    float32 // meters
	Length   float32 // meters
	DirAngle float32 // rotation to face the road (radians)
}

// System holds all ramps and the shared unit mesh.
type System struct {
	Ramps []Ramp
	Mesh  *mesh.Mesh
}

const (
	defaultWidth  float32 = 1.2 // meters
	defaultLength float32 = 1.2
	inchToMeter           = 0.0254
)

// LoadCSV parses the pedestrian ramp CSV, filtering to Manhattan (Borough "1")
// and the given bounding box.
func LoadCSV(path string, proj *geojson.Projection, minLat, minLon, maxLat, maxLon float64) []Ramp {
	f, err := os.Open(path)
	if err != nil {
		fmt.Println("Warning: could not open ramp CSV:", err)
		return nil
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Warning: could not parse ramp CSV:", err)
		return nil
	}

	if len(records) < 2 {
		return nil
	}

	// Build header index
	header := records[0]
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[h] = i
	}

	geomIdx := col["the_geom"]
	boroughIdx := col["Borough"]
	widthIdx := col["RAMP_WIDTH"]
	lengthIdx := col["RAMP_LENGTH"]

	var ramps []Ramp
	for _, row := range records[1:] {
		if len(row) <= geomIdx || len(row) <= boroughIdx {
			continue
		}

		// Manhattan only
		if strings.TrimSpace(row[boroughIdx]) != "1" {
			continue
		}

		// Parse WKT POINT
		lon, lat, ok := parseWKTPoint(row[geomIdx])
		if !ok {
			continue
		}

		// Bounding box filter
		if lat < minLat || lat > maxLat || lon < minLon || lon > maxLon {
			continue
		}

		w := parseInches(row[widthIdx], defaultWidth)
		l := parseInches(row[lengthIdx], defaultLength)

		pos := proj.ToLocal(lat, lon)
		ramps = append(ramps, Ramp{
			Position: pos,
			Width:    w,
			Length:   l,
		})
	}

	return ramps
}

// parseWKTPoint extracts lon, lat from "POINT (lon lat)".
func parseWKTPoint(s string) (lon, lat float64, ok bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "POINT (")
	s = strings.TrimPrefix(s, "POINT(")
	s = strings.TrimSuffix(s, ")")
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, 0, false
	}
	lon, err1 := strconv.ParseFloat(parts[0], 64)
	lat, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lon, lat, true
}

// parseInches converts an inches string to meters, returning the default
// for sentinel values (888, 999, negatives) or parse errors.
func parseInches(s string, def float32) float32 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 || v >= 888 {
		return def
	}
	return float32(v) * inchToMeter
}

// Orient sets each ramp's DirAngle to face the nearest street.
func Orient(ramps []Ramp, streets []geojson.StreetSegment) {
	for i := range ramps {
		bestDist := float32(math.MaxFloat32)
		var bestPerp geojson.Point2D

		for _, seg := range streets {
			for j := 0; j < len(seg.Points)-1; j++ {
				d, perp := pointToSegmentInfo(ramps[i].Position, seg.Points[j], seg.Points[j+1])
				if d < bestDist {
					bestDist = d
					bestPerp = perp
				}
			}
		}

		if bestDist > 50 {
			continue
		}

		// Point from ramp toward road (opposite of perp which points from segment toward ramp)
		ramps[i].DirAngle = float32(math.Atan2(float64(-bestPerp.X), float64(-bestPerp.Z)))
	}
}

// pointToSegmentInfo returns distance from p to segment a-b and the unit
// perpendicular from segment toward p. Duplicated from pkg/traffic (unexported).
func pointToSegmentInfo(p, a, b geojson.Point2D) (float32, geojson.Point2D) {
	dx := b.X - a.X
	dz := b.Z - a.Z
	lenSq := dx*dx + dz*dz
	if lenSq < 1e-8 {
		ddx := p.X - a.X
		ddz := p.Z - a.Z
		d := float32(math.Sqrt(float64(ddx*ddx + ddz*ddz)))
		return d, geojson.Point2D{}
	}

	t := ((p.X-a.X)*dx + (p.Z-a.Z)*dz) / lenSq
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	closest := geojson.Point2D{
		X: a.X + t*dx,
		Z: a.Z + t*dz,
	}

	toP := geojson.Point2D{X: p.X - closest.X, Z: p.Z - closest.Z}
	d := float32(math.Sqrt(float64(toP.X*toP.X + toP.Z*toP.Z)))

	if d < 1e-6 {
		segLen := float32(math.Sqrt(float64(lenSq)))
		return 0, geojson.Point2D{X: -dz / segLen, Z: dx / segLen}
	}

	return d, geojson.Point2D{X: toP.X / d, Z: toP.Z / d}
}

// MapDoodad holds the data needed to create a Ramp from map data.
type MapDoodad struct {
	Position [2]float32
	AngleDeg float32
	Width    float32
	Length   float32
}

// NewFromMapData creates ramps from pre-processed map data (no CSV parsing or orientation needed).
func NewFromMapData(r *renderer.Renderer, items []MapDoodad, curbHeight float32) (*System, error) {
	ramps := make([]Ramp, len(items))
	for i, d := range items {
		ramps[i] = Ramp{
			Position: geojson.Point2D{X: d.Position[0], Z: d.Position[1]},
			Width:    d.Width,
			Length:   d.Length,
			DirAngle: d.AngleDeg * math.Pi / 180,
		}
	}
	m, err := newMesh(r, curbHeight)
	if err != nil {
		return nil, err
	}
	return &System{Ramps: ramps, Mesh: m}, nil
}

// New creates a ramp System with a shared unit mesh.
func New(r *renderer.Renderer, ramps []Ramp, curbHeight float32) (*System, error) {
	m, err := newMesh(r, curbHeight)
	if err != nil {
		return nil, err
	}
	return &System{Ramps: ramps, Mesh: m}, nil
}

// newMesh builds the unit ramp mesh with flanking flare wings (like real ADA ramps).
// Origin is at the sidewalk edge (high end). Road edge extends in +Z direction.
// Flare wings extend ±0.5 beyond the ramp edges, creating a trapezoidal footprint.
//
// Plan view:
//
//	A---B-------C---D    (sidewalk edge, Y=curbHeight)
//	 \  |       |  /
//	  \ |  ramp | /
//	   \|       |/
//	    E-------F        (road edge, Y=0)
func newMesh(r *renderer.Renderer, curbHeight float32) (*mesh.Mesh, error) {
	// All sloped surfaces share the same normal (same tilt angle)
	ny := float32(1.0)
	nz := curbHeight
	nLen := float32(math.Sqrt(float64(ny*ny + nz*nz)))
	ny /= nLen
	nz /= nLen

	const cr, cg, cb uint8 = 70, 65, 60
	wr, wg, wb := uint8(int(cr)*7/10), uint8(int(cg)*7/10), uint8(int(cb)*7/10)

	ch := curbHeight

	vertices := []renderer.LitVertex{
		// Ramp surface (4 verts) — B, C, F, E
		{X: -0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255}, // 0: B
		{X: 0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},  // 1: C
		{X: 0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},   // 2: F
		{X: -0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},  // 3: E

		// Left flare surface (3 verts) — A, E, B
		{X: -1.0, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255}, // 4: A
		{X: -0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},  // 5: E
		{X: -0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255}, // 6: B

		// Right flare surface (3 verts) — C, F, D
		{X: 0.5, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255}, // 7: C
		{X: 0.5, Y: 0, Z: 1, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},  // 8: F
		{X: 1.0, Y: ch, Z: 0, NX: 0, NY: ny, NZ: nz, R: cr, G: cg, B: cb, A: 255},  // 9: D

		// Left outer wall (3 verts) — under diagonal edge A→E
		{X: -1.0, Y: ch, Z: 0, NX: -1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255}, // 10: A
		{X: -1.0, Y: 0, Z: 0, NX: -1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},  // 11: A bottom
		{X: -0.5, Y: 0, Z: 1, NX: -1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},  // 12: E

		// Right outer wall (3 verts) — under diagonal edge D→F
		{X: 1.0, Y: ch, Z: 0, NX: 1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255}, // 13: D
		{X: 0.5, Y: 0, Z: 1, NX: 1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},  // 14: F
		{X: 1.0, Y: 0, Z: 0, NX: 1, NY: 0, NZ: 0, R: wr, G: wg, B: wb, A: 255},  // 15: D bottom
	}

	indices := []uint16{
		// Ramp surface (CCW from above)
		0, 2, 1,
		0, 3, 2,
		// Left flare
		4, 5, 6,
		// Right flare
		7, 8, 9,
		// Left outer wall
		10, 11, 12,
		// Right outer wall
		13, 14, 15,
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

// Destroy releases the shared mesh GPU buffers.
func (s *System) Destroy(r *renderer.Renderer) {
	if s != nil && s.Mesh != nil {
		s.Mesh.Destroy(r)
	}
}
