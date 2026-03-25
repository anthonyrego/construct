package main

import (
	"fmt"
	"math"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/admin"
	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/engine"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/physics"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/settings"
	"github.com/anthonyrego/construct/pkg/snow"
	"github.com/anthonyrego/construct/pkg/ui"
)

const gridCellSize float32 = 100.0

// NYCGame implements engine.Game for the NYC reconstruction.
type NYCGame struct {
	world      *MapWorld
	useFetch   bool
	mapDataDir string

	pauseMenu *ui.PauseMenu
	adminMode *admin.Mode

	snowSys     *snow.System
	snowMesh    *mesh.Mesh
	skyDome     *mesh.Mesh
	groundPlane *mesh.Mesh

	headlampColor mgl32.Vec4
	configWatcher *engine.ConfigWatcher
	mapWatcher    *mapdata.MapWatcher

	physWorld  *physics.World
	playerBody physics.Body
}

func (g *NYCGame) Init(e *engine.Engine) error {
	// Camera starting position
	e.Cam.Position = mgl32.Vec3{-10, 2, 2}
	e.Cam.Yaw = -90

	// Pause menu
	resolutions := e.Win.DisplayModes()
	g.pauseMenu = ui.NewPauseMenu(e.Rend, e.PixelScale, resolutions)
	startResIdx := 0
	for i, res := range resolutions {
		if res.W == e.Win.Width() && res.H == e.Win.Height() {
			startResIdx = i
			break
		}
	}
	startPSIdx := 0
	for i, v := range ui.PixelScales {
		if v == e.PixelScale {
			startPSIdx = i
			break
		}
	}
	startRDIdx := 0
	for i, v := range ui.RenderDistances {
		if float32(v) == e.Cam.Far {
			startRDIdx = i
			break
		}
	}
	g.pauseMenu.SetAppliedState(e.Win.IsFullscreen(), startResIdx, startPSIdx, startRDIdx)

	// Admin mode
	g.adminMode = admin.New(e.Rend, e.PixelScale)

	// Snow
	snowMesh, err := mesh.NewLitCube(e.Rend, 255, 255, 255)
	if err != nil {
		return fmt.Errorf("snow mesh: %w", err)
	}
	g.snowMesh = snowMesh
	g.snowSys = snow.New(200)

	// Sky dome
	g.skyDome, err = mesh.NewSkyDome(e.Rend, 400,
		15, 12, 22,
		160, 150, 175,
	)
	if err != nil {
		return fmt.Errorf("sky dome: %w", err)
	}

	// Ground plane
	g.groundPlane, err = mesh.NewGroundPlane(e.Rend, 5000, 70, 65, 60)
	if err != nil {
		return fmt.Errorf("ground plane: %w", err)
	}

	// Build world
	minLat, minLon, maxLat, maxLon := 40.700, -74.020, 40.747, -73.970
	g.world = newMapWorld(e.Rend, gridCellSize)

	if g.useFetch {
		g.world.reg = building.NewRegistry(e.Rend, gridCellSize)
		g.world.scene = &scene.Scene{}
		loadFromAPIs(e.Rend, g.world.reg, g.world.scene, &g.world.groundMeshes, &g.world.trafficSys, g.world.doodads, minLat, minLon, maxLat, maxLon)
	} else {
		store, err := mapdata.Load(g.mapDataDir)
		if err != nil {
			fmt.Println("Warning: could not load map data, falling back to API fetch:", err)
			g.world.reg = building.NewRegistry(e.Rend, gridCellSize)
			g.world.scene = &scene.Scene{}
			loadFromAPIs(e.Rend, g.world.reg, g.world.scene, &g.world.groundMeshes, &g.world.trafficSys, g.world.doodads, minLat, minLon, maxLat, maxLon)
		} else {
			g.world.store = store
			g.world.reg = building.NewRegistry(e.Rend, gridCellSize)
			g.world.scene = &scene.Scene{}
			loadFromMapData(e.Rend, store, g.world.reg, g.world.scene, &g.world.groundMeshes, &g.world.trafficSys, g.world.doodads)
		}
	}

	// Build spatial grid + cell meshes
	g.world.grid = scene.NewSpatialGrid(g.world.scene.Objects, gridCellSize)
	fmt.Printf("Built spatial grid: %d objects\n", len(g.world.scene.Objects))
	cellMeshes := g.world.reg.BuildCellMeshes()
	for key, cm := range cellMeshes {
		g.world.grid.CellMeshes[key] = &scene.CellMesh{Mesh: cm.Mesh, CellX: cm.CellX, CellZ: cm.CellZ}
	}
	fmt.Printf("Built %d merged cell meshes for far rendering\n", len(cellMeshes))

	// Initialize physics
	g.playerBody = physics.Body{
		X:          e.Cam.Position.X(),
		Y:          e.Cam.Position.Y() - 1.6,
		Z:          e.Cam.Position.Z(),
		Radius:     0.3,
		Height:     1.8,
		Gravity:    20.0,
		StepHeight: 0.3,
	}
	g.physWorld = &physics.World{DefaultY: 0.0}
	g.rebuildPhysics()

	// Initialize light uniforms
	e.LightUniforms = renderer.LightUniforms{
		AmbientColor: mgl32.Vec4{0.05, 0.03, 0.02, 1.0},
		SunDirection: mgl32.Vec4{0.3, 0.8, 0.5, 0},
		SunColor:     mgl32.Vec4{1.0, 0.95, 0.9, 0.3},
	}
	g.headlampColor = mgl32.Vec4{1.0, 0.95, 0.8, 8.0}
	g.rebuildLightUniforms(e)

	// Initialize post-process
	e.PostProcess = renderer.PostProcessUniforms{
		Dither: mgl32.Vec4{1.0, 8.0, 0, 0},
		Tint:   mgl32.Vec4{1.08, 1.0, 0.85, 0},
	}

	// Config watcher — force initial load
	g.configWatcher = engine.NewConfigWatcher("scene.json")
	g.configWatcher.ForceReload()
	if cfg, ok := g.configWatcher.Check(); ok {
		g.applyConfig(e, cfg)
	}

	// Map data watcher
	if !g.useFetch {
		g.mapWatcher = mapdata.NewWatcher(g.mapDataDir)
	}

	fmt.Println("\nControls:")
	fmt.Println("  WASD         - Move")
	fmt.Println("  Mouse        - Look around")
	fmt.Println("  Scroll Wheel - Throttle up/down")
	fmt.Println("  Tab          - Pause menu")
	fmt.Println("  ESC          - Quit")

	return nil
}

