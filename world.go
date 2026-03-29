package main

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/asset"
	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/doodad"
	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/ground"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/ramp"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/traffic"
)

const mapDataDir = "data/map"

// MapWorld consolidates all map-data-dependent state for clean reload.
type MapWorld struct {
	rend         *renderer.Renderer
	gridCellSize float32
	reg          *building.Registry
	scene        *scene.Scene
	grid         *scene.SpatialGrid
	groundMeshes []*mesh.Mesh
	trafficSys   *traffic.System
	doodads      map[string]*doodad.System
	store        *mapdata.Store
}

func newMapWorld(rend *renderer.Renderer, cellSize float32) *MapWorld {
	return &MapWorld{
		rend:         rend,
		gridCellSize: cellSize,
		doodads:      make(map[string]*doodad.System),
	}
}

// destroyResources releases all GPU resources and nils pointers. Idempotent.
func (w *MapWorld) destroyResources() {
	if w.reg != nil {
		w.reg.Destroy()
		w.reg = nil
	}
	for _, gm := range w.groundMeshes {
		gm.Destroy(w.rend)
	}
	w.groundMeshes = nil
	if w.trafficSys != nil {
		w.trafficSys.Destroy(w.rend)
		w.trafficSys = nil
	}
	for _, sys := range w.doodads {
		sys.Destroy(w.rend)
	}
	w.doodads = make(map[string]*doodad.System)
	w.scene = nil
	w.grid = nil
}

// Destroy releases all resources. Used by defer.
func (w *MapWorld) Destroy() {
	w.destroyResources()
}

// Reload destroys current resources and reloads from map data.
func (w *MapWorld) Reload(dir string) error {
	w.destroyResources()

	store, err := mapdata.Load(dir)
	if err != nil {
		return err
	}
	w.store = store

	w.reg = building.NewRegistry(w.rend, w.gridCellSize)
	w.scene = &scene.Scene{}

	loadFromMapData(w.rend, store, w.reg, w.scene, &w.groundMeshes, &w.trafficSys, w.doodads)

	w.grid = scene.NewSpatialGrid(w.scene.Objects, w.gridCellSize)
	cellMeshes := w.reg.BuildCellMeshes()
	for key, cm := range cellMeshes {
		w.grid.CellMeshes[key] = &scene.CellMesh{Mesh: cm.Mesh, CellX: cm.CellX, CellZ: cm.CellZ}
	}
	fmt.Printf("Reload complete: %d objects, %d cell meshes\n", len(w.scene.Objects), len(cellMeshes))
	return nil
}

