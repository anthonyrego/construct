# Asset Pipeline

Offline pipeline for generating detailed textured building models from NYC open data. Each building goes through stages — sculpting detailed geometry via OpenSCAD (driven by templates or LLM), UV unwrapping, AI texture generation — producing a mesh with UV coordinates and a per-building texture. Results are packed into per-cell texture atlases for efficient runtime rendering.

## Pipeline Stages

```
Stage 0: Source Data (existing)
  Input:  Building footprint polygon, height, PLUTO metadata, BBL identifier
  Source: data/map/blocks/{block-id}.json

Stage 1: Reference Images
  Input:  BBL, address, lat/lon
  Output: data/assets/buildings/{BBL}/reference/*.jpg
  Status: STUB — AI integration TBD
  Notes:  Fetch street-view or satellite imagery of the actual building.
          Used by sculpt and texture stages as visual reference.

Stage 2: Sculpt (Detailed Mesh)
  Input:  Footprint polygon, height, floors, building class, land use
  Output: data/assets/buildings/{BBL}/mesh.scad, mesh.stl, mesh.bin
  Status: IMPLEMENTED (template mode)
  Method: OpenSCAD + template-based code generation
  Notes:  Generates parametric OpenSCAD code from building data.
          Maps PLUTO building class to architectural features:
            A/B (1-2 family) → stoop, windows, cornice
            C (walk-up)      → stoop, fire escape, windows, cornice
            D (elevator)     → lobby entrance, windows, parapet
            E/F (industrial) → few windows, parapet
            K/S (commercial) → ground floor storefront, upper windows
            O (office)       → wide windows, setback for tall buildings
          Runs OpenSCAD headless to produce STL.
          Converts STL → mesh.bin (Y↔Z swap, vertex dedup, normal computation).
          Future: LLM generates OpenSCAD code for more creative/detailed results.

Stage 3: UV Unwrap
  Input:  Detailed mesh from Stage 2
  Output: Updated mesh with UV coordinates in [0,1] range
  Status: STUB
  Plan:   xatlas (MIT, CPU-only, milliseconds per building)
  Notes:  Algorithmic UV unwrapping, not AI. xatlas via Python bindings.
          Building geometry is mostly rectangular faces — ideal for xatlas.

Stage 4: Texture Generation
  Input:  UV-mapped mesh, reference images
  Output: data/assets/buildings/{BBL}/texture.png (256x256 or 512x512)
  Status: STUB
  Plan:   TRELLIS.2 texturing pipeline (MIT, 24GB VRAM, ~3-5s/building)
          or SDXL + ControlNet Depth + IP-Adapter (~10-12GB VRAM, ~20-30s/building)
  Notes:  AI generates a texture image that maps onto the UV layout.
          Should match reference photos: brick color, window placement,
          signage, fire escape shadows, stoop details.

Stage 5: Bake
  Input:  mesh.bin + texture.png
  Output: Updates manifest.json with bake status "complete"
  Status: IMPLEMENTED
  Notes:  Validates mesh has correct vertex format (position, normal, color, UV).
          Validates texture file exists and is loadable.
          Marks the building as ready for runtime loading.
```

## OpenSCAD Building Library

Reusable parametric modules in `data/scad/building_lib.scad`. The sculpt stage composes these to generate building geometry. Coordinate convention: XY = floor plan, Z = height (Z-up). The STL converter swaps Y↔Z for the engine.

| Module | Parameters | Description |
|--------|-----------|-------------|
| `building_shell` | footprint, height | Extrude 2D polygon to height |
| `window_grid` | wall_w, wall_h, rows, cols, win_w, win_h, depth | Grid of recessed window openings |
| `window` | w, h, depth | Single recessed window |
| `wall_windows` | face_origin, face_angle, wall_w, wall_h, rows, cols | Position window grid on a specific wall |
| `stoop` | width, depth, steps, step_h | NYC-style entrance stairs with landing |
| `cornice` | footprint, height, depth, cornice_h | Decorative edge wrapping the roofline |
| `parapet` | footprint, height, thickness, parapet_h | Low wall around roof edge |
| `fire_escape` | width, floors, floor_h, depth | Platforms + rails + ladder structure |
| `storefront` | width, height, depth, door_w | Ground floor commercial opening |
| `setback` | footprint, total_height, setback_height, inset | Upper floor narrower volume |