func (g *NYCGame) Update(e *engine.Engine, dt float32) bool {
	// Pause menu input
	wasActive := g.pauseMenu.IsActive()
	action := g.pauseMenu.HandleInput(e.Input)

	switch action {
	case ui.ActionQuit:
		return false
	case ui.ActionApplySettings:
		fs := g.pauseMenu.PendingFullscreen()
		w, h := g.pauseMenu.PendingResolution()
		ps := g.pauseMenu.PendingPixelScale()
		rd := g.pauseMenu.PendingRenderDistance()
		e.ApplyDisplaySettings(fs, w, h, ps, rd)
		e.LightUniforms.FogParams[2] = e.Cam.Far
		g.pauseMenu.ConfirmApply()
		settings.Save("settings.json", settings.Settings{
			WindowWidth:   w,
			WindowHeight:  h,
			Fullscreen:    fs,
			PixelScale:    ps,
			RenderDistance: rd,
		})
	}

	// Toggle mouse mode on pause state change
	if g.pauseMenu.IsActive() && !wasActive {
		e.SetMouseMode(false)
	} else if !g.pauseMenu.IsActive() && wasActive {
		e.SetMouseMode(true)
	}

	// Admin toggle
	if !g.pauseMenu.IsActive() && e.Input.IsKeyPressed(sdl.K_GRAVE) {
		wasAdmin := g.adminMode.IsActive()
		g.adminMode.Toggle()
		// Returning from fly mode to physics mode: snap body to ground
		if wasAdmin && !g.adminMode.IsActive() {
			g.playerBody.X = e.Cam.Position.X()
			g.playerBody.Z = e.Cam.Position.Z()
			if gy, ok := g.physWorld.GroundHeight(g.playerBody.X, g.playerBody.Z); ok {
				g.playerBody.Y = gy
			} else {
				g.playerBody.Y = g.physWorld.DefaultY
			}
			g.playerBody.VelX = 0
			g.playerBody.VelY = 0
			g.playerBody.VelZ = 0
			g.playerBody.Grounded = true
			e.Cam.Position = mgl32.Vec3{g.playerBody.X, g.playerBody.Y + 1.6, g.playerBody.Z}
		}
	}

	// Hot-reload config
	if cfg, ok := g.configWatcher.Check(); ok {
		g.applyConfig(e, cfg)
	}

	// Hot-reload map data
	if g.mapWatcher != nil && g.mapWatcher.Check() {
		fmt.Println("Reloading map data...")
		g.adminMode.ClearSelection()
		g.adminMode.ResetEditing()
		if err := g.world.Reload(g.mapDataDir); err != nil {
			fmt.Println("Map reload error:", err)
		} else {
			g.rebuildLightUniforms(e)
			g.rebuildPhysics()
		}
	}

	if !g.pauseMenu.IsActive() {
		// Scroll wheel speed
		if scroll := e.Input.ScrollY(); scroll != 0 {
			e.Cam.MoveSpeed *= 1 + scroll*0.1
			if e.Cam.MoveSpeed < 1 {
				e.Cam.MoveSpeed = 1
			}
			if e.Cam.MoveSpeed > 500 {
				e.Cam.MoveSpeed = 500
			}
			fmt.Printf("Speed: %.0f\n", e.Cam.MoveSpeed)
		}

		if g.adminMode.IsActive() {
			// Admin fly mode (existing behavior)
			var forward, right, up float32
			cmdHeld := e.Input.IsKeyDown(sdl.K_LGUI) || e.Input.IsKeyDown(sdl.K_LCTRL)
			if e.Input.IsKeyDown(sdl.K_W) {
				forward = 1
			}
			if e.Input.IsKeyDown(sdl.K_S) && !cmdHeld {
				forward = -1
			}
			if e.Input.IsKeyDown(sdl.K_D) {
				right = 1
			}
			if e.Input.IsKeyDown(sdl.K_A) {
				right = -1
			}
			e.Cam.Move(forward, right, up, dt)
		} else {
			// Physics walking mode
			var forward, right float32
			if e.Input.IsKeyDown(sdl.K_W) {
				forward = 1
			}
			if e.Input.IsKeyDown(sdl.K_S) {
				forward = -1
			}
			if e.Input.IsKeyDown(sdl.K_D) {
				right = 1
			}
			if e.Input.IsKeyDown(sdl.K_A) {
				right = -1
			}

			// Yaw-only direction (ignore pitch so looking down doesn't push into ground)
			yawRad := float64(e.Cam.Yaw) * math.Pi / 180
			fwdX := float32(math.Cos(yawRad))
			fwdZ := float32(math.Sin(yawRad))
			rgtX := -fwdZ
			rgtZ := fwdX

			speed := e.Cam.MoveSpeed
			g.playerBody.VelX = (fwdX*forward + rgtX*right) * speed
			g.playerBody.VelZ = (fwdZ*forward + rgtZ*right) * speed

			// Jump
			if e.Input.IsKeyPressed(sdl.K_SPACE) && g.playerBody.Grounded {
				g.playerBody.VelY = 8.0
				g.playerBody.Grounded = false
			}

			g.physWorld.Step(&g.playerBody, dt)

			e.Cam.Position = mgl32.Vec3{
				g.playerBody.X,
				g.playerBody.Y + 1.6,
				g.playerBody.Z,
			}
		}

		mouseDX, mouseDY := e.Input.MouseDelta()
		e.Cam.Look(mouseDX, mouseDY)

		// Admin mode
		if g.adminMode.IsActive() {
			g.handleAdmin(e, dt)
		}

		// Traffic signals
		if g.world.trafficSys != nil {
			g.world.trafficSys.Update(dt)
			if g.world.trafficSys.Dirty() {
				g.rebuildLightUniforms(e)
			}
		}

		// Snow particles
		g.snowSys.SetCenter(e.Cam.Position.X(), e.Cam.Position.Y(), e.Cam.Position.Z())
		g.snowSys.Update(dt)
	}

	return true
}

