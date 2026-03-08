package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/camera"
	"github.com/anthonyrego/construct/pkg/input"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
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
	PixelScale   int  `json:"pixelScale"`
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
	} `json:"lighting"`
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
	cam.Position = mgl32.Vec3{0, 2, 2}
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

	body0 := createLitCube("body0", 120, 75, 55)
	body1 := createLitCube("body1", 105, 80, 60)
	body2 := createLitCube("body2", 95, 65, 50)
	body3 := createLitCube("body3", 130, 85, 65)
	stoopMesh := createLitCube("stoop", 140, 130, 120)
	corniceMesh := createLitCube("cornice", 110, 95, 80)
	poleMesh := createLitCube("pole", 40, 42, 45)
	lanternMesh := createLitCube("lantern", 255, 230, 180)
	streetMesh := createLitCube("street", 50, 48, 45)
	sidewalkMesh := createLitCube("sidewalk", 110, 105, 100)

	defer func() {
		for _, nm := range meshes {
			nm.mesh.Destroy(rend)
		}
	}()

	bodyMeshes := []*mesh.Mesh{body0, body1, body2, body3}
	heights := [12]float32{8, 10, 9, 11, 8.5, 10.5, 7.5, 9.5, 8, 10.5, 9, 11.5}

	// --- Build the scene ---
	s := &scene.Scene{}

	// 12 buildings per side
	for i := 0; i < 12; i++ {
		z := float32(-3.5) - float32(i)*3.0
		bodyMesh := bodyMeshes[i%len(bodyMeshes)]
		h := heights[i]

		// Left side (center X = -5.0, stoop faces +X toward street)
		leftObjs := scene.Brownstone(bodyMesh, stoopMesh, corniceMesh, mgl32.Vec3{-5.0, 0, z}, h, 1.0)
		s.Add(leftObjs...)

		// Right side (center X = 5.0, stoop faces -X toward street)
		rightObjs := scene.Brownstone(bodyMesh, stoopMesh, corniceMesh, mgl32.Vec3{5.0, 0, z}, h, -1.0)
		s.Add(rightObjs...)

		// Street lights every other building
		if i%2 == 0 {
			// Left sidewalk light (X = -2.75, lantern extends toward street +X)
			lObjs, lLight := scene.StreetLight(poleMesh, lanternMesh,
				mgl32.Vec3{-2.75, 0, z}, 1.0,
				mgl32.Vec3{1.0, 0.85, 0.5}, 4.0)
			s.Add(lObjs...)
			s.AddLight(lLight)

			// Right sidewalk light (X = 2.75, lantern extends toward street -X)
			rObjs, rLight := scene.StreetLight(poleMesh, lanternMesh,
				mgl32.Vec3{2.75, 0, z}, -1.0,
				mgl32.Vec3{1.0, 0.85, 0.5}, 4.0)
			s.Add(rObjs...)
			s.AddLight(rLight)
		}
	}

	// Ground strips
	streetLength := float32(40.0)
	s.Add(scene.GroundStrip(streetMesh, 0, 4.0, streetLength))
	s.Add(scene.GroundStrip(sidewalkMesh, -2.75, 1.5, streetLength))
	s.Add(scene.GroundStrip(sidewalkMesh, 2.75, 1.5, streetLength))

	// --- Build light uniforms ---
	lightUniforms := renderer.LightUniforms{
		AmbientColor: mgl32.Vec4{0.05, 0.03, 0.02, 1.0},
	}
	n := len(s.Lights)
	if n > 16 {
		n = 16
	}
	lightUniforms.NumLights = mgl32.Vec4{float32(n), 0, 0, 0}
	for i := 0; i < n; i++ {
		l := s.Lights[i]
		lightUniforms.LightPositions[i] = mgl32.Vec4{l.Position.X(), l.Position.Y(), l.Position.Z(), 0}
		lightUniforms.LightColors[i] = mgl32.Vec4{l.Color.X(), l.Color.Y(), l.Color.Z(), l.Intensity}
	}

	postProcess := renderer.PostProcessUniforms{
		Dither: mgl32.Vec4{1.0, 8.0, 0, 0},
		Tint:   mgl32.Vec4{1.08, 1.0, 0.85, 0},
	}

	// Hot-reload config
	applyConfig := func(cfg *SceneConfig) {
		postProcess.Dither = mgl32.Vec4{cfg.PostProcess.DitherStrength, cfg.PostProcess.ColorLevels, 0, 0}
		postProcess.Tint = mgl32.Vec4{cfg.PostProcess.TintR, cfg.PostProcess.TintG, cfg.PostProcess.TintB, 0}

		lightUniforms.AmbientColor = mgl32.Vec4{cfg.Lighting.AmbientR, cfg.Lighting.AmbientG, cfg.Lighting.AmbientB, 1.0}

		// Update street light color/intensity from config (positions stay fixed from scene builder)
		streetColor := mgl32.Vec3{cfg.Lighting.StreetLightR, cfg.Lighting.StreetLightG, cfg.Lighting.StreetLightB}
		streetIntensity := cfg.Lighting.StreetLightIntensity
		for i := 0; i < n; i++ {
			lightUniforms.LightColors[i] = mgl32.Vec4{streetColor.X(), streetColor.Y(), streetColor.Z(), streetIntensity}
		}

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
	}

	// Apply config on first load (already loaded above for window init)
	configWatcher.modTime = time.Time{} // reset so it reloads
	if cfg, ok := configWatcher.Load(); ok {
		applyConfig(cfg)
	}

	fmt.Println("\nControls:")
	fmt.Println("  WASD  - Move")
	fmt.Println("  Mouse - Look around")
	fmt.Println("  Space - Move up")
	fmt.Println("  Shift - Move down")
	fmt.Println("  ESC   - Quit")

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
		if inp.IsKeyDown(sdl.K_SPACE) {
			up = 1
		}
		if inp.IsKeyDown(sdl.K_LSHIFT) || inp.IsKeyDown(sdl.K_RSHIFT) {
			up = -1
		}

		cam.Move(forward, right, up, deltaTime)

		// Handle camera look
		mouseDX, mouseDY := inp.MouseDelta()
		cam.Look(mouseDX, mouseDY)

		// Update camera position in light uniforms
		lightUniforms.CameraPos = mgl32.Vec4{cam.Position.X(), cam.Position.Y(), cam.Position.Z(), 0}

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

		for _, obj := range s.Objects {
			model := mgl32.Translate3D(obj.Position.X(), obj.Position.Y(), obj.Position.Z())
			model = model.Mul4(mgl32.Scale3D(obj.Scale.X(), obj.Scale.Y(), obj.Scale.Z()))
			mvp := viewProj.Mul4(model)

			rend.DrawLit(cmdBuf, scenePass, renderer.LitDrawCall{
				VertexBuffer: obj.Mesh.VertexBuffer,
				IndexBuffer:  obj.Mesh.IndexBuffer,
				IndexCount:   obj.Mesh.IndexCount,
				MVP:          mvp,
				Model:        model,
			})
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
