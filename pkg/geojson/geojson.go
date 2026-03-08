package geojson

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// Point2D is a 2D point in game-world meters.
type Point2D struct {
	X, Z float32
}

// Footprint is a parsed building footprint ready for extrusion.
type Footprint struct {
	Rings  [][]Point2D // Outer ring first (CCW), then holes (CW)
	Height float32     // Roof height in meters
	BBL    string      // Borough-Block-Lot identifier
}

// Projection converts WGS84 lat/lon to local meters.
type Projection struct {
	refLat, refLon float64
	cosLat         float64
}

const metersPerDegLat = 111_320.0

func NewProjection(refLat, refLon float64) *Projection {
	return &Projection{
		refLat: refLat,
		refLon: refLon,
		cosLat: math.Cos(refLat * math.Pi / 180.0),
	}
}

func (p *Projection) ToLocal(lat, lon float64) Point2D {
	x := (lon - p.refLon) * metersPerDegLat * p.cosLat
	z := (lat - p.refLat) * metersPerDegLat
	return Point2D{X: float32(x), Z: float32(z)}
}

// SODA API types (unexported)

type sodaRecord struct {
	TheGeom    geojsonGeometry `json:"the_geom"`
	HeightRoof string          `json:"height_roof"`
	BaseBBL    string          `json:"base_bbl"`
}

type geojsonGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

const sodaEndpoint = "https://data.cityofnewyork.us/resource/5zhs-2jue.json"

const cacheDir = ".cache"

// cacheKey returns a deterministic filename for a given query.
func cacheKey(minLat, minLon, maxLat, maxLon float64, limit int) string {
	key := fmt.Sprintf("%f_%f_%f_%f_%d", minLat, minLon, maxLat, maxLon, limit)
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("footprints_%x.json", hash[:8])
}

// FetchFootprints queries the NYC SODA API for building footprints
// within the given bounding box. Results are cached locally so subsequent
// runs don't need to re-fetch.
func FetchFootprints(minLat, minLon, maxLat, maxLon float64, limit int) ([]Footprint, error) {
	data, err := loadOrFetch(minLat, minLon, maxLat, maxLon, limit)
	if err != nil {
		return nil, err
	}

	var records []sodaRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("failed to decode SODA response: %w", err)
	}

	return recordsToFootprints(records)
}

func loadOrFetch(minLat, minLon, maxLat, maxLon float64, limit int) ([]byte, error) {
	filename := filepath.Join(cacheDir, cacheKey(minLat, minLon, maxLat, maxLon, limit))

	// Try cache first
	if data, err := os.ReadFile(filename); err == nil {
		fmt.Println("Loaded building footprints from cache:", filename)
		return data, nil
	}

	// Fetch from API
	where := fmt.Sprintf(
		"within_box(the_geom, %f, %f, %f, %f)",
		maxLat, minLon, minLat, maxLon,
	)

	params := url.Values{}
	params.Set("$where", where)
	params.Set("$select", "the_geom,height_roof,base_bbl")
	params.Set("$limit", strconv.Itoa(limit))

	reqURL := sodaEndpoint + "?" + params.Encode()

	resp, err := http.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("SODA API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("SODA API returned %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read SODA response: %w", err)
	}

	// Save to cache
	if err := os.MkdirAll(cacheDir, 0o755); err == nil {
		if err := os.WriteFile(filename, data, 0o644); err == nil {
			fmt.Println("Cached building footprints to:", filename)
		}
	}

	return data, nil
}

func recordsToFootprints(records []sodaRecord) ([]Footprint, error) {
	var allLats, allLons []float64
	for _, rec := range records {
		lats, lons := extractCoords(rec.TheGeom)
		allLats = append(allLats, lats...)
		allLons = append(allLons, lons...)
	}

	if len(allLats) == 0 {
		return nil, nil
	}

	var sumLat, sumLon float64
	for i := range allLats {
		sumLat += allLats[i]
		sumLon += allLons[i]
	}
	n := float64(len(allLats))
	proj := NewProjection(sumLat/n, sumLon/n)

	var footprints []Footprint
	for _, rec := range records {
		fps := parseRecord(rec, proj)
		footprints = append(footprints, fps...)
	}

	return footprints, nil
}