func (g *NYCGame) Render(e *engine.Engine, frame renderer.RenderFrame) {
	viewProj := frame.ViewProj

	// Sky dome
	skyModel := mgl32.Translate3D(frame.CamPos.X(), frame.CamPos.Y(), frame.CamPos.Z())
	e.Rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
		VertexBuffer: g.skyDome.VertexBuffer,
		IndexBuffer:  g.skyDome.IndexBuffer,
		IndexCount:   g.skyDome.IndexCount,
		MVP:          viewProj.Mul4(skyModel),
		Model:        skyModel,
		NoFog:        true,
		NoDepthWrite: true,
	})

	// Ground plane
	gpModel := mgl32.Translate3D(frame.CamPos.X(), 0, frame.CamPos.Z())
	e.Rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
		VertexBuffer: g.groundPlane.VertexBuffer,
		IndexBuffer:  g.groundPlane.IndexBuffer,
		IndexCount:   g.groundPlane.IndexCount,
		MVP:          viewProj.Mul4(gpModel),
		Model:        gpModel,
		DepthBias:    true,
	})

	// Two-tier building rendering
	g.renderBuildings(e, frame)

	// Entity systems
	g.snowSys.Render(e.Rend, frame, g.snowMesh)

	highlightIdx := -1
	if g.adminMode.IsActive() {
		highlightIdx = g.adminMode.SelectedSignalIdx()
	}
	g.world.trafficSys.Render(e.Rend, frame, highlightIdx)

	for _, sys := range g.world.doodads {
		sys.Render(e.Rend, frame)
	}
}