Utility functions: `edge_length`, `edge_angle`, `edge_midpoint`, `longest_edge`

### LLM Integration (Future)

The template-based code generator can be replaced by an LLM that generates OpenSCAD code:
- System prompt describes NYC architectural conventions and available modules
- User prompt provides footprint, height, floors, class, address, reference image descriptions
- LLM returns a complete .scad file that `use`s the library
- The same OpenSCAD → STL → mesh.bin pipeline processes the output

## Directory Structure

```
data/
  scad/
    building_lib.scad       OpenSCAD module library
  assets/
    buildings/
      {BBL}/
        manifest.json       Pipeline state tracking
        reference/           Source photographs
        mesh.scad            Generated OpenSCAD code (kept for inspection)
        mesh.stl             OpenSCAD output (intermediate)
        mesh.bin             Engine-ready binary mesh
        texture.png          Per-building texture (RGBA)
    atlases/
      cell_{CX}_{CZ}.png    Per-cell packed texture atlas
      cell_{CX}_{CZ}.json   Atlas UV remapping manifest
```

## Manifest Format

```json
{
  "bbl": "1005300033",
  "version": 1,
  "stages": {
    "reference": { "status": "pending" },
    "sculpt":    { "status": "complete", "updated": "2026-03-27T12:00:00Z" },
    "uv":        { "status": "pending" },
    "texture":   { "status": "pending" },
    "bake":      { "status": "pending" }
  }
}
```

Stage statuses: `pending`, `running`, `complete`, `failed`

## Binary Mesh Format (mesh.bin)

```
Header (8 bytes):
  uint32 vertexCount (little-endian)
  uint32 indexCount  (little-endian)

Vertices (vertexCount * 36 bytes each):
  float32 X, Y, Z       Position (centroid-relative, Y-up)
  float32 NX, NY, NZ    Normal (normalized)
  uint8   R, G, B, A    Vertex color (from PLUTO building class)
  float32 U, V          Texture coordinates [0,1] (0,0 until UV stage)

Indices (indexCount * 4 bytes each):
  uint32 index (little-endian)
```

Total size: `8 + vertexCount * 36 + indexCount * 4` bytes

## STL Conversion

The STL converter (`pkg/asset/stl.go`) handles:
- Binary and ASCII STL parsing
- Vertex deduplication (quantized to 0.1mm precision)
- Per-vertex normal computation (averaged from adjacent face normals)
- Y↔Z axis swap (OpenSCAD Z-up → engine Y-up)
- Centroid computation and vertex centering
- Vertex color assignment from `building.StyleColor()` (PLUTO-based)
- Output in the binary mesh format above

## CLI Usage

```bash
# Process a single building through all pending stages
construct -pipeline -bbl=1005300033

# Run a specific stage (even if already complete)
construct -pipeline -bbl=1005300033 -stage=sculpt

# Preview results
open data/assets/buildings/1005300033/mesh.stl     # macOS Preview (3D viewer)
open data/assets/buildings/1005300033/mesh.scad     # OpenSCAD GUI (editable)

# Import map data first (if needed)
construct -import
```

## Runtime Loading

At startup, `loadFromMapData()` checks each building:
1. Look for `data/assets/buildings/{BBL}/manifest.json`
2. If bake stage is "complete", load `mesh.bin` and replace the extruded box mesh
3. Fallback: use the standard `ExtrudeRaw()` vertex-colored box

Textured buildings use vertex UV coordinates to sample from the building texture (shader slot 1). Untextured buildings have UV=(0,0) and skip texture sampling.

## Atlas System

Per-cell atlases pack individual building textures into one image for efficient rendering.

