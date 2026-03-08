# NYC Building Data Sources

Public data sources for reconstructing NYC neighborhoods in 3D. All are free and public domain under NYC Open Data Terms of Use.

## 1. Building Footprints

Polygon outlines for every building in NYC (1M+ buildings).

**Key fields:**
- `the_geom` — MultiPolygon footprint geometry
- `bin` — Building Identification Number
- `base_bbl` — Borough-Block-Lot (for joining to PLUTO)
- `height_roof` — Roof height above ground in feet
- `ground_elevation` — Ground elevation (NAVD88, feet)
- `construction_year` — Year built
- `feature_code` — Type (2100=Building, 5100=Under Construction, 5110=Garage, etc.)
- `name` — Building name (if any)

**API:**
```
https://data.cityofnewyork.us/resource/5zhs-2jue.geojson
```

**Spatial query example (200m radius around a point):**
```
?$where=within_circle(the_geom,40.7484,-73.9856,200)&$select=the_geom,bin,height_roof,ground_elevation,construction_year,base_bbl
```

**Coordinate system:** API returns WGS84 (lat/lon). Native data is EPSG:2263 (NY State Plane).

**Format:** GeoJSON, Shapefile, CSV. Updated daily.

---

## 2. PLUTO (Primary Land Use Tax Lot Output)

Tax lot-level data for all ~870,000 NYC properties with 90+ attributes.

**Key fields for 3D reconstruction:**

| Field | Description |
|-------|-------------|
| `bbl` | Borough-Block-Lot unique identifier |
| `address` | Street address |
| `bldgclass` | DOF building classification code (e.g. "A5", "D3", "O4") |
| `landuse` | Land use category (1-11) |
| `numfloors` | Number of floors |
| `yearbuilt` | Year of construction |
| `bldgfront` / `bldgdepth` | Building dimensions in feet |
| `bldgarea` | Total gross building area in sq ft |
| `numbldgs` | Number of buildings on lot |
| `unitsres` / `unitstotal` | Residential / total units |
| `comarea` / `resarea` / `officearea` / `retailarea` | Area breakdowns by use |
| `histdist` | Historic district name |
| `landmark` | Landmark status |
| `zonedist1`-`zonedist4` | Zoning districts |
| `latitude` / `longitude` | Centroid coordinates |

**Note:** PLUTO has no explicit height field. Use Building Footprints `height_roof` instead, or estimate from `numfloors * ~10-12 ft`.

**API:**
```
https://data.cityofnewyork.us/resource/64uk-42ks.json
```

**Example query (Manhattan buildings over 20 floors):**
```
?$where=numfloors>20 AND borough='MN'&$select=bbl,address,bldgclass,numfloors,yearbuilt,bldgarea
```

**Bulk download (with lot geometry):** MapPLUTO from https://www.nyc.gov/site/planning/data-maps/open-data/dwn-pluto-mappluto.page

---

## 3. Building Classification Codes

Source: https://www.nyc.gov/assets/finance/jump/hlpbldgcode.html

~170 codes across 26 letter categories. These are the key signal for determining building visual style.

### Categories Relevant to Visual Style

| Code | Category | Typical Appearance |
|------|----------|--------------------|
| **A** | One Family Dwellings | Cape Cod, detached frame, townhouse, mansion |
| **B** | Two Family Dwellings | Brick or frame, converted single-family |
| **C** | Walk-Up Apartments | Old law tenement, brownstone, garden apt (3-6 stories) |
| **D** | Elevator Apartments | Pre-war brick, modern high-rise, luxury (7+ stories) |
| **E** | Warehouses | Industrial, large footprint, few windows |
| **F** | Factories | Heavy/light manufacturing, industrial windows |
| **G** | Garages & Gas Stations | Open structures, ramps, canopies |
| **H** | Hotels | Varies by subtype: luxury, chain, motel, boutique |
| **K** | Store Buildings | Retail storefronts, 1-story to department store |
| **L** | Lofts | Cast-iron facades, large windows, industrial converted |
| **M** | Religious | Churches, synagogues, chapels |
| **O** | Office Buildings | O1=1-story, O2=2-6, O3=7-19, O4=20+ (glass towers) |
| **R** | Condominiums | Walk-up or elevator, residential or commercial |
| **S** | Mixed-Use Residential | Residential above, commercial at street level |
| **W** | Educational | Schools, universities |
| **Y** | Government | Fire dept, police, prisons |

### Style Mapping Strategy

Combine `bldgclass` + `yearbuilt` + `numfloors` to select facade style:

```
D3 + yearbuilt=1928 + numfloors=12  →  "pre-war brick elevator building"
C4 + yearbuilt=1905 + numfloors=5   →  "old law tenement"
O4 + yearbuilt=2015 + numfloors=45  →  "modern glass curtain wall tower"
K1 + yearbuilt=1960 + numfloors=1   →  "single-story retail storefront"
L1 + yearbuilt=1890 + numfloors=10  →  "cast-iron loft building"
A7 + yearbuilt=1910 + numfloors=4   →  "brownstone townhouse"
```

---

## 4. NYC 3D Building Model (Reference)

Pre-built 3D massing models from 2014 aerial photography.

- **LOD:** Mostly LOD1 (extruded footprints). ~100 iconic buildings at LOD2 (detailed roofs).
- **Formats:** CityGML (894 MB), Multipatch Shapefile (251 MB), DGN (744 MB)
- **Attributes:** BIN, DOITT_ID only
- **Accuracy:** Horizontal +/-1.25 ft, Vertical +/-1.6 ft
- **Download:** https://data.cityofnewyork.us/City-Government/3-D-Building-Model/tnru-abg2
- **Enhanced version** (merged with PLUTO): https://github.com/georocket/new-york-city-model-enhanced

Useful as reference/validation, but we generate our own geometry from footprints for better control.

---

## 5. API Details

All APIs use Socrata Open Data API (SODA). No authentication required but an app token is recommended to avoid throttling.

**Common query parameters:**
- `$select=field1,field2` — choose columns
- `$where=condition` — filter (SQL-like)
- `$limit=1000` — row limit (default 1000, max 50000)
- `$offset=1000` — pagination
- `$order=field DESC` — sorting
- Spatial: `within_circle(the_geom, lat, lon, radius_meters)`

**Join strategy:** Building Footprints and PLUTO join on BBL (Borough-Block-Lot). Footprints have `base_bbl`, PLUTO has `bbl`.

**Register for app token:** https://data.cityofnewyork.us/profile/edit/developer_settings