func (g *NYCGame) Overlay(e *engine.Engine, cmdBuf *sdl.GPUCommandBuffer, target *sdl.GPUTexture) {
	if g.pauseMenu.IsActive() {
		g.pauseMenu.Render(e.Rend, cmdBuf, target, e.Win.Width(), e.Win.Height())
	}
	if g.adminMode.IsActive() && !g.pauseMenu.IsActive() {
		g.adminMode.Render(e.Rend, cmdBuf, target, e.Win.Width(), e.Win.Height())
	}
}

func (g *NYCGame) Destroy(e *engine.Engine) {
	g.pauseMenu.Destroy(e.Rend)
	g.adminMode.Destroy(e.Rend)
	g.world.Destroy()
	g.skyDome.Destroy(e.Rend)
	g.groundPlane.Destroy(e.Rend)
	g.snowMesh.Destroy(e.Rend)
}

// --- Private methods ---

func (g *NYCGame) rebuildLightUniforms(e *engine.Engine) {
	e.LightUniforms.LightColors[0] = g.headlampColor

	nScene := len(g.world.scene.Lights)
	if nScene > 511 {
		nScene = 511
	}
	for i := 0; i < nScene; i++ {
		l := g.world.scene.Lights[i]
		e.LightUniforms.LightPositions[i+1] = mgl32.Vec4{l.Position.X(), l.Position.Y(), l.Position.Z(), 0}
		e.LightUniforms.LightColors[i+1] = mgl32.Vec4{l.Color.X(), l.Color.Y(), l.Color.Z(), l.Intensity}
	}

	totalLights := 1 + nScene
	if g.world.trafficSys != nil {
		trafficLights := g.world.trafficSys.Lights()
		for i, tl := range trafficLights {
			slot := 1 + nScene + i
			if slot >= 512 {
				break
			}
			e.LightUniforms.LightPositions[slot] = mgl32.Vec4{tl.Position.X(), tl.Position.Y(), tl.Position.Z(), 0}
			e.LightUniforms.LightColors[slot] = mgl32.Vec4{tl.Color.X(), tl.Color.Y(), tl.Color.Z(), tl.Intensity}
			totalLights = slot + 1
		}
	}

	for i := totalLights; i < 512; i++ {
		e.LightUniforms.LightPositions[i] = mgl32.Vec4{}
		e.LightUniforms.LightColors[i] = mgl32.Vec4{}
	}

	e.LightUniforms.NumLights = mgl32.Vec4{float32(totalLights), 0, 0, 0}
}

