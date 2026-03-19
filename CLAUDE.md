# CLAUDE.md

## Project Overview

**Construct** is a cross-platform game library in Go using SDL3 for window management, input handling, and GPU rendering. The goal is to provide a simple foundation for building 3D games with a pixel art aesthetic. Currently renders a real-time explorable 3D reconstruction of lower Manhattan using NYC open data — building footprints, roadbeds, sidewalks, parks, traffic signals, street signs, and pedestrian ramps. Includes an in-game admin editor for modifying buildings and signals, a pause menu with settings UI, and persistent map data with hot-reload.

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
├── scene.json              # Hot-reloadable rendering/lighting config
├── settings.json           # Persistent window/display settings
├── build/                  # Build output (gitignored)
├── .cache/                 # Cached API responses (gitignored)
├── data/
│   └── map/                # Persistent map data (JSON)
│       ├── meta.json       # Projection reference, coordinate bounds
│       ├── blocks/         # Building block files (one per block)
│       ├── intersections/  # Traffic intersection files
│       ├── surfaces/       # Surface polygons (roadbed, sidewalk, park)
│       └── doodads/        # Doodad placement files
├── docs/                   # Reference docs (NYC data APIs, reconstruction plan)
├── pkg/
│   ├── admin/              # In-game admin editor (raycast selection, undo/redo, property editing)
│   ├── building/           # Building registry, polygon extruder, ear-clip triangulation, PLUTO styling
│   ├── camera/             # First-person camera, reversed-Z projection, frustum culling, raycast
│   ├── geojson/            # NYC SODA API fetcher, GeoJSON parser, coordinate projection, PLUTO enrichment
│   ├── ground/             # Surface flattener (roadbeds, sidewalks, parks → flat meshes with curb walls)
│   ├── input/              # Keyboard and mouse input handling
│   ├── mapdata/            # Map data serialization, import from APIs, hot-reload watcher
│   ├── mesh/               # Mesh primitives (cube, lit cube, ground plane, sky dome)
│   ├── ramp/               # Pedestrian ramp geometry (surface + flare wings + edge walls)
│   ├── renderer/           # Two-pass GPU rendering pipeline (reversed-Z depth, multiple pipeline variants)
│   ├── scene/              # Scene graph, spatial grid for frustum culling and two-tier LOD rendering
│   ├── settings/           # Persistent window/display settings (JSON load/save)
│   ├── sign/               # Street name sign mesh generator (text → geometry)
│   ├── snow/               # Snow particle system (billboarded, follows camera)
│   ├── traffic/            # Traffic signal system (pole + heads + phased light cycling)
│   ├── ui/                 # Pause menu + font/text mesh generation
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

### Map Data Store (`pkg/mapdata`)
Two-level data pipeline: API import → JSON files → runtime load.
- **Import**: fetches from NYC APIs and OSM, writes structured JSON to `data/map/`
- **Store**: loads all map data from disk at startup (`blocks/`, `intersections/`, `surfaces/`, `doodads/`)
- **Watcher**: poll-based file change detection (1s interval, 300ms debounce) triggers scene reload
- Data organized by block (buildings grouped by tax block), intersection (traffic signals), and surface type
- `meta.json` stores projection reference point and coordinate bounds

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
- Generates curb walls at surface edges for visual depth between layers

### Traffic Signals (`pkg/traffic`)
- Places traffic light poles at intersection positions
- Snaps to nearest street centerline for curb-edge offset
- Two directional signal heads per intersection (oriented along cross streets)
- Phased light cycling (green → yellow → red) with staggered timing
- Emits point lights matching the active signal color
- Can be constructed from map data (`NewFromMapData()`)

### Street Signs (`pkg/sign`)
- Generates 3D mesh geometry from street name strings
- 3x5 pixel font glyphs extruded into geometry with front/back text
- Mounted on traffic signal poles at intersections

