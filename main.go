package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/admin"
	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/camera"
	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/ground"
	"github.com/anthonyrego/construct/pkg/input"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/ramp"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/settings"
	"github.com/anthonyrego/construct/pkg/sign"
	"github.com/anthonyrego/construct/pkg/snow"
	"github.com/anthonyrego/construct/pkg/traffic"
	"github.com/anthonyrego/construct/pkg/ui"
	"github.com/anthonyrego/construct/pkg/window"
)

const (
	defaultWindowWidth  = 1280
	defaultWindowHeight = 720
	defaultPixelScale   = 4
)

type SceneConfig struct {
	WindowWidth  int  `json:"windowWidth"`
	WindowHeight int  `json:"windowHeight"`
	Fullscreen   bool `json:"fullscreen"`
	PixelScale     int     `json:"pixelScale"`
	RenderDistance float32 `json:"renderDistance"`
	PostProcess  struct {
		DitherStrength float32 `json:"ditherStrength"`
		ColorLevels    float32 `json:"colorLevels"`
		TintR          float32 `json:"tintR"`
		TintG          float32 `json:"tintG"`
		TintB          float32 `json:"tintB"`
	} `json:"postProcess"`
	Lighting struct {
		AmbientR             float32 `json:"ambientR"`
		AmbientG             float32 `json:"ambientG"`
		AmbientB             float32 `json:"ambientB"`
		StreetLightR         float32 `json:"streetLightR"`
		StreetLightG         float32 `json:"streetLightG"`
		StreetLightB         float32 `json:"streetLightB"`
		StreetLightIntensity float32 `json:"streetLightIntensity"`
		SunDirX              float32 `json:"sunDirX"`
		SunDirY              float32 `json:"sunDirY"`
		SunDirZ              float32 `json:"sunDirZ"`
		SunR                 float32 `json:"sunR"`
		SunG                 float32 `json:"sunG"`
		SunB                 float32 `json:"sunB"`
		SunIntensity         float32 `json:"sunIntensity"`
	} `json:"lighting"`
	Headlamp struct {
		R         float32 `json:"r"`
		G         float32 `json:"g"`
		B         float32 `json:"b"`
		Intensity float32 `json:"intensity"`
	} `json:"headlamp"`
	Snow struct {
		Count        int     `json:"count"`
		FallSpeed    float32 `json:"fallSpeed"`
		WindStrength float32 `json:"windStrength"`
		ParticleSize float32 `json:"particleSize"`
	} `json:"snow"`
	Fog struct {
		R     float32 `json:"r"`
		G     float32 `json:"g"`
		B     float32 `json:"b"`
		Start float32 `json:"start"`
		End   float32 `json:"end"`
	} `json:"fog"`
	Textures struct {
		GroundScale    float32 `json:"groundScale"`
		GroundStrength float32 `json:"groundStrength"`
	} `json:"textures"`
}

type ConfigWatcher struct {
	path    string
	modTime time.Time
}

func (cw *ConfigWatcher) Load() (*SceneConfig, bool) {
	info, err := os.Stat(cw.path)
	if err != nil {
		return nil, false
	}
	if !info.ModTime().After(cw.modTime) {
		return nil, false
	}
	cw.modTime = info.ModTime()

	data, err := os.ReadFile(cw.path)
	if err != nil {
		fmt.Println("Config read error:", err)
		return nil, false
	}

	var cfg SceneConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Println("Config parse error:", err)
		return nil, false
	}

	fmt.Println("Config reloaded")
	return &cfg, true
}

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
	signMeshes   map[string]*mesh.Mesh
	signWidths   map[string]float32
	rampSys      *ramp.System
	store        *mapdata.Store
}

func newMapWorld(rend *renderer.Renderer, cellSize float32) *MapWorld {
	return &MapWorld{
		rend:         rend,
		gridCellSize: cellSize,
		signMeshes:   make(map[string]*mesh.Mesh),
		signWidths:   make(map[string]float32),
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
	for _, sm := range w.signMeshes {
		sm.Destroy(w.rend)
	}
	w.signMeshes = make(map[string]*mesh.Mesh)
	w.signWidths = make(map[string]float32)
	if w.rampSys != nil {
		w.rampSys.Destroy(w.rend)
		w.rampSys = nil
	}
	w.trafficSys = nil
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

	loadFromMapData(w.rend, store, w.reg, w.scene, &w.groundMeshes, &w.trafficSys, w.signMeshes, w.signWidths, &w.rampSys)

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
	signMeshes map[string]*mesh.Mesh, signWidths map[string]float32,
	rampSys **ramp.System,
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
			*trafficSys = traffic.NewFromPoints(signalPts, 2.0, streets)

			for _, sig := range (*trafficSys).Signals {
				for _, name := range []string{sig.Street1, sig.Street2} {
					if name == "" {
						continue
					}
					if _, exists := signMeshes[name]; exists {
						continue
					}
					m, w, err := sign.NewMesh(rend, name)
					if err != nil {
						fmt.Printf("Warning: could not create sign mesh for %q: %v\n", name, err)
						continue
					}
					signMeshes[name] = m
					signWidths[name] = w
				}
			}
			fmt.Printf("Created %d unique street sign meshes\n", len(signMeshes))
		}
	}

	if proj != nil {
		ramps := ramp.LoadCSV("data/Pedestrian_Ramp_Locations_20260317.csv", proj,
			minLat, minLon, maxLat, maxLon)
		if len(ramps) > 0 {
			if len(streets) > 0 {
				ramp.Orient(ramps, streets)
			}
			rs, err := ramp.New(rend, ramps, 0.14)
			if err != nil {
				fmt.Println("Warning: could not create ramp system:", err)
			} else {
				fmt.Printf("Loaded %d pedestrian ramps\n", len(ramps))
				*rampSys = rs
			}
		}
	}
}