func (g *NYCGame) applyConfig(e *engine.Engine, cfg *engine.SceneConfig) {
	e.PostProcess.Dither = mgl32.Vec4{cfg.PostProcess.DitherStrength, cfg.PostProcess.ColorLevels, 0, 0}
	e.PostProcess.Tint = mgl32.Vec4{cfg.PostProcess.TintR, cfg.PostProcess.TintG, cfg.PostProcess.TintB, 0}

	e.LightUniforms.AmbientColor = mgl32.Vec4{cfg.Lighting.AmbientR, cfg.Lighting.AmbientG, cfg.Lighting.AmbientB, 1.0}

	if cfg.Lighting.SunIntensity > 0 {
		e.LightUniforms.SunDirection = mgl32.Vec4{cfg.Lighting.SunDirX, cfg.Lighting.SunDirY, cfg.Lighting.SunDirZ, 0}
		e.LightUniforms.SunColor = mgl32.Vec4{cfg.Lighting.SunR, cfg.Lighting.SunG, cfg.Lighting.SunB, cfg.Lighting.SunIntensity}
	}

	if cfg.Headlamp.Intensity > 0 {
		g.headlampColor = mgl32.Vec4{cfg.Headlamp.R, cfg.Headlamp.G, cfg.Headlamp.B, cfg.Headlamp.Intensity}
	}

	streetColor := mgl32.Vec3{cfg.Lighting.StreetLightR, cfg.Lighting.StreetLightG, cfg.Lighting.StreetLightB}
	streetIntensity := cfg.Lighting.StreetLightIntensity
	nScene := len(g.world.scene.Lights)
	for i := 0; i < nScene; i++ {
		g.world.scene.Lights[i].Color = streetColor
		g.world.scene.Lights[i].Intensity = streetIntensity
	}
	g.rebuildLightUniforms(e)

	newOffW := uint32(e.Win.Width() / e.PixelScale)
	newOffH := uint32(e.Win.Height() / e.PixelScale)
	if newOffW < 1 {
		newOffW = 1
	}
	if newOffH < 1 {
		newOffH = 1
	}
	if err := e.Rend.SetOffscreenResolution(newOffW, newOffH); err != nil {
		fmt.Println("Error changing resolution:", err)
	}

	if cfg.Snow.Count > 0 {
		g.snowSys.SetCount(cfg.Snow.Count)
	}
	if cfg.Snow.FallSpeed > 0 {
		g.snowSys.SetFallSpeed(cfg.Snow.FallSpeed)
	}
	if cfg.Snow.WindStrength > 0 {
		g.snowSys.WindStrength = cfg.Snow.WindStrength
	}
	if cfg.Snow.ParticleSize > 0 {
		g.snowSys.SetParticleSize(cfg.Snow.ParticleSize)
	}

	if cfg.Textures.GroundScale > 0 {
		e.LightUniforms.TextureParams = mgl32.Vec4{cfg.Textures.GroundScale, cfg.Textures.GroundStrength, 0, 0}
	}

	if cfg.Fog.End > cfg.Fog.Start {
		e.LightUniforms.FogColor = mgl32.Vec4{cfg.Fog.R, cfg.Fog.G, cfg.Fog.B, 0}
		e.LightUniforms.FogParams = mgl32.Vec4{cfg.Fog.Start, cfg.Fog.End, e.Cam.Far, 0}
	} else {
		e.LightUniforms.FogParams[2] = e.Cam.Far
	}
}

