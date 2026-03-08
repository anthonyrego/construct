# NYC Neighborhood Reconstruction Plan

## Pipeline Overview

```
NYC Data                    Processing                   Rendering
┌──────────────┐     ┌─────────────────────┐     ┌──────────────────┐
│ Footprints   │────>│ Parse GeoJSON       │────>│ Extruded quads   │
│ (polygon +   │     │ polygons -> vertices │     │ with UV-mapped   │
│  height)     │     │                     │     │ facade textures  │
├──────────────┤     ├─────────────────────┤     ├──────────────────┤
│ PLUTO        │────>│ Join on BBL ->      │────>│ Style selection: │
│ (class, era, │     │ building descriptor │     │ which atlas tile │
│  floors)     │     │                     │     │ for each face    │
├──────────────┤     ├─────────────────────┤     ├──────────────────┤
│ Class codes  │────>│ Map code -> style   │────>│ Facade atlas     │
│ (A1, D3, O4) │     │ (brick, glass, etc) │     │ (pre-generated)  │
└──────────────┘     └─────────────────────┘     └──────────────────┘
```

## Components to Build

### 1. GeoJSON Fetcher (`pkg/geodata/`)
- HTTP client to query NYC Open Data SODA API
- Fetch building footprints by bounding box or radius
- Fetch PLUTO attributes and join on BBL
- Parse GeoJSON polygons (no external packages — just `encoding/json` + `net/http`)
- Coordinate transform: WGS84 lat/lon -> local game-world meters

### 2. Polygon Extruder (`pkg/geodata/` or `pkg/mesh/`)
- Take 2D footprint polygon + height -> 3D mesh
- Triangulate the footprint polygon for roof/floor caps (ear clipping)
- Generate wall quads from consecutive polygon vertices
- Subdivide walls into floor-height rows for texture mapping
- Output `LitVertex` data compatible with existing renderer

### 3. Style Mapper (future)
- Map `bldgclass` + `yearbuilt` + `numfloors` -> facade style enum
- Select appropriate texture atlas region per style
- Assign UV coordinates accordingly

### 4. Facade Texture Atlas (future)
- Pre-generate tileable facade textures using SDXL + tiling mode
- Categories: brick residential, brownstone, glass commercial, industrial, retail storefront
- Pack into atlas with known UV regions
- The post-process shader gives everything pixel-art treatment automatically

## Coordinate System

NYC footprints come in WGS84 (lat/lon). We need to convert to game-world meters.

**Approach:** Pick a center point (lat0, lon0) as origin, then:
```
x = (lon - lon0) * cos(lat0) * 111319.5   (meters east)
z = (lat - lat0) * 111319.5                (meters north)
y = ground_elevation (feet -> meters)       or 0 if flat
```

This is a simple equirectangular projection, accurate enough for a neighborhood-scale area (~1km).

## Mesh Generation Strategy

For each building:

1. **Walls:** For each consecutive pair of footprint vertices (v0, v1):
   - Create a quad from ground to `height_roof`
   - Optionally subdivide vertically by floor count for texture UV mapping
   - Compute outward-facing normal from edge direction

2. **Roof:** Triangulate the footprint polygon (ear clipping) at y = height_roof
   - Normal = (0, 1, 0)

3. **Floor (optional):** Same triangulation at y = 0
   - Normal = (0, -1, 0)

All output as `LitVertex` (position + normal + color) for the existing lit rendering pipeline. Textures come later via UV coordinates.

## What Works Today vs. Future

| Piece | Approach | Status |
|-------|----------|--------|
| GeoJSON fetch + parse | Hand-rolled Go (`net/http` + `encoding/json`) | To build |
| Polygon extrusion | Walls = edge quads, roof = ear-clip triangulation | To build |
| Coordinate transform | Equirectangular projection from center point | To build |
| Integration with renderer | Output LitVertex, use existing DrawLit pipeline | Straightforward |
| Style mapping | bldgclass + era -> style enum | Future |
| Facade textures | SDXL-generated atlas, UV-mapped facades | Future |
| AI 3D props | Meshy/Tripo for trees, cars, furniture (GLB import) | Future |