func extractCoords(geom geojsonGeometry) (lats, lons []float64) {
	switch geom.Type {
	case "Polygon":
		var coords [][][2]float64
		if err := json.Unmarshal(geom.Coordinates, &coords); err != nil {
			return
		}
		for _, ring := range coords {
			for _, pt := range ring {
				lons = append(lons, pt[0])
				lats = append(lats, pt[1])
			}
		}
	case "MultiPolygon":
		var coords [][][][2]float64
		if err := json.Unmarshal(geom.Coordinates, &coords); err != nil {
			return
		}
		for _, poly := range coords {
			for _, ring := range poly {
				for _, pt := range ring {
					lons = append(lons, pt[0])
					lats = append(lats, pt[1])
				}
			}
		}
	}
	return
}

func parseRecord(rec sodaRecord, proj *Projection) []Footprint {
	height := parseHeight(rec.HeightRoof)

	switch rec.TheGeom.Type {
	case "Polygon":
		var coords [][][2]float64
		if err := json.Unmarshal(rec.TheGeom.Coordinates, &coords); err != nil {
			return nil
		}
		fp := buildFootprint(coords, height, rec.BaseBBL, proj)
		if fp != nil {
			return []Footprint{*fp}
		}
	case "MultiPolygon":
		var coords [][][][2]float64
		if err := json.Unmarshal(rec.TheGeom.Coordinates, &coords); err != nil {
			return nil
		}
		var fps []Footprint
		for _, poly := range coords {
			fp := buildFootprint(poly, height, rec.BaseBBL, proj)
			if fp != nil {
				fps = append(fps, *fp)
			}
		}
		return fps
	}
	return nil
}

func buildFootprint(rings [][][2]float64, height float32, bbl string, proj *Projection) *Footprint {
	if len(rings) == 0 || len(rings[0]) < 3 {
		return nil
	}

	var fp Footprint
	fp.Height = height
	fp.BBL = bbl

	for i, ring := range rings {
		var pts []Point2D
		for _, coord := range ring {
			// GeoJSON is [lon, lat]
			pt := proj.ToLocal(coord[1], coord[0])
			pts = append(pts, pt)
		}

		// Remove closing duplicate if present
		if len(pts) > 1 && pts[0] == pts[len(pts)-1] {
			pts = pts[:len(pts)-1]
		}
		if len(pts) < 3 {
			continue
		}

		// Outer ring (i==0) should be CCW; holes should be CW
		if i == 0 {
			if !isCCW(pts) {
				reverse(pts)
			}
		} else {
			if isCCW(pts) {
				reverse(pts)
			}
		}

		fp.Rings = append(fp.Rings, pts)
	}

	if len(fp.Rings) == 0 {
		return nil
	}
	return &fp
}

func parseHeight(s string) float32 {
	if s == "" {
		return 10.0
	}
	v, err := strconv.ParseFloat(s, 32)
	if err != nil || v <= 0 {
		return 10.0
	}
	// Convert feet to meters
	return float32(v * 0.3048)
}

// isCCW returns true if the polygon ring is counter-clockwise
// using the shoelace formula for signed area.
func isCCW(pts []Point2D) bool {
	var area float32
	n := len(pts)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		area += pts[i].X * pts[j].Z
		area -= pts[j].X * pts[i].Z
	}
	return area > 0
}

func reverse(pts []Point2D) {
	for i, j := 0, len(pts)-1; i < j; i, j = i+1, j-1 {
		pts[i], pts[j] = pts[j], pts[i]
	}
}