func (g *NYCGame) handleAdmin(e *engine.Engine, dt float32) {
	g.adminMode.Update(e.Cam, g.world.grid, g.world.scene.Objects, g.world.reg, g.world.trafficSys,
		g.world.doodads, e.Input)

	if g.world.store != nil {
		action := g.adminMode.HandleEdit(e.Input, g.world.reg, g.world.trafficSys,
			g.world.scene, g.world.grid, g.world.store, g.world.doodads)
		switch action {
		case admin.EditSave:
			var selBBL, selIntID, selDoodadID, selDoodadType string
			if sel := g.adminMode.Selection(); sel.Type == admin.EntityBuilding {
				b := g.world.reg.Get(sel.BuildingID)
				if b != nil {
					selBBL = b.BBL
				}
			} else if sel.Type == admin.EntitySignal && g.world.trafficSys != nil && sel.SignalIdx >= 0 {
				selIntID = g.world.trafficSys.Signals[sel.SignalIdx].ID
			} else if sel.Type == admin.EntityTree {
				selDoodadID = sel.DoodadID
				selDoodadType = "tree"
			} else if sel.Type == admin.EntityHydrant {
				selDoodadID = sel.DoodadID
				selDoodadType = "hydrant"
			}
			if err := g.adminMode.SaveDirty(g.world.store); err != nil {
				fmt.Println("Save error:", err)
			} else {
				if err := g.world.Reload(g.mapDataDir); err != nil {
					fmt.Println("Reload error:", err)
				} else {
					if g.mapWatcher != nil {
						g.mapWatcher.Reset()
					}
					g.rebuildLightUniforms(e)
					g.rebuildPhysics()
					g.adminMode.ResetEditing()
					g.adminMode.Reselect(selBBL, selIntID, selDoodadID, selDoodadType,
						g.world.reg, g.world.trafficSys, g.world.doodads)
				}
			}
		case admin.EditDirty:
			g.rebuildLightUniforms(e)
		}
	}
}

