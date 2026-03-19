package mapdata

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/ground"
	"github.com/anthonyrego/construct/pkg/ramp"
	"github.com/anthonyrego/construct/pkg/traffic"
)

// Import reads from .cache/ and CSV files, then writes the map data directory.
func Import(outDir string, minLat, minLon, maxLat, maxLon float64) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output dir: %w", err)
	}

	// --- Buildings ---
	footprints, proj, err := geojson.FetchFootprints(minLat, minLon, maxLat, maxLon, 50000)
	if err != nil {
		return fmt.Errorf("fetching footprints: %w", err)
	}
	fmt.Printf("Import: %d building footprints\n", len(footprints))

	if len(footprints) > 0 {
		var bbls []string
		for _, fp := range footprints {
			bbls = append(bbls, fp.BBL)
		}
		pluto, err := geojson.FetchPLUTO(bbls)
		if err != nil {
			fmt.Println("Warning: could not fetch PLUTO data:", err)
		} else {
			geojson.EnrichFootprints(footprints, pluto)
		}
	}

	blocks := importBuildings(footprints, outDir)
	fmt.Printf("Import: %d blocks written\n", len(blocks))

	// --- Surfaces ---
	if proj != nil {
		importSurfaces(proj, outDir, minLat, minLon, maxLat, maxLon)
	}

	// --- Intersections ---
	if proj != nil {
		importIntersections(proj, outDir, minLat, minLon, maxLat, maxLon)
	}

	// --- Doodads ---
	if proj != nil {
		importRamps(proj, outDir, minLat, minLon, maxLat, maxLon)
		importHydrants(proj, outDir, minLat, minLon, maxLat, maxLon)
		importTrees(proj, outDir, minLat, minLon, maxLat, maxLon)
	}

	// --- meta.json ---
	meta := MapMeta{
		Version: 1,
		Projection: ProjectionConfig{
			RefLat: proj.RefLat(),
			RefLon: proj.RefLon(),
		},
		Bounds: BoundsConfig{
			MinLat: minLat, MinLon: minLon,
			MaxLat: maxLat, MaxLon: maxLon,
		},
		CoordinateSystem: "Coordinates are in local meters relative to projection origin. +X is west, +Z is north.",
	}
	if err := writeJSON(filepath.Join(outDir, "meta.json"), meta); err != nil {
		return fmt.Errorf("writing meta.json: %w", err)
	}

	fmt.Println("Import complete:", outDir)
	return nil
}

func importBuildings(footprints []geojson.Footprint, outDir string) map[string]*BlockData {
	blocks := make(map[string]*BlockData)

	for _, fp := range footprints {
		// BBL digits: borough(1) + block(5) + lot(4) = 10 digits
		// Block key = "B-BBBBB" (borough dash block)
		blockKey := bblToBlockKey(fp.BBL)
		if blockKey == "" {
			continue
		}

		bd, ok := blocks[blockKey]
		if !ok {
			bd = &BlockData{Block: blockKey}
			blocks[blockKey] = bd
		}

		// Compute color from PLUTO data
		c := building.StyleColor(fp.PLUTO)
		color := [3]uint8{c.R, c.G, c.B}

		// Convert footprint rings to [][2]float32
		var outer [][2]float32
		if len(fp.Rings) > 0 {
			for _, pt := range fp.Rings[0] {
				outer = append(outer, [2]float32{pt.X, pt.Z})
			}
		}

		var holes [][][2]float32
		for _, ring := range fp.Rings[1:] {
			var h [][2]float32
			for _, pt := range ring {
				h = append(h, [2]float32{pt.X, pt.Z})
			}
			holes = append(holes, h)
		}

		bd.Buildings = append(bd.Buildings, BuildingData{
			BBL:           fp.BBL,
			Address:       fp.PLUTO.Address,
			Height:        fp.Height,
			Color:         &color,
			BuildingClass: fp.PLUTO.BldgClass,
			LandUse:       fp.PLUTO.LandUse,
			YearBuilt:     fp.PLUTO.YearBuilt,
			Floors:        int(fp.PLUTO.NumFloors),
			Visible:       true,
			Footprint:     outer,
			Holes:         holes,
		})
	}

	// Write block files
	blocksDir := filepath.Join(outDir, "blocks")
	os.MkdirAll(blocksDir, 0o755)
	for key, bd := range blocks {
		if err := writeJSON(filepath.Join(blocksDir, key+".json"), bd); err != nil {
			fmt.Printf("Warning: could not write block %s: %v\n", key, err)
		}
	}

	return blocks
}

