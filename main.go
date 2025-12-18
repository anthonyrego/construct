package main

import (
	"fmt"
	"time"

	"github.com/Zyko0/go-sdl3/bin/binsdl"
	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/camera"
	"github.com/anthonyrego/construct/pkg/input"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/window"
)

const (
	windowWidth  = 1280
	windowHeight = 720
)

type CubeInstance struct {
	Mesh     *mesh.Mesh
	Position mgl32.Vec3
	Scale    float32
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

	// Create window
	win, err := window.New(window.Config{
		Title:  "Construct - First Person Demo",
		Width:  windowWidth,
		Height: windowHeight,
	})
	if err != nil {
		panic("failed to create window: " + err.Error())
	}
	defer win.Destroy()

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

	// Create input handler
	inp := input.New()

	// Create camera
	cam := camera.New(float32(windowWidth) / float32(windowHeight))
	cam.Position = mgl32.Vec3{0, 1, 5}

	// Create cubes with different colors and positions
	cubes := []CubeInstance{}

	// Red cube
	redCube, err := mesh.NewCube(rend, 255, 80, 80)
	if err != nil {
		panic("failed to create red cube: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: redCube, Position: mgl32.Vec3{-3, 0.5, -2}, Scale: 1.0})

	// Green cube
	greenCube, err := mesh.NewCube(rend, 80, 255, 80)
	if err != nil {
		panic("failed to create green cube: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: greenCube, Position: mgl32.Vec3{0, 0.5, -5}, Scale: 1.0})

	// Blue cube
	blueCube, err := mesh.NewCube(rend, 80, 80, 255)
	if err != nil {
		panic("failed to create blue cube: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: blueCube, Position: mgl32.Vec3{3, 0.5, -2}, Scale: 1.0})

	// Yellow cube (far)
	yellowCube, err := mesh.NewCube(rend, 255, 255, 80)
	if err != nil {
		panic("failed to create yellow cube: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: yellowCube, Position: mgl32.Vec3{0, 0.5, -10}, Scale: 2.0})

	// Cyan cube (left)
	cyanCube, err := mesh.NewCube(rend, 80, 255, 255)
	if err != nil {
		panic("failed to create cyan cube: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: cyanCube, Position: mgl32.Vec3{-6, 1, -8}, Scale: 1.5})

	// Magenta cube (right)
	magentaCube, err := mesh.NewCube(rend, 255, 80, 255)
	if err != nil {
		panic("failed to create magenta cube: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: magentaCube, Position: mgl32.Vec3{6, 0.5, -6}, Scale: 1.0})

	// Floor (gray, large flat cube)
	floorCube, err := mesh.NewCube(rend, 100, 100, 100)
	if err != nil {
		panic("failed to create floor: " + err.Error())
	}
	cubes = append(cubes, CubeInstance{Mesh: floorCube, Position: mgl32.Vec3{0, -0.5, -5}, Scale: 20.0})

	defer func() {
		for _, cube := range cubes {
			cube.Mesh.Destroy(rend)
		}
	}()

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

		// Get view-projection matrix
		viewProj := cam.ViewProjectionMatrix()

		// Begin rendering
		cmdBuf, renderPass, err := rend.BeginFrame()
		if err != nil {
			fmt.Println("Error beginning frame:", err)
			continue
		}

		if renderPass != nil {
			// Draw all cubes
			for _, cube := range cubes {
				// Create model matrix (translate + scale)
				model := mgl32.Translate3D(cube.Position.X(), cube.Position.Y(), cube.Position.Z())
				model = model.Mul4(mgl32.Scale3D(cube.Scale, cube.Scale, cube.Scale))

				// Calculate MVP
				mvp := viewProj.Mul4(model)

				rend.Draw(cmdBuf, renderPass, renderer.DrawCall{
					VertexBuffer: cube.Mesh.VertexBuffer,
					IndexBuffer:  cube.Mesh.IndexBuffer,
					IndexCount:   cube.Mesh.IndexCount,
					Transform:    mvp,
				})
			}

			rend.EndFrame(cmdBuf, renderPass)
		}
	}

	fmt.Println("\nGoodbye!")
}
