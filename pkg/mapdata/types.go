package mapdata

import "github.com/anthonyrego/construct/pkg/geojson"

// MapMeta holds projection and bounds metadata for the map data directory.
type MapMeta struct {
	Version          int              `json:"version"`
	Projection       ProjectionConfig `json:"projection"`
	Bounds           BoundsConfig     `json:"bounds"`
	CoordinateSystem string           `json:"coordinateSystem"`
}

type ProjectionConfig struct {
	RefLat float64 `json:"refLat"`
	RefLon float64 `json:"refLon"`
}

type BoundsConfig struct {
	MinLat float64 `json:"minLat"`
	MinLon float64 `json:"minLon"`
	MaxLat float64 `json:"maxLat"`
	MaxLon float64 `json:"maxLon"`
}

// BlockData holds all buildings in a single city block.
type BlockData struct {
	Block     string         `json:"block"`
	Buildings []BuildingData `json:"buildings"`
}

// BuildingData holds a single building's properties and geometry.
type BuildingData struct {
	BBL           string         `json:"bbl"`
	Address       string         `json:"address"`
	Height        float32        `json:"height"`
	Color         *[3]uint8      `json:"color,omitempty"`
	BuildingClass string         `json:"buildingClass"`
	LandUse       string         `json:"landUse"`
	YearBuilt     int            `json:"yearBuilt"`
	Floors        int            `json:"floors"`
	Visible       bool           `json:"visible"`
	Footprint     [][2]float32   `json:"footprint"`
	Holes         [][][2]float32 `json:"holes"`
}

// ToFootprint converts back to the type building.Registry.Ingest() expects.
func (b *BuildingData) ToFootprint() geojson.Footprint {
	var rings [][]geojson.Point2D

	outer := make([]geojson.Point2D, len(b.Footprint))
	for i, pt := range b.Footprint {
		outer[i] = geojson.Point2D{X: pt[0], Z: pt[1]}
	}
	rings = append(rings, outer)

	for _, hole := range b.Holes {
		h := make([]geojson.Point2D, len(hole))
		for i, pt := range hole {
			h[i] = geojson.Point2D{X: pt[0], Z: pt[1]}
		}
		rings = append(rings, h)
	}

	return geojson.Footprint{
		Rings:  rings,
		Height: b.Height,
		BBL:    b.BBL,
		PLUTO: geojson.PLUTOData{
			Address:   b.Address,
			BldgClass: b.BuildingClass,
			LandUse:   b.LandUse,
			YearBuilt: b.YearBuilt,
			NumFloors: float32(b.Floors),
		},
	}
}

// IntersectionData holds a single traffic intersection.
type IntersectionData struct {
	ID             string     `json:"id"`
	Position       [2]float32 `json:"position"`
	Street1        string     `json:"street1"`
	Street2        string     `json:"street2"`
	DirectionDeg   float32    `json:"directionDeg"`
	CycleOffsetSec float32    `json:"cycleOffsetSec"`
	Features       []any      `json:"features"`
}

// SurfaceFileData holds all polygons for a single surface type.
type SurfaceFileData struct {
	Type     string               `json:"type"`
	Polygons []SurfacePolygonData `json:"polygons"`
}

// SurfacePolygonData holds a single ground polygon.
type SurfacePolygonData struct {
	ID      string         `json:"id"`
	Name    string         `json:"name,omitempty"`
	Visible bool           `json:"visible"`
	Outer   [][2]float32   `json:"outer"`
	Holes   [][][2]float32 `json:"holes"`
}

// ToSurfacePolygon converts back to the type ground.Flatten() expects.
func (s *SurfacePolygonData) ToSurfacePolygon() geojson.SurfacePolygon {
	var rings [][]geojson.Point2D

	outer := make([]geojson.Point2D, len(s.Outer))
	for i, pt := range s.Outer {
		outer[i] = geojson.Point2D{X: pt[0], Z: pt[1]}
	}
	rings = append(rings, outer)

	for _, hole := range s.Holes {
		h := make([]geojson.Point2D, len(hole))
		for i, pt := range hole {
			h[i] = geojson.Point2D{X: pt[0], Z: pt[1]}
		}
		rings = append(rings, h)
	}

	return geojson.SurfacePolygon{
		Rings: rings,
		Name:  s.Name,
	}
}

// DoodadFileData holds all items for a single doodad type.
type DoodadFileData struct {
	Type  string       `json:"type"`
	Items []DoodadItem `json:"items"`
}

// DoodadItem holds a single point-based entity.
type DoodadItem struct {
	ID         string                 `json:"id"`
	Position   [2]float32             `json:"position"`
	AngleDeg   float32                `json:"angleDeg,omitempty"`
	Width      float32                `json:"width,omitempty"`
	Length     float32                `json:"length,omitempty"`
	Visible    bool                   `json:"visible"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}