func loadFromMapData(
	rend *renderer.Renderer, store *mapdata.Store, reg *building.Registry, s *scene.Scene,
	groundMeshes *[]*mesh.Mesh, trafficSys **traffic.System,
	signMeshes map[string]*mesh.Mesh, signWidths map[string]float32,
	rampSys **ramp.System,
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
			// Override PLUTO-computed color with stored color if present
			if b.Color != nil {
				fp.PLUTO.BldgClass = b.BuildingClass
				fp.PLUTO.LandUse = b.LandUse
			}
			allFootprints = append(allFootprints, fp)
		}
	}

	count := reg.Ingest(allFootprints)
	fmt.Printf("Loaded %d buildings from map data\n", count)

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
		*trafficSys = traffic.NewFromMapData(intersections, 2.0)
		fmt.Printf("Loaded %d intersections from map data\n", len(intersections))

		for _, sig := range (*trafficSys).Signals {
			for _, name := range []string{sig.Street1, sig.Street2} {
				if name == "" {
					continue
				}
				if _, exists := signMeshes[name]; exists {
					continue
				}
				m, w, err := sign.NewMesh(rend, name)
				if err != nil {
					continue
				}
				signMeshes[name] = m
				signWidths[name] = w
			}
		}
		fmt.Printf("Created %d unique street sign meshes\n", len(signMeshes))
	}

	// --- Ramps ---
	if rampData, ok := store.Doodads["ramp"]; ok && len(rampData.Items) > 0 {
		var items []ramp.MapDoodad
		for _, d := range rampData.Items {
			if !d.Visible {
				continue
			}
			items = append(items, ramp.MapDoodad{
				Position: d.Position,
				AngleDeg: d.AngleDeg,
				Width:    d.Width,
				Length:   d.Length,
			})
		}
		rs, err := ramp.NewFromMapData(rend, items, 0.14)
		if err != nil {
			fmt.Println("Warning: could not create ramp system:", err)
		} else {
			*rampSys = rs
			fmt.Printf("Loaded %d ramps from map data\n", len(items))
		}
	}
}