func loadFromAPIs(
	rend *renderer.Renderer, reg *building.Registry, s *scene.Scene,
	groundMeshes *[]*mesh.Mesh, trafficSys **traffic.System,
	doodads map[string]*doodad.System,
	minLat, minLon, maxLat, maxLon float64,
) {
	footprints, proj, err := geojson.FetchFootprints(minLat, minLon, maxLat, maxLon, 50000)
	if err != nil {
		fmt.Println("Warning: could not fetch buildings:", err)
	} else {
		fmt.Printf("Fetched %d building footprints\n", len(footprints))
	}

	if len(footprints) > 0 {
		var bbls []string
		for _, fp := range footprints {
			bbls = append(bbls, fp.BBL)
		}
		pluto, err := geojson.FetchPLUTO(bbls)
		if err != nil {
			fmt.Println("Warning: could not fetch PLUTO data:", err)
		} else {
			fmt.Printf("Fetched PLUTO data for %d lots\n", len(pluto))
			geojson.EnrichFootprints(footprints, pluto)
		}
	}

	count := reg.Ingest(footprints)
	fmt.Printf("Registered %d buildings\n", count)

	for _, b := range reg.Buildings() {
		s.Add(scene.Object{
			Mesh:       b.Mesh,
			Position:   b.Position,
			Scale:      mgl32.Vec3{1, 1, 1},
			Radius:     b.Radius,
			BuildingID: uint32(b.ID) + 1,
		})
	}

	if proj != nil {
		type surfaceEntry struct {
			dataset  geojson.DatasetConfig
			surfType ground.SurfaceType
			label    string
		}
		surfaces := []surfaceEntry{
			{ground.RoadbedDataset, ground.Roadbed, "roadbed"},
			{ground.SidewalkDataset, ground.Sidewalk, "sidewalk"},
			{ground.ParkDataset, ground.Park, "park"},
		}
		for _, se := range surfaces {
			polys, err := geojson.FetchSurfacePolygons(se.dataset, minLat, minLon, maxLat, maxLon, 50000, proj)
			if err != nil {
				fmt.Printf("Warning: could not fetch %s polygons: %v\n", se.label, err)
				continue
			}
			fmt.Printf("Fetched %d %s polygons\n", len(polys), se.label)
			for _, poly := range polys {
				m, pos, radius, err := ground.Flatten(rend, poly, se.surfType)
				if err != nil {
					continue
				}
				*groundMeshes = append(*groundMeshes, m)
				s.Add(scene.Object{Mesh: m, Position: pos, Scale: mgl32.Vec3{1, 1, 1}, Radius: radius, SurfaceType: int(se.surfType) + 1})
			}
		}
	}

	var streets []geojson.StreetSegment
	if proj != nil {
		segs, err := geojson.FetchStreetSegments(traffic.CenterlineDataset, minLat, minLon, maxLat, maxLon, 50000, proj)
		if err != nil {
			fmt.Println("Warning: could not fetch centerlines:", err)
		} else {
			fmt.Printf("Fetched %d street centerline segments\n", len(segs))
			streets = segs
		}

		signalPts, err := geojson.FetchOSMTrafficSignals(minLat, minLon, maxLat, maxLon, proj)
		if err != nil {
			fmt.Println("Warning: could not fetch OSM traffic signals:", err)
		} else {
			fmt.Printf("Fetched %d traffic signals from OpenStreetMap\n", len(signalPts))
		}

		if len(signalPts) > 0 {
			*trafficSys = traffic.NewFromPoints(rend, signalPts, 2.0, streets)
		}
	}

	if proj != nil {
		ramps := ramp.LoadCSV("data/Pedestrian_Ramp_Locations_20260317.csv", proj,
			minLat, minLon, maxLat, maxLon)
		if len(ramps) > 0 {
			if len(streets) > 0 {
				ramp.Orient(ramps, streets)
			}
			// Convert legacy ramp data to DoodadItems for the generic system
			items := make([]mapdata.DoodadItem, len(ramps))
			for i, r := range ramps {
				items[i] = mapdata.DoodadItem{
					Position: [2]float32{r.Position.X, r.Position.Z},
					AngleDeg: r.DirAngle * 180 / 3.14159265,
					Width:    r.Width,
					Length:   r.Length,
					Visible:  true,
				}
			}
			sys, err := doodad.New(rend, "ramp", items, doodad.RampConfig)
			if err != nil {
				fmt.Println("Warning: could not create ramp system:", err)
			} else {
				fmt.Printf("Loaded %d pedestrian ramps\n", len(ramps))
				doodads["ramp"] = sys
			}
		}
	}
}