func importSurfaces(proj *geojson.Projection, outDir string, minLat, minLon, maxLat, maxLon float64) {
	type surfEntry struct {
		dataset  geojson.DatasetConfig
		surfType string
		label    string
	}
	entries := []surfEntry{
		{ground.RoadbedDataset, "road", "roadbed"},
		{ground.SidewalkDataset, "sidewalk", "sidewalk"},
		{ground.ParkDataset, "park", "park"},
	}

	surfDir := filepath.Join(outDir, "surfaces")
	os.MkdirAll(surfDir, 0o755)

	for _, se := range entries {
		polys, err := geojson.FetchSurfacePolygons(se.dataset, minLat, minLon, maxLat, maxLon, 50000, proj)
		if err != nil {
			fmt.Printf("Warning: could not fetch %s polygons: %v\n", se.label, err)
			continue
		}
		fmt.Printf("Import: %d %s polygons\n", len(polys), se.label)

		sfd := SurfaceFileData{Type: se.surfType}
		for i, poly := range polys {
			var outer [][2]float32
			if len(poly.Rings) > 0 {
				for _, pt := range poly.Rings[0] {
					outer = append(outer, [2]float32{pt.X, pt.Z})
				}
			}
			var holes [][][2]float32
			for _, ring := range poly.Rings[1:] {
				var h [][2]float32
				for _, pt := range ring {
					h = append(h, [2]float32{pt.X, pt.Z})
				}
				holes = append(holes, h)
			}

			sfd.Polygons = append(sfd.Polygons, SurfacePolygonData{
				ID:      fmt.Sprintf("%s-%04d", se.surfType, i+1),
				Name:    poly.Name,
				Visible: true,
				Outer:   outer,
				Holes:   holes,
			})
		}

		if err := writeJSON(filepath.Join(surfDir, se.surfType+"s.json"), sfd); err != nil {
			fmt.Printf("Warning: could not write %s surfaces: %v\n", se.surfType, err)
		}
	}
}

func importIntersections(proj *geojson.Projection, outDir string, minLat, minLon, maxLat, maxLon float64) {
	// Fetch street centerlines
	streets, err := geojson.FetchStreetSegments(traffic.CenterlineDataset, minLat, minLon, maxLat, maxLon, 50000, proj)
	if err != nil {
		fmt.Println("Warning: could not fetch centerlines:", err)
		return
	}
	fmt.Printf("Import: %d street centerline segments\n", len(streets))

	// Fetch OSM traffic signal points
	signalPts, err := geojson.FetchOSMTrafficSignals(minLat, minLon, maxLat, maxLon, proj)
	if err != nil {
		fmt.Println("Warning: could not fetch OSM traffic signals:", err)
		return
	}
	fmt.Printf("Import: %d OSM traffic signal nodes\n", len(signalPts))

	if len(signalPts) == 0 {
		return
	}

	// Use the traffic package to cluster and snap signals
	sys := traffic.NewFromPoints(signalPts, 2.0, streets)

	intDir := filepath.Join(outDir, "intersections")
	os.MkdirAll(intDir, 0o755)

	for i, sig := range sys.Signals {
		seq := i + 1
		id := IntersectionFilename(seq, sig.Street1, sig.Street2)

		d := IntersectionData{
			ID:             id,
			Position:       [2]float32{sig.Position.X, sig.Position.Z},
			Street1:        sig.Street1,
			Street2:        sig.Street2,
			DirectionDeg:   float32(sig.DirAngle * 180 / math.Pi),
			CycleOffsetSec: rand.Float32() * 70.0, // random offset within 70s cycle
			Features:       []any{},
		}

		if err := writeJSON(filepath.Join(intDir, id+".json"), d); err != nil {
			fmt.Printf("Warning: could not write intersection %s: %v\n", id, err)
		}
	}

	fmt.Printf("Import: %d intersections written\n", len(sys.Signals))
}

func importRamps(proj *geojson.Projection, outDir string, minLat, minLon, maxLat, maxLon float64) {
	// Find ramp CSV
	csvPath := findDataFile("Pedestrian_Ramp_Locations")
	if csvPath == "" {
		fmt.Println("Warning: no ramp CSV found in data/")
		return
	}

	ramps := ramp.LoadCSV(csvPath, proj, minLat, minLon, maxLat, maxLon)
	if len(ramps) == 0 {
		return
	}

	// Orient ramps toward nearest street
	streets, err := geojson.FetchStreetSegments(traffic.CenterlineDataset, minLat, minLon, maxLat, maxLon, 50000, proj)
	if err == nil && len(streets) > 0 {
		ramp.Orient(ramps, streets)
	}

	dfd := DoodadFileData{Type: "ramp"}
	for i, r := range ramps {
		dfd.Items = append(dfd.Items, DoodadItem{
			ID:       fmt.Sprintf("ramp-%04d", i+1),
			Position: [2]float32{r.Position.X, r.Position.Z},
			AngleDeg: float32(r.DirAngle * 180 / math.Pi),
			Width:    r.Width,
			Length:   r.Length,
			Visible:  true,
		})
	}

	doodadDir := filepath.Join(outDir, "doodads")
	os.MkdirAll(doodadDir, 0o755)
	if err := writeJSON(filepath.Join(doodadDir, "ramps.json"), dfd); err != nil {
		fmt.Printf("Warning: could not write ramps: %v\n", err)
	}
	fmt.Printf("Import: %d ramps written\n", len(ramps))
}

