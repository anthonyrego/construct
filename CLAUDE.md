# CLAUDE.md

## Project Overview

**Construct** is a cross-platform game library in Go using SDL3 for window management, input handling, and GPU rendering. The goal is to provide a simple foundation for building 3D games with a pixel art aesthetic. Currently renders a real-time explorable 3D reconstruction of lower Manhattan using NYC open data — building footprints, roadbeds, sidewalks, parks, traffic signals, and street signs.

## Tech Stack

- **Go 1.24+**
- **SDL3** via [Zyko0/go-sdl3](https://github.com/Zyko0/go-sdl3) (pure Go, no CGO required)
- **SDL3 GPU API** for rendering (abstracts Vulkan/Metal/D3D12)
- **mathgl** (go-gl/mathgl) for 3D math (vectors, matrices)

## Project Structure

```
construct/
├── main.go                 # NYC reconstruction demo scene
├── Makefile                # Build commands
├── scene.json              # Hot-reloadable config (edit while running)
├── build/                  # Build output (gitignored)
├── .cache/                 # Cached API responses (gitignored)
├── docs/                   # Reference docs (NYC data APIs, reconstruction plan)
├── pkg/
│   ├── building/           # Building registry, polygon extruder (footprint → 3D mesh), ear-clip triangulation, PLUTO-based styling
│   ├── camera/             # First-person camera + reversed-Z projection + frustum culling
│   ├── engine/             # (placeholder)
│   ├── geojson/            # NYC SODA API fetcher, GeoJSON parser, coordinate projection, PLUTO enrichment, OSM traffic signals
│   ├── ground/             # Surface flattener (roadbeds, sidewalks, parks → flat meshes with Y-offset layering)
│   ├── input/              # Keyboard and mouse input handling
│   ├── mesh/               # Mesh primitives (cube, lit cube, ground plane, sky dome)
│   ├── renderer/           # Two-pass GPU rendering pipeline (reversed-Z depth, multiple pipeline variants)
│   ├── scene/              # Scene graph + spatial grid for frustum culling and two-tier LOD rendering
│   ├── sign/               # Street name sign mesh generator (text → geometry)
│   ├── snow/               # Snow particle system (billboarded, follows camera)
│   ├── traffic/            # Traffic signal system (pole + heads + phased light cycling)
│   └── window/             # SDL3 window + GPU device + fullscreen
└── shaders/
    ├── embed.go            # Shader loading with go:embed
    └── compiled/
        ├── msl/            # Metal shaders (macOS)
        ├── spirv/          # Vulkan shaders (Linux, Windows)
        └── dxil/           # Direct3D 12 shaders (Windows)
```

## Build & Run

```bash
make build    # compile to build/construct
make run      # build and run
make clean    # remove build output
make vet      # go vet ./...
make fmt      # gofmt -w .
```

## Architecture: Two-Pass Rendering

```
Pass 1 (Scene)                    Pass 2 (Post-Process)
┌─────────────────────┐          ┌──────────────────────────┐
│ Render lit geometry  │          │ Fullscreen triangle      │
│ to low-res offscreen │───────▶ │ samples offscreen texture │
│ texture (with depth) │          │ applies dither + palette  │
│ + up to 512 lights   │          │ outputs to swapchain      │
└─────────────────────┘          └──────────────────────────┘
```

Offscreen resolution = window size / `pixelScale`. At scale 3: each game pixel = 3x3 screen pixels.

### Lit Rendering (Pass 1)
- `LitVertex` type: position (float3) + normal (float3) + color (ubyte4_norm)
- Vertex uniforms: MVP + Model matrices + flags (fog skip)
- Fragment uniforms: up to 512 point lights, directional sun light, ambient, fog, camera pos
- Lambertian diffuse with distance attenuation for point lights
- Directional sun light (no attenuation)
- Distance fog with configurable start/end + smooth far-plane fade
- Renders to R8G8B8A8_UNORM offscreen texture + D32_FLOAT reversed-Z depth

### Reversed-Z Depth Buffer
Uses reversed-Z projection (near=1.0, far=0.0) with GREATER_OR_EQUAL compare for dramatically better depth precision at large distances. Eliminates z-fighting between coplanar ground surfaces.

### Post-Process (Pass 2)
- Fullscreen triangle from vertex_id (no vertex buffer)
- Samples offscreen texture with nearest-neighbor sampler (chunky pixels)
- 4x4 Bayer ordered dithering
- Posterization to N color levels per channel
- Warm color tint (configurable per-channel multipliers)

### Two-Tier LOD Rendering
- **Near tier**: individual building meshes with full detail (from `BuildingRegistry`)
- **Far tier**: merged cell meshes with per-building span tracking (one draw call per 100m grid cell)
- Tier decision made per-cell via spatial grid to avoid boundary gaps
- Frustum culling on both tiers via extracted frustum planes

### Pipeline Variants
The renderer maintains four GPU pipeline variants:
- **Default lit** — standard depth test + write (GREATER_OR_EQUAL)
- **No depth write** — depth test only, for sky dome (renders behind everything)
- **Depth bias** — negative bias pushes ground plane behind coplanar surfaces
- **Post-process** — fullscreen triangle, no depth

## Shader System

Shaders are pre-compiled and embedded via `go:embed`. The system auto-selects format by GPU backend:
- **SPIRV** — Vulkan (Linux, Windows)
- **MSL** — Metal (macOS, iOS)
- **DXIL** — Direct3D 12 (Windows)

Current shaders:
- `PositionColorTransform.vert` / `SolidColor.frag` — Original unlit pipeline (retained)
- `Lit.vert` / `Lit.frag` — Lit scene rendering with point lights, sun, and fog
- `PostProcess.vert` / `PostProcess.frag` — Pixel art post-processing

## Data Pipeline

### GeoJSON Fetcher (`pkg/geojson`)
- Fetches building footprints from NYC SODA API (`data.cityofnewyork.us`, dataset `5zhs-2jue`)
- Fetches PLUTO lot data for building classification and styling
- Fetches roadbed, sidewalk, and park surface polygons
- Fetches street centerline segments for traffic signal placement
- Fetches traffic signal locations from OpenStreetMap (Overpass API)
- Caches all API responses to `.cache/` — subsequent runs load from disk
- Parses Polygon and MultiPolygon geometries
- Projects WGS84 lat/lon to local meters via equirectangular approximation
- Enforces CCW winding on outer rings, converts height from feet to meters

### Building Registry & Extruder (`pkg/building`)
- `Registry` is the single owner of all building data and GPU resources
- Each `Building` retains its identity (BBL, PLUTO metadata, footprint) and CPU-side `RawMesh` throughout its lifecycle
- Lookup by `BuildingID` or BBL; supports per-building mesh replacement and per-cell re-merging
- Extrudes 2D footprint polygons into 3D `LitVertex` meshes (walls + ear-clipped roof)
- PLUTO-based color styling (land use classification → building color)
- Merged cell meshes track per-building `BuildingSpan` (index offset/count) for future per-building operations

### Ground Surfaces (`pkg/ground`)
- Flattens surface polygons into horizontal meshes at Y=0
- Three surface types with distinct colors and Y-offsets: roadbed (0.01), sidewalk (0.05), park (0.10)

### Traffic Signals (`pkg/traffic`)
- Places traffic light poles at OSM-sourced intersection positions
- Snaps to nearest street centerline for curb-edge offset
- Two directional signal heads per intersection (oriented along cross streets)
- Phased light cycling (green → yellow → red) with staggered timing
- Emits point lights matching the active signal color

### Street Signs (`pkg/sign`)
- Generates 3D mesh geometry from street name strings
- Mounted on traffic signal poles at intersections

## scene.json — Hot-Reloadable Config

Edit while the app is running; changes apply instantly on save.

```json
{
  "windowWidth": 1280,
  "windowHeight": 720,
  "fullscreen": false,
  "pixelScale": 4,
  "renderDistance": 1500,
  "postProcess": {
    "ditherStrength": 0.985,
    "colorLevels": 8.0,
    "tintR": 1.08, "tintG": 1.0, "tintB": 0.85
  },
  "lighting": {
    "ambientR": 0.05, "ambientG": 0.03, "ambientB": 0.02,
    "streetLightR": 1.0, "streetLightG": 0.85, "streetLightB": 0.5,
    "streetLightIntensity": 3.2,
    "sunDirX": 0.3, "sunDirY": 0.8, "sunDirZ": 0.5,
    "sunR": 1.0, "sunG": 0.95, "sunB": 0.9, "sunIntensity": 0.5
  },
  "headlamp": {
    "r": 1.0, "g": 0.95, "b": 0.8, "intensity": 8.0
  },
  "snow": {
    "count": 2000, "fallSpeed": 1.2, "windStrength": 0.4, "particleSize": 0.04
  },
  "fog": {
    "r": 0.096, "g": 0.03, "b": 0.136,
    "start": 350, "end": 750
  }
}
```

Key parameters:
- `pixelScale` — Controls pixel chunkiness. Offscreen = window / scale.
- `renderDistance` — Far plane distance in meters (drives culling, fog fade, camera)
- `ditherStrength` — 0 = smooth, 1 = full Bayer dithering
- `colorLevels` — 2 = extreme posterization, 8 = default, 256 = smooth
- `tintR/G/B` — Per-channel color multipliers (1.0 = neutral)
- `headlamp` — Point light that follows the camera (color + intensity)
- `snow` — Particle count, fall speed, wind, and size
- `fog` — Distance fog color, start/end distances
- `lighting.sun*` — Directional sun light direction, color, and intensity

## Demo Controls

- **WASD** — Move
- **Mouse** — Look around
- **Scroll Wheel** — Adjust movement speed
- **ESC** — Quit
