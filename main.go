package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"time"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/camera"
	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/ground"
	"github.com/anthonyrego/construct/pkg/input"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/sign"
	"github.com/anthonyrego/construct/pkg/snow"
	"github.com/anthonyrego/construct/pkg/traffic"
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

func main() {
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

	// Load initial config for window settings
	startWidth := defaultWindowWidth
	startHeight := defaultWindowHeight
	startFullscreen := false
	pixelScale := defaultPixelScale

	configWatcher := &ConfigWatcher{path: "scene.json"}
	if cfg, ok := configWatcher.Load(); ok {
		if cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
			startWidth = cfg.WindowWidth
			startHeight = cfg.WindowHeight
		}
		startFullscreen = cfg.Fullscreen
		if cfg.PixelScale > 0 {
			pixelScale = cfg.PixelScale
		}
	}

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

	// Create input handler
	inp := input.New()

	// Create camera
	cam := camera.New(float32(win.Width()) / float32(win.Height()))
	cam.Position = mgl32.Vec3{-10, 2, 2}
	cam.Yaw = -90 // Looking toward -Z

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

	var groundMeshes []*mesh.Mesh
	signMeshes := make(map[string]*mesh.Mesh)
	signWidths := make(map[string]float32)

	defer func() {
		for _, nm := range meshes {
			nm.mesh.Destroy(rend)
		}
		for _, gm := range groundMeshes {
			gm.Destroy(rend)
		}
		for _, sm := range signMeshes {
			sm.Destroy(rend)
		}
	}()

	// --- Build the scene ---
	s := &scene.Scene{}

	// --- Fetch and extrude NYC building footprints ---
	minLat, minLon, maxLat, maxLon := 40.700, -74.020, 40.747, -73.970 // Below 30th St, Manhattan
	footprints, proj, err := geojson.FetchFootprints(minLat, minLon, maxLat, maxLon, 50000)
	if err != nil {
		fmt.Println("Warning: could not fetch buildings:", err)
	} else {
		fmt.Printf("Fetched %d building footprints\n", len(footprints))
	}

	// Enrich with PLUTO data for building style/color
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

	const gridCellSize float32 = 100.0
	reg := building.NewRegistry(rend, gridCellSize)
	defer reg.Destroy()

	count := reg.Ingest(footprints)
	fmt.Printf("Registered %d buildings\n", count)

	for _, b := range reg.Buildings() {
		s.Add(scene.Object{
			Mesh:       b.Mesh,
			Position:   b.Position,
			Scale:      mgl32.Vec3{1, 1, 1},
			Radius:     b.Radius,
			BuildingID: uint32(b.ID),
		})
	}

	// --- Fetch and flatten ground surfaces ---
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
				groundMeshes = append(groundMeshes, m)
				s.Add(scene.Object{Mesh: m, Position: pos, Scale: mgl32.Vec3{1, 1, 1}, Radius: radius, SurfaceType: int(se.surfType) + 1})
			}
		}
	}

	// --- Fetch traffic signal locations ---
	var trafficSys *traffic.System
	poleMesh := createLitCube("pole", 30, 30, 30)
	housingMesh := createLitCube("housing", 20, 20, 20)
	// Lit (active) light meshes
	greenOn := createLitCube("greenOn", 0, 255, 76)
	yellowOn := createLitCube("yellowOn", 255, 230, 0)
	redOn := createLitCube("redOn", 255, 25, 0)
	// Dim (inactive) light meshes
	greenOff := createLitCube("greenOff", 0, 30, 9)
	yellowOff := createLitCube("yellowOff", 30, 27, 0)
	redOff := createLitCube("redOff", 30, 3, 0)

	if proj != nil {
		// Fetch street centerlines for curb-edge offset and street name lookup
		var streets []geojson.StreetSegment
		segs, err := geojson.FetchStreetSegments(traffic.CenterlineDataset, minLat, minLon, maxLat, maxLon, 50000, proj)
		if err != nil {
			fmt.Println("Warning: could not fetch centerlines:", err)
		} else {
			fmt.Printf("Fetched %d street centerline segments\n", len(segs))
			streets = segs
		}

		// Fetch traffic signal positions from OpenStreetMap
		signalPts, err := geojson.FetchOSMTrafficSignals(minLat, minLon, maxLat, maxLon, proj)
		if err != nil {
			fmt.Println("Warning: could not fetch OSM traffic signals:", err)
		} else {
			fmt.Printf("Fetched %d traffic signals from OpenStreetMap\n", len(signalPts))
		}

		if len(signalPts) > 0 {
			trafficSys = traffic.NewFromPoints(signalPts, 2.0, streets)

			// Create sign meshes for unique street names
			for _, sig := range trafficSys.Signals {
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

	// --- Build spatial grid for frustum culling ---
	grid := scene.NewSpatialGrid(s.Objects, gridCellSize)
	fmt.Printf("Built spatial grid: %d objects\n", len(s.Objects))

	// Build merged meshes per cell for efficient far rendering
	cellMeshes := reg.BuildCellMeshes()
	for key, cm := range cellMeshes {
		grid.CellMeshes[key] = &scene.CellMesh{Mesh: cm.Mesh, CellX: cm.CellX, CellZ: cm.CellZ}
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

	nScene := len(s.Lights)
	if nScene > 511 {
		nScene = 511
	}

	rebuildLightUniforms := func() {
		lightUniforms.LightColors[0] = headlampColor

		for i := 0; i < nScene; i++ {
			l := s.Lights[i]
			lightUniforms.LightPositions[i+1] = mgl32.Vec4{l.Position.X(), l.Position.Y(), l.Position.Z(), 0}
			lightUniforms.LightColors[i+1] = mgl32.Vec4{l.Color.X(), l.Color.Y(), l.Color.Z(), l.Intensity}
		}

		totalLights := 1 + nScene
		if trafficSys != nil {
			trafficLights := trafficSys.Lights()
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
		for i := 0; i < nScene; i++ {
			s.Lights[i].Color = streetColor
			s.Lights[i].Intensity = streetIntensity
		}
		rebuildLightUniforms()

		// Window / fullscreen
		if err := win.SetFullscreen(cfg.Fullscreen); err != nil {
			fmt.Println("Fullscreen error:", err)
		}
		if !cfg.Fullscreen && cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
			win.SetSize(cfg.WindowWidth, cfg.WindowHeight)
		}

		// Derive offscreen resolution from pixel scale
		scale := cfg.PixelScale
		if scale < 1 {
			scale = defaultPixelScale
		}
		newOffW := uint32(win.Width() / scale)
		newOffH := uint32(win.Height() / scale)
		if newOffW < 1 {
			newOffW = 1
		}
		if newOffH < 1 {
			newOffH = 1
		}
		if err := rend.SetOffscreenResolution(newOffW, newOffH); err != nil {
			fmt.Println("Error changing resolution:", err)
		}

		// Update camera aspect ratio to match
		cam.AspectRatio = float32(win.Width()) / float32(win.Height())

		// Render distance (drives camera far plane, spatial grid query, and frustum culling)
		if cfg.RenderDistance > 0 {
			cam.Far = cfg.RenderDistance
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

	fmt.Println("\nControls:")
	fmt.Println("  WASD         - Move")
	fmt.Println("  Mouse        - Look around")
	fmt.Println("  Scroll Wheel - Throttle up/down")
	fmt.Println("  ESC          - Quit")

	// Main loop
	lastTime := time.Now()

	for !inp.ShouldQuit() {
		// Calculate delta time
		currentTime := time.Now()
		deltaTime := float32(currentTime.Sub(lastTime).Seconds())
		lastTime = currentTime

		// Update input
		inp.Update()

		// Hot-reload config
		if cfg, ok := configWatcher.Load(); ok {
			applyConfig(cfg)
		}

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

		if inp.IsKeyDown(sdl.K_W) {
			forward = 1
		}
		if inp.IsKeyDown(sdl.K_S) {
			forward = -1
		}
		if inp.IsKeyDown(sdl.K_D) {
			right = 1
		}
		if inp.IsKeyDown(sdl.K_A) {
			right = -1
		}

		cam.Move(forward, right, up, deltaTime)

		// Handle camera look
		mouseDX, mouseDY := inp.MouseDelta()
		cam.Look(mouseDX, mouseDY)

		// Update traffic lights
		if trafficSys != nil {
			trafficSys.Update(deltaTime)
			if trafficSys.Dirty() {
				rebuildLightUniforms()
			}
		}

		// Update snow particles (follow camera)
		snowSys.SetCenter(cam.Position.X(), cam.Position.Y(), cam.Position.Z())
		snowSys.Update(deltaTime)

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
		allCells := grid.QueryCells(cam.Position.X(), cam.Position.Z(), cullDist)
		for _, key := range allCells {
			cellDistSq := grid.CellDistSq(key, cam.Position.X(), cam.Position.Z())
			if cellDistSq > cullDistSq {
				continue
			}
			if cellDistSq >= detailDistSq {
				// Far tier: draw merged mesh
				cm := grid.CellMeshes[key]
				ccx, ccz := grid.CellCenter(key)
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
		nearby := grid.QueryRadius(cam.Position.X(), cam.Position.Z(), cullDist)
		for _, idx := range nearby {
			obj := s.Objects[idx]
			dx := obj.Position.X() - cam.Position.X()
			dz := obj.Position.Z() - cam.Position.Z()
			distSq := dx*dx + dz*dz
			if distSq > cullDistSq {
				continue
			}
			// Skip objects whose cell is handled by the far tier
			cellKey := grid.CellKeyFor(obj.Position.X(), obj.Position.Z())
			if farCellSet[cellKey] {
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

			rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
				VertexBuffer: obj.Mesh.VertexBuffer,
				IndexBuffer:  obj.Mesh.IndexBuffer,
				IndexCount:   obj.Mesh.IndexCount,
				MVP:          mvp,
				Model:        model,
				FadeFactor:   fade,
				SurfaceType:  obj.SurfaceType,
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
		if trafficSys != nil {
			drawBox := func(m *mesh.Mesh, x, y, z float32) {
				model := mgl32.Translate3D(x, y, z)
				model = model.Mul4(mgl32.Scale3D(traffic.LightBoxSize, traffic.LightBoxSize, traffic.LightBoxSize))
				rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
					VertexBuffer: m.VertexBuffer,
					IndexBuffer:  m.IndexBuffer,
					IndexCount:   m.IndexCount,
					MVP:          viewProj.Mul4(model),
					Model:        model,
				})
			}

			for _, sig := range trafficSys.Signals {
				x, z := sig.Position.X, sig.Position.Z

				// Frustum cull entire intersection (generous 10m radius)
				if !frustum.SphereVisible(mgl32.Vec3{x, traffic.PoleHeight / 2, z}, 10) {
					continue
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
					sm, ok := signMeshes[name]
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
		}

		rend.EndLitFrame(cmdBuf)
	}

	fmt.Println("\nGoodbye!")
}