func importHydrants(proj *geojson.Projection, outDir string, minLat, minLon, maxLat, maxLon float64) {
	csvPath := findDataFile("Hydrants")
	if csvPath == "" {
		fmt.Println("Warning: no hydrant CSV found in data/")
		return
	}

	f, err := os.Open(csvPath)
	if err != nil {
		fmt.Println("Warning: could not open hydrant CSV:", err)
		return
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Warning: could not parse hydrant CSV:", err)
		return
	}
	if len(records) < 2 {
		return
	}

	header := records[0]
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[h] = i
	}

	boroIdx := col["BORO"]
	latIdx := col["LATITUDE"]
	lonIdx := col["LONGITUDE"]

	dfd := DoodadFileData{Type: "hydrant"}
	seq := 0
	for _, row := range records[1:] {
		if len(row) <= boroIdx || len(row) <= latIdx || len(row) <= lonIdx {
			continue
		}
		// Manhattan only
		if strings.TrimSpace(row[boroIdx]) != "1" {
			continue
		}
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(row[latIdx]), 64)
		lon, err2 := strconv.ParseFloat(strings.TrimSpace(row[lonIdx]), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if lat < minLat || lat > maxLat || lon < minLon || lon > maxLon {
			continue
		}

		pt := proj.ToLocal(lat, lon)
		seq++
		dfd.Items = append(dfd.Items, DoodadItem{
			ID:       fmt.Sprintf("hydrant-%04d", seq),
			Position: [2]float32{pt.X, pt.Z},
			Visible:  true,
		})
	}

	if len(dfd.Items) > 0 {
		doodadDir := filepath.Join(outDir, "doodads")
		os.MkdirAll(doodadDir, 0o755)
		if err := writeJSON(filepath.Join(doodadDir, "hydrants.json"), dfd); err != nil {
			fmt.Printf("Warning: could not write hydrants: %v\n", err)
		}
		fmt.Printf("Import: %d hydrants written\n", len(dfd.Items))
	}
}

func importTrees(proj *geojson.Projection, outDir string, minLat, minLon, maxLat, maxLon float64) {
	csvPath := findDataFile("Street_Tree_Census")
	if csvPath == "" {
		fmt.Println("Warning: no tree census CSV found in data/")
		return
	}

	f, err := os.Open(csvPath)
	if err != nil {
		fmt.Println("Warning: could not open tree CSV:", err)
		return
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Println("Warning: could not parse tree CSV:", err)
		return
	}
	if len(records) < 2 {
		return
	}

	header := records[0]
	col := make(map[string]int, len(header))
	for i, h := range header {
		col[h] = i
	}

	boroIdx := col["borocode"]
	latIdx := col["latitude"]
	lonIdx := col["longitude"]
	speciesIdx := col["spc_common"]
	diamIdx := col["tree_dbh"]

	dfd := DoodadFileData{Type: "tree"}
	seq := 0
	for _, row := range records[1:] {
		if len(row) <= boroIdx || len(row) <= latIdx || len(row) <= lonIdx {
			continue
		}
		// Manhattan only (borocode "1")
		if strings.TrimSpace(row[boroIdx]) != "1" {
			continue
		}
		lat, err1 := strconv.ParseFloat(strings.TrimSpace(row[latIdx]), 64)
		lon, err2 := strconv.ParseFloat(strings.TrimSpace(row[lonIdx]), 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if lat < minLat || lat > maxLat || lon < minLon || lon > maxLon {
			continue
		}

		pt := proj.ToLocal(lat, lon)
		seq++

		props := map[string]interface{}{}
		if speciesIdx < len(row) && row[speciesIdx] != "" {
			props["species"] = strings.TrimSpace(row[speciesIdx])
		}
		if diamIdx < len(row) {
			if d, err := strconv.ParseFloat(strings.TrimSpace(row[diamIdx]), 64); err == nil && d > 0 {
				props["diameter"] = d
			}
		}

		dfd.Items = append(dfd.Items, DoodadItem{
			ID:         fmt.Sprintf("tree-%05d", seq),
			Position:   [2]float32{pt.X, pt.Z},
			Visible:    true,
			Properties: props,
		})
	}

	if len(dfd.Items) > 0 {
		doodadDir := filepath.Join(outDir, "doodads")
		os.MkdirAll(doodadDir, 0o755)
		if err := writeJSON(filepath.Join(doodadDir, "trees.json"), dfd); err != nil {
			fmt.Printf("Warning: could not write trees: %v\n", err)
		}
		fmt.Printf("Import: %d trees written\n", len(dfd.Items))
	}
}

// bblToBlockKey extracts "B-BBBBB" block key from a 10-digit BBL string.
func bblToBlockKey(bbl string) string {
	if len(bbl) < 6 {
		return ""
	}
	return bbl[0:1] + "-" + bbl[1:6]
}

// findDataFile finds a CSV file in data/ matching the given prefix.
func findDataFile(prefix string) string {
	matches, _ := filepath.Glob(filepath.Join("data", prefix+"*.csv"))
	if len(matches) == 0 {
		// Try case-insensitive partial match
		entries, _ := os.ReadDir("data")
		for _, e := range entries {
			if strings.Contains(e.Name(), prefix) && strings.HasSuffix(e.Name(), ".csv") {
				return filepath.Join("data", e.Name())
			}
		}
		return ""
	}
	return matches[0]
}