### Pedestrian Ramps (`pkg/ramp`)
- Generates ramp geometry: sloped surface + flare wings + edge walls
- Constructed from map data with NYC CSV-sourced positions
- Orients toward nearest street segment

## Configuration

### scene.json — Hot-Reloadable Rendering Config

Edit while the app is running; changes apply instantly on save.

```json
{
  "postProcess": {
    "ditherStrength": 0.985,
    "colorLevels": 8.0,
    "tintR": 1.08, "tintG": 1.0, "tintB": 0.85
  },
  "lighting": {
    "ambientR": 0.35, "ambientG": 0.33, "ambientB": 0.32,
    "streetLightR": 1.0, "streetLightG": 0.85, "streetLightB": 0.5,
    "streetLightIntensity": 3.2,
    "sunDirX": 0.3, "sunDirY": 0.8, "sunDirZ": 0.5,
    "sunR": 1.0, "sunG": 0.95, "sunB": 0.9, "sunIntensity": 0.5
  },
  "headlamp": {
    "r": 1.0, "g": 0.95, "b": 0.8, "intensity": 3.0
  },
  "snow": {
    "count": 2500, "fallSpeed": 1.2, "windStrength": 0.4, "particleSize": 0.04
  },
  "fog": {
    "r": 0.096, "g": 0.03, "b": 0.136,
    "start": 550, "end": 1550
  },
  "textures": {
    "groundScale": 3.0, "groundStrength": 1
  }
}
```

Key parameters:
- `ditherStrength` — 0 = smooth, 1 = full Bayer dithering
- `colorLevels` — 2 = extreme posterization, 8 = default, 256 = smooth
- `tintR/G/B` — Per-channel color multipliers (1.0 = neutral)
- `headlamp` — Point light that follows the camera (color + intensity)
- `snow` — Particle count, fall speed, wind, and size
- `fog` — Distance fog color, start/end distances
- `lighting.sun*` — Directional sun light direction, color, and intensity
- `textures.groundScale` — UV scale for procedural ground texture
- `textures.groundStrength` — Intensity of ground texture effect

### settings.json — Persistent Display Settings

Saved automatically when changed via pause menu. Loaded at startup.

```json
{
  "windowWidth": 1496,
  "windowHeight": 967,
  "fullscreen": true,
  "pixelScale": 4,
  "renderDistance": 1250
}
```

Key parameters:
- `windowWidth` / `windowHeight` — Window resolution in pixels
- `fullscreen` — Fullscreen toggle
- `pixelScale` — Controls pixel chunkiness. Offscreen = window / scale.
- `renderDistance` — Far plane distance in meters (drives culling, fog fade, camera)

## Demo Controls

### Movement
- **WASD** — Move
- **Mouse** — Look around
- **Scroll Wheel** — Adjust movement speed