func (g *NYCGame) renderBuildings(e *engine.Engine, frame renderer.RenderFrame) {
	viewProj := frame.ViewProj
	cullDist := frame.CullDist
	cullDistSq := frame.CullDistSq
	fadeStart := frame.FadeStart
	fadeRange := frame.FadeRange
	frustum := frame.Frustum
	detailDistSq := e.Cam.Far * 0.45 * e.Cam.Far * 0.45

	farCellSet := make(map[uint64]bool)

	allCells := g.world.grid.QueryCells(frame.CamPos.X(), frame.CamPos.Z(), cullDist)
	for _, key := range allCells {
		cellDistSq := g.world.grid.CellDistSq(key, frame.CamPos.X(), frame.CamPos.Z())
		if cellDistSq > cullDistSq {
			continue
		}
		if cellDistSq >= detailDistSq {
			cm := g.world.grid.CellMeshes[key]
			ccx, ccz := g.world.grid.CellCenter(key)
			if !frustum.SphereVisible(mgl32.Vec3{ccx, 50, ccz}, gridCellSize) {
				continue
			}
			cellDist := float32(math.Sqrt(float64(cellDistSq)))
			var fade float32
			if cellDist > fadeStart {
				fade = (cellDist - fadeStart) / fadeRange
				if fade > 1 {
					fade = 1
				}
			}
			identity := mgl32.Ident4()
			e.Rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
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
	}

	nearby := g.world.grid.QueryRadius(frame.CamPos.X(), frame.CamPos.Z(), cullDist)
	for _, idx := range nearby {
		obj := g.world.scene.Objects[idx]
		if obj.Hidden {
			continue
		}
		dx := obj.Position.X() - frame.CamPos.X()
		dz := obj.Position.Z() - frame.CamPos.Z()
		distSq := dx*dx + dz*dz
		effectiveCull := cullDist + obj.Radius
		if distSq > effectiveCull*effectiveCull {
			continue
		}
		cellKey := g.world.grid.CellKeyFor(obj.Position.X(), obj.Position.Z())
		if farCellSet[cellKey] && obj.SurfaceType == 0 {
			continue
		}
		if obj.Radius > 0 && !frustum.SphereVisible(obj.Position, obj.Radius) {
			continue
		}
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
		if g.adminMode.IsActive() && obj.BuildingID > 0 && g.world.reg != nil &&
			building.BuildingID(obj.BuildingID-1) == g.adminMode.SelectedBuildingID() {
			highlight = 1.0
		}

		e.Rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
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
}

// --- Physics callbacks ---

type surfaceEntry struct {
	outer    [][2]float32
	y        float32
	centerX  float32
	centerZ  float32
	radiusSq float32
}

type surfaceIndex struct {
	entries  []surfaceEntry
	cells    map[uint64][]int
	cellSize float32
}

func (si *surfaceIndex) cellKey(x, z float32) uint64 {
	cx := int32(x / si.cellSize)
	cz := int32(z / si.cellSize)
	if x < 0 {
		cx--
	}
	if z < 0 {
		cz--
	}
	return uint64(uint32(cx))<<32 | uint64(uint32(cz))
}

func (si *surfaceIndex) query(x, z float32) []int {
	return si.cells[si.cellKey(x, z)]
}

func buildSurfaceIndex(store *mapdata.Store) *surfaceIndex {
	surfaceYValues := map[string]float32{
		"road":     0.06,
		"sidewalk": 0.20,
		"park":     0.01,
	}

	si := &surfaceIndex{
		cells:    make(map[uint64][]int),
		cellSize: 100.0,
	}

	for typeName, y := range surfaceYValues {
		sfd, ok := store.Surfaces[typeName]
		if !ok {
			continue
		}
		for _, sp := range sfd.Polygons {
			if !sp.Visible || len(sp.Outer) < 3 {
				continue
			}
			// Compute centroid and bounding radius
			var cx, cz float32
			for _, p := range sp.Outer {
				cx += p[0]
				cz += p[1]
			}
			n := float32(len(sp.Outer))
			cx /= n
			cz /= n
			var maxDistSq float32
			for _, p := range sp.Outer {
				dx := p[0] - cx
				dz := p[1] - cz
				if d := dx*dx + dz*dz; d > maxDistSq {
					maxDistSq = d
				}
			}

			idx := len(si.entries)
			si.entries = append(si.entries, surfaceEntry{
				outer:    sp.Outer,
				y:        y,
				centerX:  cx,
				centerZ:  cz,
				radiusSq: maxDistSq + 2, // small padding
			})

			// Insert into all cells the bounding box overlaps
			radius := float32(math.Sqrt(float64(maxDistSq))) + 1
			minCX := int32((cx - radius) / si.cellSize)
			maxCX := int32((cx + radius) / si.cellSize)
			minCZ := int32((cz - radius) / si.cellSize)
			maxCZ := int32((cz + radius) / si.cellSize)
			if cx-radius < 0 {
				minCX--
			}
			if cz-radius < 0 {
				minCZ--
			}
			for gx := minCX; gx <= maxCX; gx++ {
				for gz := minCZ; gz <= maxCZ; gz++ {
					key := uint64(uint32(gx))<<32 | uint64(uint32(gz))
					si.cells[key] = append(si.cells[key], idx)
				}
			}
		}
	}

	return si
}

func (g *NYCGame) rebuildPhysics() {
	if g.world.store == nil {
		return
	}
	si := buildSurfaceIndex(g.world.store)

	g.physWorld.GroundHeight = func(x, z float32) (float32, bool) {
		bestY := float32(-999)
		found := false
		for _, ei := range si.query(x, z) {
			e := &si.entries[ei]
			dx := x - e.centerX
			dz := z - e.centerZ
			if dx*dx+dz*dz > e.radiusSq {
				continue
			}
			if physics.PointInPolygon(x, z, e.outer) {
				if !found || e.y > bestY {
					bestY = e.y
					found = true
				}
			}
		}
		return bestY, found
	}

	w := g.world
	g.physWorld.Collision = func(oldX, oldZ, newX, newZ, radius float32) (float32, float32, bool) {
		grid := w.grid
		objects := w.scene.Objects
		reg := w.reg
		if grid == nil || reg == nil {
			return newX, newZ, false
		}

		collided := false
		slidX, slidZ := newX, newZ

		// Multiple iterations to resolve corners and narrow gaps between buildings.
		// Each pass may push the player into a new overlap that the next pass fixes.
		for iter := 0; iter < 3; iter++ {
			hitThisPass := false

		nearby := grid.QueryRadius(slidX, slidZ, radius+20)
		for _, idx := range nearby {
			obj := objects[idx]
			if obj.BuildingID == 0 || obj.Hidden {
				continue
			}
			bid := building.BuildingID(obj.BuildingID - 1)
			b := reg.Get(bid)
			if b == nil || len(b.Footprint.Rings) == 0 {
				continue
			}

			// Bounding sphere reject
			dx := slidX - b.Position.X()
			dz := slidZ - b.Position.Z()
			maxD := b.Radius + radius + 1
			if dx*dx+dz*dz > maxD*maxD {
				continue
			}

			// Test player circle against building footprint outer ring (CCW winding).
			outer := b.Footprint.Rings[0]
			n := len(outer)
			if n < 3 {
				continue
			}

			// Convert to [][2]float32 for PointInPolygon
			poly := make([][2]float32, n)
			for k := 0; k < n; k++ {
				poly[k] = [2]float32{outer[k].X, outer[k].Z}
			}

			if physics.PointInPolygon(slidX, slidZ, poly) {
				// Player center is inside the polygon — find nearest edge
				// and push out along its outward normal.
				bestDist := float32(math.MaxFloat32)
				var bestPushX, bestPushZ float32
				for i := 0; i < n; i++ {
					j := (i + 1) % n
					ax, az := outer[i].X, outer[i].Z
					bx, bz := outer[j].X, outer[j].Z
					edx, edz := bx-ax, bz-az
					lenSq := edx*edx + edz*edz
					if lenSq < 1e-8 {
						continue
					}
					t := ((slidX-ax)*edx + (slidZ-az)*edz) / lenSq
					if t < 0 {
						t = 0
					}
					if t > 1 {
						t = 1
					}
					cx, cz := ax+t*edx, az+t*edz
					ddx, ddz := slidX-cx, slidZ-cz
					dist := float32(math.Sqrt(float64(ddx*ddx + ddz*ddz)))
					if dist < bestDist {
						bestDist = dist
						eLen := float32(math.Sqrt(float64(lenSq)))
						nx, nz := edz/eLen, -edx/eLen
						bestPushX = cx + nx*(radius+0.01) - slidX
						bestPushZ = cz + nz*(radius+0.01) - slidZ
					}
				}
				slidX += bestPushX
				slidZ += bestPushZ
				collided = true
				hitThisPass = true
			} else {
				// Player center is outside — test edges from the outside only.
				for i := 0; i < n; i++ {
					j := (i + 1) % n
					ax, az := outer[i].X, outer[i].Z
					bx, bz := outer[j].X, outer[j].Z
					edx, edz := bx-ax, bz-az
					signedDist := edz*(slidX-ax) - edx*(slidZ-az)
					if signedDist < 0 {
						continue
					}
					pushX, pushZ, hit := physics.SegmentCircleIntersect(
						slidX, slidZ, radius,
						ax, az, bx, bz,
					)
					if hit {
						slidX += pushX
						slidZ += pushZ
						collided = true
						hitThisPass = true
					}
				}
			}
		}

		if !hitThisPass {
			break // no collisions this pass, position is stable
		}
		} // end iteration loop

		return slidX, slidZ, collided
	}
}