func loadFromMapData(
	rend *renderer.Renderer, store *mapdata.Store, reg *building.Registry, s *scene.Scene,
	groundMeshes *[]*mesh.Mesh, trafficSys **traffic.System,
	doodads map[string]*doodad.System,
) {
	// --- Buildings ---
	var allFootprints []geojson.Footprint
	for _, block := range store.Blocks {
		for i := range block.Buildings {
			b := &block.Buildings[i]
			if !b.Visible {
				continue
			}
			fp := b.ToFootprint()
			if b.Color != nil {
				fp.PLUTO.BldgClass = b.BuildingClass
				fp.PLUTO.LandUse = b.LandUse
			}
			allFootprints = append(allFootprints, fp)
		}
	}

	count := reg.Ingest(allFootprints)
	fmt.Printf("Loaded %d buildings from map data\n", count)

	// Check for processed building assets and replace meshes
	assetCount := 0
	for _, b := range reg.Buildings() {
		dir := asset.AssetDir("data/assets", b.BBL)
		m, err := asset.LoadManifest(dir)
		if err != nil || !m.IsStageComplete("bake") {
			continue
		}
		verts, indices, err := asset.LoadMesh(dir)
		if err != nil {
			continue
		}
		// Compute centroid from vertices
		var cx, cz float32
		for _, v := range verts {
			cx += v.X
			cz += v.Z
		}
		n := float32(len(verts))
		cx /= n
		cz /= n
		// Compute bounding radius
		var maxR float32
		for _, v := range verts {
			dx := v.X - cx
			dz := v.Z - cz
			dy := v.Y
			r := dx*dx + dz*dz + dy*dy
			if r > maxR {
				maxR = r
			}
		}
		raw := &building.RawMesh{
			Vertices: verts,
			Indices:  make([]uint16, len(indices)),
			Position: mgl32.Vec3{cx, 0, cz},
			Radius:   float32(math.Sqrt(float64(maxR))),
		}
		for i, idx := range indices {
			raw.Indices[i] = uint16(idx)
		}
		if err := reg.ReplaceMesh(b.ID, raw); err == nil {
			assetCount++
		}
	}
	if assetCount > 0 {
		fmt.Printf("Loaded %d building assets from data/assets/\n", assetCount)
	}

	for _, b := range reg.Buildings() {
		s.Add(scene.Object{
			Mesh:       b.Mesh,
			Position:   b.Position,
			Scale:      mgl32.Vec3{1, 1, 1},
			Radius:     b.Radius,
			BuildingID: uint32(b.ID) + 1,
		})
	}

	// --- Surfaces ---
	surfTypeMap := map[string]ground.SurfaceType{
		"road":     ground.Roadbed,
		"sidewalk": ground.Sidewalk,
		"park":     ground.Park,
	}
	for typeName, surfType := range surfTypeMap {
		sfd, ok := store.Surfaces[typeName]
		if !ok {
			continue
		}
		loaded := 0
		for _, sp := range sfd.Polygons {
			if !sp.Visible {
				continue
			}
			poly := sp.ToSurfacePolygon()
			m, pos, radius, err := ground.Flatten(rend, poly, surfType)
			if err != nil {
				continue
			}
			*groundMeshes = append(*groundMeshes, m)
			s.Add(scene.Object{Mesh: m, Position: pos, Scale: mgl32.Vec3{1, 1, 1}, Radius: radius, SurfaceType: int(surfType) + 1})
			loaded++
		}
		fmt.Printf("Loaded %d %s polygons from map data\n", loaded, typeName)
	}

	// --- Traffic signals ---
	if len(store.Intersections) > 0 {
		var intersections []traffic.MapIntersection
		for _, d := range store.Intersections {
			intersections = append(intersections, traffic.MapIntersection{
				ID:             d.ID,
				Position:       d.Position,
				Street1:        d.Street1,
				Street2:        d.Street2,
				DirectionDeg:   d.DirectionDeg,
				CycleOffsetSec: d.CycleOffsetSec,
			})
		}
		*trafficSys = traffic.NewFromMapData(rend, intersections, 2.0)
		fmt.Printf("Loaded %d intersections from map data\n", len(intersections))
	}

	// --- Doodads (trees, hydrants, ramps) ---
	doodadTypes := map[string]doodad.TypeConfig{
		"tree":    doodad.TreeConfig,
		"hydrant": doodad.HydrantConfig,
		"ramp":    doodad.RampConfig,
	}
	for typeName, cfg := range doodadTypes {
		data, ok := store.Doodads[typeName]
		if !ok || len(data.Items) == 0 {
			continue
		}
		sys, err := doodad.New(rend, typeName, data.Items, cfg)
		if err != nil {
			fmt.Printf("Warning: could not create %s system: %v\n", typeName, err)
			continue
		}
		doodads[typeName] = sys
		fmt.Printf("Loaded %d %ss from map data\n", len(sys.Instances), typeName)
	}
}