func main() {
	doImport := flag.Bool("import", false, "Import .cache/ data into data/map/ and exit")
	useFetch := flag.Bool("fetch", false, "Use old fetch-from-API pipeline instead of map data")
	flag.Parse()

	minLat, minLon, maxLat, maxLon := 40.700, -74.020, 40.747, -73.970

	if *doImport {
		if err := mapdata.Import(mapDataDir, minLat, minLon, maxLat, maxLon); err != nil {
			fmt.Println("Import failed:", err)
			os.Exit(1)
		}
		return
	}

	// Try to load system SDL3, fall back to embedded
	err := sdl.LoadLibrary(sdl.Path())
	if err != nil {
		fmt.Println("Loading embedded SDL3 library...")
		defer binsdl.Load().Unload()
	}

	// Initialize SDL
	err = sdl.Init(sdl.INIT_VIDEO)
	if err != nil {
		panic("failed to initialize SDL: " + err.Error())
	}
	defer sdl.Quit()

	fmt.Println("SDL Version:", sdl.GetVersion())

	// Load display settings from settings.json (separate from scene.json)
	displaySettings := settings.Load("settings.json")
	startWidth := displaySettings.WindowWidth
	startHeight := displaySettings.WindowHeight
	startFullscreen := displaySettings.Fullscreen
	pixelScale := displaySettings.PixelScale

	configWatcher := &ConfigWatcher{path: "scene.json"}

	// Create window
	win, err := window.New(window.Config{
		Title:  "Construct - City Block",
		Width:  startWidth,
		Height: startHeight,
	})
	if err != nil {
		panic("failed to create window: " + err.Error())
	}
	defer win.Destroy()

	if startFullscreen {
		if err := win.SetFullscreen(true); err != nil {
			fmt.Println("Warning: could not set fullscreen:", err)
		}
	}

	fmt.Println("GPU Driver:", win.Device().Driver())

	// Enable relative mouse mode for FPS camera
	err = win.SetRelativeMouseMode(true)
	if err != nil {
		fmt.Println("Warning: could not enable relative mouse mode:", err)
	}

	// Create renderer
	rend, err := renderer.New(win)
	if err != nil {
		panic("failed to create renderer: " + err.Error())
	}
	defer rend.Destroy()

	// Set initial offscreen resolution from window size and pixel scale
	offW := uint32(win.Width() / pixelScale)
	offH := uint32(win.Height() / pixelScale)
	if offW < 1 {
		offW = 1
	}
	if offH < 1 {
		offH = 1
	}
	rend.SetOffscreenResolution(offW, offH)

	// Create pause menu with SDL3-queried resolutions
	resolutions := win.DisplayModes()
	pauseMenu := ui.NewPauseMenu(rend, pixelScale, resolutions)
	startResIdx := 0
	for i, res := range resolutions {
		if res.W == startWidth && res.H == startHeight {
			startResIdx = i
			break
		}
	}
	startPSIdx := 0
	for i, v := range ui.PixelScales {
		if v == pixelScale {
			startPSIdx = i
			break
		}
	}
	startRDIdx := 0
	for i, v := range ui.RenderDistances {
		if float32(v) == displaySettings.RenderDistance {
			startRDIdx = i
			break
		}
	}
	pauseMenu.SetAppliedState(startFullscreen, startResIdx, startPSIdx, startRDIdx)
	defer pauseMenu.Destroy(rend)

	// Create admin mode
	adminMode := admin.New(rend, pixelScale)
	defer adminMode.Destroy(rend)

	// Create input handler
	inp := input.New()

	// Create camera
	cam := camera.New(float32(win.Width()) / float32(win.Height()))
	cam.Position = mgl32.Vec3{-10, 2, 2}
	cam.Yaw = -90 // Looking toward -Z
	cam.Far = displaySettings.RenderDistance

	// --- Create meshes ---
	type namedMesh struct {
		name string
		mesh *mesh.Mesh
	}
	var meshes []namedMesh

	createLitCube := func(name string, r, g, b uint8) *mesh.Mesh {
		m, err := mesh.NewLitCube(rend, r, g, b)
		if err != nil {
			panic("failed to create mesh " + name + ": " + err.Error())
		}
		meshes = append(meshes, namedMesh{name, m})
		return m
	}

	defer func() {
		for _, nm := range meshes {
			nm.mesh.Destroy(rend)
		}
	}()

	// --- Build the scene ---
	const gridCellSize float32 = 100.0
	world := newMapWorld(rend, gridCellSize)
	defer world.Destroy()

	poleMesh := createLitCube("pole", 30, 30, 30)
	housingMesh := createLitCube("housing", 20, 20, 20)
	greenOn := createLitCube("greenOn", 0, 255, 76)
	yellowOn := createLitCube("yellowOn", 255, 230, 0)
	redOn := createLitCube("redOn", 255, 25, 0)
	greenOff := createLitCube("greenOff", 0, 30, 9)
	yellowOff := createLitCube("yellowOff", 30, 27, 0)
	redOff := createLitCube("redOff", 30, 3, 0)

	if *useFetch {
		// --- Old path: fetch from APIs / cache ---
		world.reg = building.NewRegistry(rend, gridCellSize)
		world.scene = &scene.Scene{}
		loadFromAPIs(rend, world.reg, world.scene, &world.groundMeshes, &world.trafficSys, world.signMeshes, world.signWidths, &world.rampSys, minLat, minLon, maxLat, maxLon)
	} else {
		// --- New path: load from map data ---
		store, err := mapdata.Load(mapDataDir)
		if err != nil {
			fmt.Println("Warning: could not load map data, falling back to API fetch:", err)
			world.reg = building.NewRegistry(rend, gridCellSize)
			world.scene = &scene.Scene{}
			loadFromAPIs(rend, world.reg, world.scene, &world.groundMeshes, &world.trafficSys, world.signMeshes, world.signWidths, &world.rampSys, minLat, minLon, maxLat, maxLon)
		} else {
			world.store = store
			world.reg = building.NewRegistry(rend, gridCellSize)
			world.scene = &scene.Scene{}
			loadFromMapData(rend, store, world.reg, world.scene, &world.groundMeshes, &world.trafficSys, world.signMeshes, world.signWidths, &world.rampSys)
		}
	}

	// --- Build spatial grid for frustum culling ---
	world.grid = scene.NewSpatialGrid(world.scene.Objects, gridCellSize)
	fmt.Printf("Built spatial grid: %d objects\n", len(world.scene.Objects))

	// Build merged meshes per cell for efficient far rendering
	cellMeshes := world.reg.BuildCellMeshes()
	for key, cm := range cellMeshes {
		world.grid.CellMeshes[key] = &scene.CellMesh{Mesh: cm.Mesh, CellX: cm.CellX, CellZ: cm.CellZ}
	}
	fmt.Printf("Built %d merged cell meshes for far rendering\n", len(cellMeshes))

	// --- Snow particle system ---
	snowMesh := createLitCube("snow", 255, 255, 255)
	snowSys := snow.New(200)

	// --- Sky dome ---
	skyDome, err := mesh.NewSkyDome(rend, 400,
		15, 12, 22, // horizon: dark overcast gray
		160, 150, 175, // zenith: lighter cloud gray
	)
	if err != nil {
		panic("failed to create sky dome: " + err.Error())
	}
	defer skyDome.Destroy(rend)

	// --- Fallback ground plane (fills gaps in surface data) ---
	groundPlane, err := mesh.NewGroundPlane(rend, 5000, 70, 65, 60)
	if err != nil {
		panic("failed to create ground plane: " + err.Error())
	}
	defer groundPlane.Destroy(rend)

	// --- Build light uniforms ---
	// Slot 0 = headlamp (updated each frame), scene lights start at slot 1
	lightUniforms := renderer.LightUniforms{
		AmbientColor: mgl32.Vec4{0.05, 0.03, 0.02, 1.0},
		SunDirection: mgl32.Vec4{0.3, 0.8, 0.5, 0},    // default: angled from above-right
		SunColor:     mgl32.Vec4{1.0, 0.95, 0.9, 0.3}, // warm white, moderate intensity
	}
	headlampColor := mgl32.Vec4{1.0, 0.95, 0.8, 8.0} // rgb + intensity

	rebuildLightUniforms := func() {
		lightUniforms.LightColors[0] = headlampColor

		nScene := len(world.scene.Lights)
		if nScene > 511 {
			nScene = 511
		}
		for i := 0; i < nScene; i++ {
			l := world.scene.Lights[i]
			lightUniforms.LightPositions[i+1] = mgl32.Vec4{l.Position.X(), l.Position.Y(), l.Position.Z(), 0}
			lightUniforms.LightColors[i+1] = mgl32.Vec4{l.Color.X(), l.Color.Y(), l.Color.Z(), l.Intensity}
		}

		totalLights := 1 + nScene
		if world.trafficSys != nil {
			trafficLights := world.trafficSys.Lights()
			for i, tl := range trafficLights {
				slot := 1 + nScene + i
				if slot >= 512 {
					break
				}
				lightUniforms.LightPositions[slot] = mgl32.Vec4{tl.Position.X(), tl.Position.Y(), tl.Position.Z(), 0}
				lightUniforms.LightColors[slot] = mgl32.Vec4{tl.Color.X(), tl.Color.Y(), tl.Color.Z(), tl.Intensity}
				totalLights = slot + 1
			}
		}

		// Zero out stale light slots from previous load
		for i := totalLights; i < 512; i++ {
			lightUniforms.LightPositions[i] = mgl32.Vec4{}
			lightUniforms.LightColors[i] = mgl32.Vec4{}
		}

		lightUniforms.NumLights = mgl32.Vec4{float32(totalLights), 0, 0, 0}
	}
	rebuildLightUniforms()

	postProcess := renderer.PostProcessUniforms{
		Dither: mgl32.Vec4{1.0, 8.0, 0, 0},
		Tint:   mgl32.Vec4{1.08, 1.0, 0.85, 0},
	}

	// Hot-reload config
	applyConfig := func(cfg *SceneConfig) {
		postProcess.Dither = mgl32.Vec4{cfg.PostProcess.DitherStrength, cfg.PostProcess.ColorLevels, 0, 0}
		postProcess.Tint = mgl32.Vec4{cfg.PostProcess.TintR, cfg.PostProcess.TintG, cfg.PostProcess.TintB, 0}

		lightUniforms.AmbientColor = mgl32.Vec4{cfg.Lighting.AmbientR, cfg.Lighting.AmbientG, cfg.Lighting.AmbientB, 1.0}

		// Sun (directional light)
		if cfg.Lighting.SunIntensity > 0 {
			lightUniforms.SunDirection = mgl32.Vec4{cfg.Lighting.SunDirX, cfg.Lighting.SunDirY, cfg.Lighting.SunDirZ, 0}
			lightUniforms.SunColor = mgl32.Vec4{cfg.Lighting.SunR, cfg.Lighting.SunG, cfg.Lighting.SunB, cfg.Lighting.SunIntensity}
		}

		// Headlamp
		if cfg.Headlamp.Intensity > 0 {
			headlampColor = mgl32.Vec4{cfg.Headlamp.R, cfg.Headlamp.G, cfg.Headlamp.B, cfg.Headlamp.Intensity}
		}

		// Update street light color/intensity from config (positions stay fixed from scene builder)
		streetColor := mgl32.Vec3{cfg.Lighting.StreetLightR, cfg.Lighting.StreetLightG, cfg.Lighting.StreetLightB}
		streetIntensity := cfg.Lighting.StreetLightIntensity
		nScene := len(world.scene.Lights)
		for i := 0; i < nScene; i++ {
			world.scene.Lights[i].Color = streetColor
			world.scene.Lights[i].Intensity = streetIntensity
		}
		rebuildLightUniforms()

		// Derive offscreen resolution from current window size and pixel scale
		newOffW := uint32(win.Width() / pixelScale)
		newOffH := uint32(win.Height() / pixelScale)
		if newOffW < 1 {
			newOffW = 1
		}
		if newOffH < 1 {
			newOffH = 1
		}
		if err := rend.SetOffscreenResolution(newOffW, newOffH); err != nil {
			fmt.Println("Error changing resolution:", err)
		}

		// Snow parameters
		if cfg.Snow.Count > 0 {
			snowSys.SetCount(cfg.Snow.Count)
		}
		if cfg.Snow.FallSpeed > 0 {
			snowSys.SetFallSpeed(cfg.Snow.FallSpeed)
		}
		if cfg.Snow.WindStrength > 0 {
			snowSys.WindStrength = cfg.Snow.WindStrength
		}
		if cfg.Snow.ParticleSize > 0 {
			snowSys.SetParticleSize(cfg.Snow.ParticleSize)
		}

		// Ground textures
		if cfg.Textures.GroundScale > 0 {
			lightUniforms.TextureParams = mgl32.Vec4{cfg.Textures.GroundScale, cfg.Textures.GroundStrength, 0, 0}
		}

		// Fog
		if cfg.Fog.End > cfg.Fog.Start {
			lightUniforms.FogColor = mgl32.Vec4{cfg.Fog.R, cfg.Fog.G, cfg.Fog.B, 0}
			lightUniforms.FogParams = mgl32.Vec4{cfg.Fog.Start, cfg.Fog.End, cam.Far, 0}
		} else {
			// Even without fog, set render distance for far-plane fade
			lightUniforms.FogParams[2] = cam.Far
		}
	}

	// Apply config on first load (already loaded above for window init)
	configWatcher.modTime = time.Time{} // reset so it reloads
	if cfg, ok := configWatcher.Load(); ok {
		applyConfig(cfg)
	}

	// Map data watcher (only for local map data path, not -fetch)
	var mapWatcher *mapdata.MapWatcher
	if !*useFetch {
		mapWatcher = mapdata.NewWatcher(mapDataDir)
	}

	fmt.Println("\nControls:")
	fmt.Println("  WASD         - Move")
	fmt.Println("  Mouse        - Look around")
	fmt.Println("  Scroll Wheel - Throttle up/down")
	fmt.Println("  Tab          - Pause menu")
	fmt.Println("  ESC          - Quit")

	// Main loop
	lastTime := time.Now()
	running := true

	for running && !inp.ShouldQuit() {
		// Calculate delta time
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(lastTime).Seconds())
		lastTime = currentTime

		// Update input
		inp.Update()

		// Handle pause menu input
		wasActive := pauseMenu.IsActive()
		action := pauseMenu.HandleInput(inp)

		switch action {
		case ui.ActionQuit:
			running = false
			continue
		case ui.ActionApplySettings:
			fs := pauseMenu.PendingFullscreen()
			w, h := pauseMenu.PendingResolution()
			pixelScale = pauseMenu.PendingPixelScale()
			cam.Far = pauseMenu.PendingRenderDistance()
			if err := win.SetFullscreen(fs); err != nil {
				fmt.Println("Fullscreen error:", err)
			}
			if !fs {
				win.SetSize(w, h)
			}
			cam.AspectRatio = float32(win.Width()) / float32(win.Height())
			newOffW := uint32(win.Width() / pixelScale)
			newOffH := uint32(win.Height() / pixelScale)
			if newOffW < 1 {
				newOffW = 1
			}
			if newOffH < 1 {
				newOffH = 1
			}
			rend.SetOffscreenResolution(newOffW, newOffH)
			lightUniforms.FogParams[2] = cam.Far
			pauseMenu.ConfirmApply()
			settings.Save("settings.json", settings.Settings{
				WindowWidth:    w,
				WindowHeight:   h,
				Fullscreen:     fs,
				PixelScale:     pixelScale,
				RenderDistance:  cam.Far,
			})
		}

		// Toggle mouse mode on pause state change
		if pauseMenu.IsActive() && !wasActive {
			win.SetRelativeMouseMode(false)
		} else if !pauseMenu.IsActive() && wasActive {
			win.SetRelativeMouseMode(true)
		}

		// Admin mode toggle (backtick, only when pause menu inactive)
		if !pauseMenu.IsActive() && inp.IsKeyPressed(sdl.K_GRAVE) {
			adminMode.Toggle()
		}

		// Hot-reload config
		if cfg, ok := configWatcher.Load(); ok {
			applyConfig(cfg)
		}

		// Hot-reload map data
		if mapWatcher != nil && mapWatcher.Check() {
			fmt.Println("Reloading map data...")
			adminMode.ClearSelection()
			adminMode.ResetEditing()
			if err := world.Reload(mapDataDir); err != nil {
				fmt.Println("Map reload error:", err)
			} else {
				rebuildLightUniforms()
			}
		}

		if !pauseMenu.IsActive() {
			// Throttle: scroll wheel adjusts move speed
			if scroll := inp.ScrollY(); scroll != 0 {
				cam.MoveSpeed *= 1 + scroll*0.1
				if cam.MoveSpeed < 1 {
					cam.MoveSpeed = 1
				}
				if cam.MoveSpeed > 500 {
					cam.MoveSpeed = 500
				}
				fmt.Printf("Speed: %.0f\n", cam.MoveSpeed)
			}

			// Handle camera movement
			var forward, right, up float32

			// When admin mode is active and Cmd/Ctrl is held, skip S for backward
			// movement so Cmd+S can be used for save.
			cmdHeld := adminMode.IsActive() && (inp.IsKeyDown(sdl.K_LGUI) || inp.IsKeyDown(sdl.K_LCTRL))

			if inp.IsKeyDown(sdl.K_W) {
				forward = 1
			}
			if inp.IsKeyDown(sdl.K_S) && !cmdHeld {
				forward = -1
			}
			if inp.IsKeyDown(sdl.K_D) {
				right = 1
			}
			if inp.IsKeyDown(sdl.K_A) {
				right = -1
			}

			cam.Move(forward, right, up, deltaTime)

			// Handle camera look (always active, including admin mode)
			mouseDX, mouseDY := inp.MouseDelta()
			cam.Look(mouseDX, mouseDY)

			// Admin mode: raycast from screen center to select what you're looking at
			if adminMode.IsActive() {
				adminMode.Update(cam, world.grid, world.scene.Objects, world.reg, world.trafficSys)

				if world.store != nil {
					action := adminMode.HandleEdit(inp, world.reg, world.trafficSys,
						world.scene, world.grid, world.store)
					switch action {
					case admin.EditSave:
						// Remember selection for re-select
						var selBBL, selIntID string
						if sel := adminMode.Selection(); sel.Type == admin.EntityBuilding {
							b := world.reg.Get(sel.BuildingID)
							if b != nil {
								selBBL = b.BBL
							}
						} else if sel.Type == admin.EntitySignal && world.trafficSys != nil && sel.SignalIdx >= 0 {
							selIntID = world.trafficSys.Signals[sel.SignalIdx].ID
						}
						if err := adminMode.SaveDirty(world.store); err != nil {
							fmt.Println("Save error:", err)
						} else {
							if err := world.Reload(mapDataDir); err != nil {
								fmt.Println("Reload error:", err)
							} else {
								if mapWatcher != nil {
									mapWatcher.Reset()
								}
								rebuildLightUniforms()
								adminMode.ResetEditing()
								adminMode.Reselect(selBBL, selIntID, world.reg, world.trafficSys)
							}
						}
					case admin.EditDirty:
						rebuildLightUniforms()
					}
				}
			}

			// Update traffic lights
			if world.trafficSys != nil {
				world.trafficSys.Update(deltaTime)
				if world.trafficSys.Dirty() {
					rebuildLightUniforms()
				}
			}

			// Update snow particles (follow camera)
			snowSys.SetCenter(cam.Position.X(), cam.Position.Y(), cam.Position.Z())
			snowSys.Update(deltaTime)
		}

		// Update camera position and headlamp in light uniforms
		lightUniforms.CameraPos = mgl32.Vec4{cam.Position.X(), cam.Position.Y(), cam.Position.Z(), 0}
		lightUniforms.LightPositions[0] = lightUniforms.CameraPos
		// Keep render distance in sync for shader far-plane fade
		lightUniforms.FogParams[2] = cam.Far

		// Get view-projection matrix
		viewProj := cam.ViewProjectionMatrix()

		// --- Two-pass rendering ---

		// Pass 1: Render lit scene to offscreen texture
		cmdBuf, err := rend.BeginLitFrame()
		if err != nil {
			fmt.Println("Error beginning lit frame:", err)
			continue
		}

		scenePass := rend.BeginScenePass(cmdBuf)
		rend.PushLightUniforms(cmdBuf, lightUniforms)

		// Draw sky dome first (centered on camera, behind everything)
		{
			skyModel := mgl32.Translate3D(cam.Position.X(), cam.Position.Y(), cam.Position.Z())
			skyMVP := viewProj.Mul4(skyModel)
			rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
				VertexBuffer: skyDome.VertexBuffer,
				IndexBuffer:  skyDome.IndexBuffer,
				IndexCount:   skyDome.IndexCount,
				MVP:          skyMVP,
				Model:        skyModel,
				NoFog:        true,
				NoDepthWrite: true,
			})
		}

		// Fallback ground plane (follows camera, depth bias pushes it behind surface data)
		{
			gpModel := mgl32.Translate3D(cam.Position.X(), 0, cam.Position.Z())
			rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
				VertexBuffer: groundPlane.VertexBuffer,
				IndexBuffer:  groundPlane.IndexBuffer,
				IndexCount:   groundPlane.IndexCount,
				MVP:          viewProj.Mul4(gpModel),
				Model:        gpModel,
				DepthBias:    true,
			})
		}

		// Two-tier rendering:
		// - Near cells: individual objects with full lighting
		// - Far cells: single merged mesh per cell (1 draw call instead of N)
		// Decision is made per-cell to avoid gaps at the boundary.
		frustum := camera.ExtractFrustum(viewProj)
		cullDist := cam.Far * 0.90
		cullDistSq := cullDist * cullDist
		fadeStart := cullDist * 0.80  // dithered fade-out begins here
		fadeRange := cullDist - fadeStart
		detailDistSq := cam.Far * 0.45 * cam.Far * 0.45 // near/far tier boundary

		// Track which cells are rendered as merged (far tier)
		farCellSet := make(map[uint64]bool)

		// Iterate all cells with merged meshes and decide per-cell
		allCells := world.grid.QueryCells(cam.Position.X(), cam.Position.Z(), cullDist)
		for _, key := range allCells {
			cellDistSq := world.grid.CellDistSq(key, cam.Position.X(), cam.Position.Z())
			if cellDistSq > cullDistSq {
				continue
			}
			if cellDistSq >= detailDistSq {
				// Far tier: draw merged mesh
				cm := world.grid.CellMeshes[key]
				ccx, ccz := world.grid.CellCenter(key)
				if !frustum.SphereVisible(mgl32.Vec3{ccx, 50, ccz}, gridCellSize) {
					continue
				}
				// Dithered fade-out near cull boundary
				cellDist := float32(math.Sqrt(float64(cellDistSq)))
				var fade float32
				if cellDist > fadeStart {
					fade = (cellDist - fadeStart) / fadeRange
					if fade > 1 {
						fade = 1
					}
				}
				identity := mgl32.Ident4()
				rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
					VertexBuffer: cm.Mesh.VertexBuffer,
					IndexBuffer:  cm.Mesh.IndexBuffer,
					IndexCount:   cm.Mesh.IndexCount,
					MVP:          viewProj,
					Model:        identity,
					Index32:      true,
					FadeFactor:   fade,
				})
				farCellSet[key] = true
			}
			// Near cells: individual objects rendered below
		}

		// Near tier: individual objects (skip objects in far-tier cells)
		nearby := world.grid.QueryRadius(cam.Position.X(), cam.Position.Z(), cullDist)
		for _, idx := range nearby {
			obj := world.scene.Objects[idx]
			if obj.Hidden {
				continue
			}
			dx := obj.Position.X() - cam.Position.X()
			dz := obj.Position.Z() - cam.Position.Z()
			distSq := dx*dx + dz*dz
			// Expand cull distance by bounding radius so long/thin
			// geometry (roads, parks) stays visible when partially in range.
			effectiveCull := cullDist + obj.Radius
			if distSq > effectiveCull*effectiveCull {
				continue
			}
			// Skip objects whose cell is handled by the far tier
			cellKey := world.grid.CellKeyFor(obj.Position.X(), obj.Position.Z())
			if farCellSet[cellKey] && obj.SurfaceType == 0 {
				continue
			}
			if obj.Radius > 0 && !frustum.SphereVisible(obj.Position, obj.Radius) {
				continue
			}
			// Dithered fade-out near cull boundary
			dist := float32(math.Sqrt(float64(distSq)))
			var fade float32
			if dist > fadeStart {
				fade = (dist - fadeStart) / fadeRange
				if fade > 1 {
					fade = 1
				}
			}
			model := mgl32.Translate3D(obj.Position.X(), obj.Position.Y(), obj.Position.Z())
			model = model.Mul4(mgl32.Scale3D(obj.Scale.X(), obj.Scale.Y(), obj.Scale.Z()))
			mvp := viewProj.Mul4(model)

			var highlight float32
			if adminMode.IsActive() && obj.BuildingID > 0 && world.reg != nil &&
				building.BuildingID(obj.BuildingID-1) == adminMode.SelectedBuildingID() {
				highlight = 1.0
			}

			rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
				VertexBuffer: obj.Mesh.VertexBuffer,
				IndexBuffer:  obj.Mesh.IndexBuffer,
				IndexCount:   obj.Mesh.IndexCount,
				MVP:          mvp,
				Model:        model,
				FadeFactor:   fade,
				SurfaceType:  obj.SurfaceType,
				Highlight:    highlight,
			})
		}

		// Draw snow particles (billboarded to face camera)
		camRight := cam.Right()
		camUp := cam.Up()
		for i := range snowSys.Particles {
			p := &snowSys.Particles[i]
			sx := p.Size
			sy := p.Size * p.Aspect
			r := camRight.Mul(sx)
			u := camUp.Mul(sy)
			f := camRight.Cross(camUp).Mul(p.Size * 0.05)
			model := mgl32.Mat4{
				r[0], r[1], r[2], 0,
				u[0], u[1], u[2], 0,
				f[0], f[1], f[2], 0,
				p.Pos[0], p.Pos[1], p.Pos[2], 1,
			}
			mvp := viewProj.Mul4(model)

			rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
				VertexBuffer: snowMesh.VertexBuffer,
				IndexBuffer:  snowMesh.IndexBuffer,
				IndexCount:   snowMesh.IndexCount,
				MVP:          mvp,
				Model:        model,
			})
		}

		// Draw traffic signals (pole + 2 directional signal heads per intersection)
		if world.trafficSys != nil {
			var sigHighlight float32
			drawBox := func(m *mesh.Mesh, x, y, z float32) {
				model := mgl32.Translate3D(x, y, z)
				model = model.Mul4(mgl32.Scale3D(traffic.LightBoxSize, traffic.LightBoxSize, traffic.LightBoxSize))
				rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
					VertexBuffer: m.VertexBuffer,
					IndexBuffer:  m.IndexBuffer,
					IndexCount:   m.IndexCount,
					MVP:          viewProj.Mul4(model),
					Model:        model,
					Highlight:    sigHighlight,
				})
			}

			for sigIdx, sig := range world.trafficSys.Signals {
				x, z := sig.Position.X, sig.Position.Z

				// Frustum cull entire intersection (generous 10m radius)
				if !frustum.SphereVisible(mgl32.Vec3{x, traffic.PoleHeight / 2, z}, 10) {
					continue
				}

				sigHighlight = 0
				if adminMode.IsActive() && sigIdx == adminMode.SelectedSignalIdx() {
					sigHighlight = 1.0
				}

				// Pole (one per intersection)
				poleModel := mgl32.Translate3D(x, traffic.PoleHeight/2, z)
				poleModel = poleModel.Mul4(mgl32.Scale3D(traffic.PoleWidth, traffic.PoleHeight, traffic.PoleWidth))
				rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
					VertexBuffer: poleMesh.VertexBuffer,
					IndexBuffer:  poleMesh.IndexBuffer,
					IndexCount:   poleMesh.IndexCount,
					MVP:          viewProj.Mul4(poleModel),
					Model:        poleModel,
					Highlight:    sigHighlight,
				})

				// Two directional signal heads per intersection
				heads := sig.Heads()
				for _, h := range heads {
					// Forward direction for this head
					sinA := float32(math.Sin(float64(h.Angle)))
					cosA := float32(math.Cos(float64(h.Angle)))

					// Housing (dark box behind lights, blocks side/rear view)
					hModel := mgl32.Translate3D(h.X, traffic.HousingY, h.Z)
					hModel = hModel.Mul4(mgl32.HomogRotate3DY(h.Angle))
					hModel = hModel.Mul4(mgl32.Scale3D(traffic.HousingWidth, traffic.HousingHeight, traffic.HousingDepth))
					rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
						VertexBuffer: housingMesh.VertexBuffer,
						IndexBuffer:  housingMesh.IndexBuffer,
						IndexCount:   housingMesh.IndexCount,
						MVP:          viewProj.Mul4(hModel),
						Model:        hModel,
						Highlight:    sigHighlight,
					})

					// Light cubes offset forward from housing
					fwd := traffic.LightForward
					lx := h.X + sinA*fwd
					lz := h.Z + cosA*fwd
					if h.Phase == traffic.Red {
						drawBox(redOn, lx, traffic.RedY, lz)
					} else {
						drawBox(redOff, lx, traffic.RedY, lz)
					}
					if h.Phase == traffic.Yellow {
						drawBox(yellowOn, lx, traffic.YellowY, lz)
					} else {
						drawBox(yellowOff, lx, traffic.YellowY, lz)
					}
					if h.Phase == traffic.Green {
						drawBox(greenOn, lx, traffic.GreenY, lz)
					} else {
						drawBox(greenOff, lx, traffic.GreenY, lz)
					}
				}

				// Street name signs (stacked on the pole, not on the heads)
				drawSign := func(name string, y, angle float32) {
					sm, ok := world.signMeshes[name]
					if !ok {
						return
					}
					model := mgl32.Translate3D(x, y, z).Mul4(mgl32.HomogRotate3DY(angle))
					rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
						VertexBuffer: sm.VertexBuffer,
						IndexBuffer:  sm.IndexBuffer,
						IndexCount:   sm.IndexCount,
						MVP:          viewProj.Mul4(model),
						Model:        model,
					})
				}
				if sig.Street1 != "" {
					drawSign(sig.Street1, traffic.SignY1, sig.DirAngle+math.Pi/2)
				}
				if sig.Street2 != "" {
					drawSign(sig.Street2, traffic.SignY2, sig.DirAngle+math.Pi)
				}
			}
		}

		// Draw pedestrian ramps
		if world.rampSys != nil {
			rm := world.rampSys.Mesh
			for _, r := range world.rampSys.Ramps {
				rx, rz := r.Position.X, r.Position.Z
				// Frustum cull with 2m radius
				if !frustum.SphereVisible(mgl32.Vec3{rx, 0.1, rz}, 2) {
					continue
				}
				// Distance cull
				dx := rx - cam.Position.X()
				dz := rz - cam.Position.Z()
				if dx*dx+dz*dz > cullDistSq {
					continue
				}
				model := mgl32.Translate3D(rx, 0.07, rz).
					Mul4(mgl32.HomogRotate3DY(r.DirAngle)).
					Mul4(mgl32.Scale3D(r.Width, 1.0, r.Length))
				rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
					VertexBuffer: rm.VertexBuffer,
					IndexBuffer:  rm.IndexBuffer,
					IndexCount:   rm.IndexCount,
					MVP:          viewProj.Mul4(model),
					Model:        model,
				})
			}
		}

		rend.EndScenePass(scenePass)

		// Pass 2: Post-process to swapchain
		swapchain, err := cmdBuf.WaitAndAcquireGPUSwapchainTexture(win.Handle())
		if err != nil {
			fmt.Println("Error acquiring swapchain:", err)
			rend.EndLitFrame(cmdBuf)
			continue
		}

		if swapchain != nil {
			rend.RunPostProcess(cmdBuf, swapchain.Texture, postProcess)
			if pauseMenu.IsActive() {
				pauseMenu.Render(rend, cmdBuf, swapchain.Texture, win.Width(), win.Height())
			}
			if adminMode.IsActive() && !pauseMenu.IsActive() {
				adminMode.Render(rend, cmdBuf, swapchain.Texture, win.Width(), win.Height())
			}
		}

		rend.EndLitFrame(cmdBuf)
	}

	fmt.Println("\nGoodbye!")
}