### Menus
- **ESC** — Toggle pause menu
- **Backtick (`)** — Toggle admin mode

### Pause Menu
- **Up/Down** — Navigate options
- **Left/Right** — Adjust setting values
- **Enter** — Select option
- **ESC** — Back / close menu

### Admin Mode
- **Crosshair** — Raycast selects the building or signal at screen center
- **Up/Down** — Adjust selected property (e.g. building height)
- **V** — Toggle building visibility (hidden flag)
- **Left/Right** — Adjust signal direction angle
- **Cmd+Z** — Undo last edit
- **Cmd+S** — Save dirty changes to map data files + reload scene

## Codebase Index

### main.go
- `SceneConfig`, `ConfigWatcher` — hot-reloadable scene.json
- `MapWorld` — consolidates map-data-dependent state for clean reload
- `loadFromMapData()` — populates world from `mapdata.Store`
- Main loop: input → update → render (two-pass)

### pkg/admin/
- **admin.go** — `Mode` struct, `Toggle()`, `Update()` (raycast selection), `HandleEdit()`
- **panel.go** — `InfoPanel`: crosshair, mode indicator, entity property display
- **editor.go** — `adjustHeight`, `toggleVisibility`, `rotateSignal`, undo, `SaveDirty`

### pkg/building/
- **building.go** — `Building`, `RawMesh`, `MergedCell`, `BuildingSpan` types; `ExtrudeRaw()`, `UploadMesh()`, `MergeMeshesWithSpans()`
- **registry.go** — `Registry`: central owner of buildings + GPU resources; `ReplaceMesh()`, `RebuildCellMesh()`, `BuildCellMeshes()`
- **style.go** — `StyleColor()`: PLUTO land-use → RGB color
- **triangulate.go** — Ear-clipping polygon triangulation

### pkg/camera/
- **camera.go** — First-person camera, `ReversedZPerspective()`, `ViewMatrix()`
- **frustum.go** — Frustum plane extraction + sphere/AABB culling
- **raycast.go** — Screen-center ray generation for admin selection

### pkg/geojson/
- **geojson.go** — NYC SODA API fetchers, GeoJSON parser, WGS84→local projection, PLUTO enrichment, OSM traffic signals

### pkg/ground/
- **ground.go** — `Flatten()`: surface polygons → horizontal meshes with curb walls
- **datasets.go** — `DatasetConfig` presets for roadbed, sidewalk, park

### pkg/input/
- **input.go** — Keyboard/mouse state, `IsKeyDown`/`Pressed`/`Released`, scroll wheel

### pkg/mapdata/
- **types.go** — `BlockData`, `BuildingData`, `IntersectionData`, `SurfaceFileData`, `DoodadFileData`, `MapMeta`
- **store.go** — `Load(dir)`, `SaveBlock()`, `SaveIntersection()`, `FindBuildingByBBL()`
- **import.go** — `Import()`: fetch from APIs → write JSON to `data/map/`
- **watcher.go** — `MapWatcher`: poll-based file change detection with debounce
- **slug.go** — Street name → filename slug

### pkg/mesh/
- **mesh.go** — Mesh primitives (cube, lit cube, ground plane, sky dome), GPU upload/destroy

### pkg/ramp/
- **ramp.go** — Ramp geometry (surface + flare wings + edge walls), `NewFromMapData()`, `Orient()`

### pkg/renderer/
- **renderer.go** — Two-pass GPU pipeline, 4 pipeline variants (lit, no-depth-write, depth-bias, post-process), `LitDrawCall`, `LightUniforms`
- **texture.go** — GPU texture creation helpers

### pkg/scene/
- **scene.go** — Scene graph, `Object` struct (with `BuildingID`, `Hidden`, `SurfaceType`)
- **grid.go** — `SpatialGrid`: 2D hash grid, `QueryRadius()`, `QueryCells()`, `CellMesh`
- **builders.go** — Helper functions for building scene objects

### pkg/settings/
- **settings.go** — `Settings` struct, `Load()`/`Save()` for persistent window/display config

### pkg/sign/
- **sign.go** — 3x5 pixel font glyphs → 3D sign meshes, front/back text

### pkg/snow/
- **snow.go** — Billboarded particle system with layered wind, dynamic respawn

### pkg/traffic/
- **traffic.go** — `Signal`/`SignalHead` types, `System` with phased cycling + point lights, `NewFromMapData()`
- **datasets.go** — `DatasetConfig` presets for signals and centerlines

### pkg/ui/
- **ui.go** — `PauseMenu`: state machine (Hidden/Main/Settings), `HandleInput()`, `Render()`
- **font.go** — Text mesh generation for UI elements

### pkg/window/
- **window.go** — SDL3 window + GPU device, fullscreen toggle, shader format selection

### shaders/
- **embed.go** — `go:embed` shader loading, auto-selects MSL/SPIRV/DXIL by backend
- **compiled/msl/** — Metal shaders (Lit, PostProcess, PositionColorTransform, SolidColor)
- **compiled/spirv/** — Vulkan shaders
- **compiled/dxil/** — Direct3D 12 shaders
