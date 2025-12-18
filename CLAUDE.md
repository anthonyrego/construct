# CLAUDE.md

## Project Overview

**Construct** is a cross-platform game library in Go using SDL3 for window management, input handling, and GPU rendering. The goal is to provide a simple foundation for building 3D games.

## Tech Stack

- **Go 1.21+**
- **SDL3** via [Zyko0/go-sdl3](https://github.com/Zyko0/go-sdl3) (pure Go, no CGO required)
- **SDL3 GPU API** for rendering (abstracts Vulkan/Metal/D3D12)
- **mathgl** (go-gl/mathgl) for 3D math (vectors, matrices)

## Project Structure

```
construct/
├── main.go                 # Demo entry point (FPS camera + cubes)
├── pkg/
│   ├── window/             # SDL3 window + GPU device wrapper
│   ├── input/              # Keyboard and mouse input handling
│   ├── renderer/           # SDL3 GPU rendering pipeline
│   ├── camera/             # First-person camera implementation
│   └── mesh/               # Mesh primitives (cube)
└── shaders/
    ├── embed.go            # Shader loading with go:embed
    └── compiled/           # Pre-compiled shaders (SPIRV/MSL/DXIL)
```

## Build & Run

```bash
go build .
./construct

# Or directly:
go run .
```

## Shader System

Shaders are pre-compiled and embedded. The system automatically selects the correct format based on the GPU backend:
- **SPIRV** - Vulkan (Linux, Windows)
- **MSL** - Metal (macOS, iOS)
- **DXIL** - Direct3D 12 (Windows)

Current shaders:
- `PositionColorTransform.vert` - Vertex shader with MVP matrix uniform
- `SolidColor.frag` - Simple color passthrough fragment shader

## Key Patterns

### Rendering Loop
```go
cmdBuf, renderPass, err := renderer.BeginFrame()
if renderPass != nil {
    renderer.Draw(cmdBuf, renderPass, DrawCall{...})
    renderer.EndFrame(cmdBuf, renderPass)
}
```

### Camera
The camera uses yaw/pitch angles for FPS-style look. ViewProjection matrix is computed each frame and passed as uniform to vertex shader.

### Input
Poll-based input with key state tracking. Relative mouse mode enabled for FPS camera control.

## Demo Controls

- **WASD** - Move
- **Mouse** - Look around
- **Space** - Move up
- **Shift** - Move down
- **ESC** - Quit
