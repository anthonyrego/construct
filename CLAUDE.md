# CLAUDE.md

## Project Overview

**Construct** is a cross-platform game library in Go using SDL3 for window management, input handling, and GPU rendering. The goal is to provide a simple foundation for building 3D games with a pixel art aesthetic. Currently renders real NYC building footprints fetched from open data.

## Tech Stack

- **Go 1.24+**
- **SDL3** via [Zyko0/go-sdl3](https://github.com/Zyko0/go-sdl3) (pure Go, no CGO required)
- **SDL3 GPU API** for rendering (abstracts Vulkan/Metal/D3D12)
- **mathgl** (go-gl/mathgl) for 3D math (vectors, matrices)

## Project Structure

```
construct/
├── main.go                 # NYC building footprint demo scene
├── scene.json              # Hot-reloadable config (edit while running)
├── build/                  # Build output (gitignored)
├── .cache/                 # Cached API responses (gitignored)
├── pkg/
│   ├── geojson/            # NYC SODA API fetcher, GeoJSON parser, coordinate projection
│   ├── building/           # Polygon extruder (footprint → 3D mesh) + ear-clip triangulation
│   ├── window/             # SDL3 window + GPU device + fullscreen
│   ├── input/              # Keyboard and mouse input handling
│   ├── renderer/           # Two-pass GPU rendering pipeline
│   ├── camera/             # First-person camera implementation
│   ├── mesh/               # Mesh primitives (cube, lit cube, ground plane)
│   └── snow/               # Snow particle system (follows camera)
└── shaders/
    ├── embed.go            # Shader loading with go:embed
    └── compiled/
        ├── msl/            # Metal shaders (macOS)
        ├── spirv/          # Vulkan shaders (Linux, Windows)
        └── dxil/           # Direct3D 12 shaders (Windows)
```

## Build & Run

```bash
go build -o build/construct .
./build/construct
```

## Architecture: Two-Pass Rendering

```
Pass 1 (Scene)                    Pass 2 (Post-Process)
┌─────────────────────┐          ┌──────────────────────────┐
│ Render lit geometry  │          │ Fullscreen triangle      │
│ to low-res offscreen │───────▶│ samples offscreen texture │
│ texture (with depth) │          │ applies dither + palette  │
│ + up to 64 lights    │          │ outputs to swapchain      │
└─────────────────────┘          └──────────────────────────┘
```

Offscreen resolution = window size / `pixelScale`. At scale 4: each game pixel = 4x4 screen pixels.

### Lit Rendering (Pass 1)
- `LitVertex` type: position (float3) + normal (float3) + color (ubyte4_norm)
- Vertex uniforms: MVP + Model matrices
- Fragment uniforms: up to 64 point lights (position/color/intensity), ambient, camera pos
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

## Data Pipeline

### GeoJSON Fetcher (`pkg/geojson`)
- Fetches building footprints from NYC SODA API (`data.cityofnewyork.us`, dataset `5zhs-2jue`)
- Caches raw JSON responses to `.cache/` — subsequent runs load from disk
- Parses Polygon and MultiPolygon geometries
- Projects WGS84 lat/lon to local meters via equirectangular approximation
- Enforces CCW winding on outer rings, converts height from feet to meters

### Building Extruder (`pkg/building`)
- Extrudes 2D footprint polygons into 3D `LitVertex` meshes
- Walls: 4 vertices per edge with outward normals
- Roof: ear-clipping triangulation with upward normal
- Positions are centroid-relative; centroid returned for scene placement

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
    "tintR": 1.08, "tintG": 1.0, "tintB": 0.85
  },
  "lighting": {
    "ambientR": 0.05, "ambientG": 0.03, "ambientB": 0.02,
    "streetLightR": 1.0, "streetLightG": 0.85, "streetLightB": 0.5,
    "streetLightIntensity": 3.2
  },
  "headlamp": {
    "r": 1.0, "g": 0.95, "b": 0.8, "intensity": 8.0
  },
  "snow": {
    "count": 9000, "fallSpeed": 1.2, "windStrength": 0.4, "particleSize": 0.04
  }
}
```

Key parameters:
- `pixelScale` — Controls pixel chunkiness. Offscreen = window / scale.
- `ditherStrength` — 0 = smooth, 1 = full Bayer dithering
- `colorLevels` — 2 = extreme posterization, 8 = default, 256 = smooth
- `tintR/G/B` — Per-channel color multipliers (1.0 = neutral)
- `headlamp` — Point light that follows the camera (color + intensity)
- `snow` — Particle count, fall speed, wind, and size

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

### Building Mesh Creation
```go
footprints, _ := geojson.FetchFootprints(minLat, minLon, maxLat, maxLon, limit)
for _, fp := range footprints {
    m, pos, _ := building.Extrude(rend, fp, r, g, b)
    scene.Add(scene.Object{Mesh: m, Position: pos, Scale: mgl32.Vec3{1,1,1}})
}
```

### Mesh Types
- `NewCube(r, r, g, b)` — Unlit vertex-color cube (original)
- `NewLitCube(r, r, g, b)` — Cube with face normals for lighting
- `NewGroundPlane(r, size, r, g, b)` — Flat plane at Y=0 with up normal
- `building.Extrude(r, footprint, r, g, b)` — Extruded polygon mesh from GeoJSON

### Camera
FPS-style yaw/pitch camera (far plane = 1000m). ViewProjection matrix computed each frame. Aspect ratio updates when window/fullscreen changes.

### Snow
Particle system that follows the camera in all 3 axes. Configurable count, fall speed, wind, and particle size via scene.json.

## Demo Controls

- **WASD** - Move
- **Mouse** - Look around
- **ESC** - Quit
