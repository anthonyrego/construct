# CLAUDE.md

## Project Overview

**Construct** is a cross-platform game library in Go using SDL3 for window management, input handling, and GPU rendering. The goal is to provide a simple foundation for building 3D games with a pixel art aesthetic.

## Tech Stack

- **Go 1.21+**
- **SDL3** via [Zyko0/go-sdl3](https://github.com/Zyko0/go-sdl3) (pure Go, no CGO required)
- **SDL3 GPU API** for rendering (abstracts Vulkan/Metal/D3D12)
- **mathgl** (go-gl/mathgl) for 3D math (vectors, matrices)

## Project Structure

```
construct/
├── main.go                 # Winter night demo scene
├── scene.json              # Hot-reloadable config (edit while running)
├── pkg/
│   ├── window/             # SDL3 window + GPU device + fullscreen
│   ├── input/              # Keyboard and mouse input handling
│   ├── renderer/           # Two-pass GPU rendering pipeline
│   ├── camera/             # First-person camera implementation
│   └── mesh/               # Mesh primitives (cube, lit cube, ground plane)
└── shaders/
    ├── embed.go            # Shader loading with go:embed
    └── compiled/
        ├── msl/            # Metal shaders (macOS)
        ├── spirv/          # Vulkan shaders (Linux, Windows)
        └── dxil/           # Direct3D 12 shaders (Windows)
```

## Build & Run

```bash
go build .
./construct

# Or directly:
go run .
```

## Architecture: Two-Pass Rendering

```
Pass 1 (Scene)                    Pass 2 (Post-Process)
┌─────────────────────┐          ┌──────────────────────────┐
│ Render lit geometry  │          │ Fullscreen triangle      │
│ to low-res offscreen │───────▶│ samples offscreen texture │
│ texture (with depth) │          │ applies dither + palette  │
│ + 4 point lights     │          │ outputs to swapchain      │
└─────────────────────┘          └──────────────────────────┘
```

Offscreen resolution = window size / `pixelScale`. At scale 4: each game pixel = 4x4 screen pixels.

### Lit Rendering (Pass 1)
- `LitVertex` type: position (float3) + normal (float3) + color (ubyte4_norm)
- Vertex uniforms: MVP + Model matrices
- Fragment uniforms: 4 point lights (position/color/intensity), ambient, camera pos
- Lambertian diffuse with distance attenuation
- Renders to R8G8B8A8_UNORM offscreen texture + D32_FLOAT depth

### Post-Process (Pass 2)
- Fullscreen triangle from vertex_id (no vertex buffer)
- Samples offscreen texture with nearest-neighbor sampler (chunky pixels)
- 4x4 Bayer ordered dithering
- Posterization to N color levels per channel
- Warm color tint (configurable per-channel multipliers)

## Shader System

Shaders are pre-compiled and embedded via `go:embed`. The system auto-selects format by GPU backend:
- **SPIRV** - Vulkan (Linux, Windows)
- **MSL** - Metal (macOS, iOS)
- **DXIL** - Direct3D 12 (Windows)

Current shaders:
- `PositionColorTransform.vert` / `SolidColor.frag` — Original unlit pipeline (retained)
- `Lit.vert` / `Lit.frag` — Lit scene rendering with point lights (MSL only currently)
- `PostProcess.vert` / `PostProcess.frag` — Pixel art post-processing (MSL only currently)

## scene.json — Hot-Reloadable Config

Edit while the app is running; changes apply instantly on save.

```json
{
  "windowWidth": 1280,
  "windowHeight": 720,
  "fullscreen": false,
  "pixelScale": 4,
  "postProcess": {
    "ditherStrength": 0.985,
    "colorLevels": 8.0,
    "tintR": 1.08,
    "tintG": 1.0,
    "tintB": 0.85
  },
  "lighting": {
    "ambientR": 0.06, "ambientG": 0.04, "ambientB": 0.03,
    "lights": [
      { "x": -2, "y": 3, "z": -3, "r": 1.0, "g": 0.8, "b": 0.4, "intensity": 5.0 }
    ]
  }
}
```

Key parameters:
- `pixelScale` — Controls pixel chunkiness. Offscreen = window / scale. Stays consistent across resolutions.
- `ditherStrength` — 0 = smooth, 1 = full Bayer dithering
- `colorLevels` — 2 = extreme posterization, 8 = default, 256 = smooth
- `tintR/G/B` — Per-channel color multipliers (1.0 = neutral)
- `lights` — Up to 4 point lights with position, color, and intensity

## Key Patterns

### Two-Pass Render Loop
```go
cmdBuf := rend.BeginLitFrame()
scenePass := rend.BeginScenePass(cmdBuf)
  rend.PushLightUniforms(cmdBuf, lights)
  rend.DrawLit(cmdBuf, scenePass, LitDrawCall{...})
rend.EndScenePass(scenePass)
swapchain := cmdBuf.WaitAndAcquireGPUSwapchainTexture(win.Handle())
rend.RunPostProcess(cmdBuf, swapchain.Texture, postProcessUniforms)
rend.EndLitFrame(cmdBuf)
```

### Original Unlit Render Loop (retained, not used in demo)
```go
cmdBuf, renderPass, err := renderer.BeginFrame()
if renderPass != nil {
    renderer.Draw(cmdBuf, renderPass, DrawCall{...})
    renderer.EndFrame(cmdBuf, renderPass)
}
```

### Mesh Types
- `NewCube(r, r, g, b)` — Unlit vertex-color cube (original)
- `NewLitCube(r, r, g, b)` — Cube with face normals for lighting
- `NewGroundPlane(r, size, r, g, b)` — Flat plane at Y=0 with up normal

### Camera
FPS-style yaw/pitch camera. ViewProjection matrix computed each frame. Aspect ratio updates when window/fullscreen changes.

### Window
Supports windowed and fullscreen modes. `SetFullscreen(bool)` toggles desktop fullscreen. `SetSize(w, h)` for windowed resize.

## Demo Controls

- **WASD** - Move
- **Mouse** - Look around
- **Space** - Move up
- **Shift** - Move down
- **ESC** - Quit