**Packing**: Row-based rectangle packer (`pkg/asset/atlas.go`). Pixel (0,0) reserved as white so untextured buildings (UV=0,0) sample white.

**UV remapping**: Each building's UVs are transformed from [0,1] to their atlas sub-region:
```
atlasU = entry.UOff + originalU * entry.UScale
atlasV = entry.VOff + originalV * entry.VScale
```

**Runtime**: The merged cell mesh (far-tier LOD) binds the cell's atlas texture before drawing. One draw call per cell with one texture.

## Renderer Integration

**Vertex format** (`LitVertex`): 36 bytes
- Position (float3) + Normal (float3) + Color (ubyte4) + UV (float2)

**Texture slots**:
- Slot 0: Ground texture (procedural tiles for road/sidewalk/park)
- Slot 1: Building atlas (per-cell) or placeholder (1x1 white)

**Shader behavior**: If vertex UV > 0.001, sample building texture with full lighting. Otherwise use vertex color (existing behavior).

## AI Model Research (for future stages)

### UV Unwrapping (Stage 3)
**Recommended: xatlas** — MIT license, CPU-only, milliseconds per building. Standard algorithmic UV unwrapping used by most 3D AI models internally. Python bindings via `pip install xatlas`. Ideal for rectangular building faces.

### Texture Generation (Stage 4)
Two viable approaches for local inference on RTX 4090 (24GB VRAM):

| Approach | VRAM | Speed | Notes |
|----------|------|-------|-------|
| TRELLIS.2 texturing | 24GB | 3-5s/building | MIT license, best quality PBR. Can retexture existing meshes with reference image. |
| SDXL + ControlNet + IP-Adapter | 10-12GB | 20-30s/building | Most control. Render depth maps per facade, generate textures conditioned on reference photos. |

### Mesh Sculpting (Stage 2 — current approach)
OpenSCAD + LLM/templates chosen over image-to-3D AI models because:
- Buildings are parametric (windows in grids, stoops at entrances) — code generation beats image reconstruction
- Uses real PLUTO data (floor count, building class) for accurate architectural features
- Deterministic and inspectable (.scad files can be viewed/edited)
- No GPU required for mesh generation (OpenSCAD is CPU-based)
- Image-to-3D models (TRELLIS.2, SF3D, etc.) not trained on building architecture specifically

## Dependencies

| Tool | Purpose | Install |
|------|---------|---------|
| OpenSCAD | Headless mesh generation from .scad code | `brew install openscad` |
| xatlas (future) | UV unwrapping | `pip install xatlas` |
| TRELLIS.2 or SDXL (future) | Texture generation | Python + CUDA |

## Code Locations

| Component | File |
|-----------|------|
| Pipeline runner | `pkg/pipeline/runner.go` |
| Stage interface | `pkg/pipeline/runner.go` — `Stage` interface |
| Sculpt stage | `pkg/pipeline/sculpt.go` — OpenSCAD code gen + execution |
| Building library | `data/scad/building_lib.scad` — OpenSCAD modules |
| Manifest types | `pkg/asset/manifest.go` |
| Mesh loader | `pkg/asset/loader.go` — `LoadMesh()` |
| STL converter | `pkg/asset/stl.go` — `ParseSTL()`, `ConvertSTLToMeshBin()` |
| Texture loader | `pkg/asset/loader.go` — `LoadTexture()` |
| Atlas packer | `pkg/asset/atlas.go` — `PackAtlas()`, `RemapUVs()` |
| Building style colors | `pkg/building/style.go` — `StyleColor()` |
| Runtime asset check | `world.go` — `loadFromMapData()` |
| Vertex format | `pkg/renderer/renderer.go` — `LitVertex` |
| Texture binding | `pkg/renderer/renderer.go` — `BindBuildingAtlas()` |
| Fragment shader | `shaders/compiled/msl/Lit.frag.msl` |
| CLI flags | `main.go` — `-pipeline`, `-bbl`, `-stage` |
