package pipeline

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anthonyrego/construct/pkg/asset"
	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/mapdata"
)

// sculptStage generates detailed building geometry using OpenSCAD.
type sculptStage struct {
	scadLibDir string // directory containing building_lib.scad
}

func (s *sculptStage) Name() string { return "sculpt" }

func (s *sculptStage) Run(ctx StageContext) error {
	if ctx.Building == nil {
		return fmt.Errorf("no building data for BBL %s", ctx.BBL)
	}

	// Generate OpenSCAD code
	scadCode := generateScadCode(ctx.Building, s.scadLibDir)
	scadPath := filepath.Join(ctx.AssetDir, "mesh.scad")
	if err := os.WriteFile(scadPath, []byte(scadCode), 0o644); err != nil {
		return fmt.Errorf("write scad: %w", err)
	}

	// Run OpenSCAD
	stlPath := filepath.Join(ctx.AssetDir, "mesh.stl")
	cmd := exec.Command("openscad", "-o", stlPath, scadPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("openscad: %w", err)
	}

	// Get building color from PLUTO data
	pluto := geojson.PLUTOData{
		BldgClass: ctx.Building.BuildingClass,
		LandUse:   ctx.Building.LandUse,
	}
	color := building.StyleColor(pluto)

	// Convert STL → mesh.bin (swap Y↔Z for engine's Y-up convention)
	meshBinPath := filepath.Join(ctx.AssetDir, "mesh.bin")
	if err := asset.ConvertSTLToMeshBin(stlPath, meshBinPath, color.R, color.G, color.B, true); err != nil {
		return fmt.Errorf("convert STL: %w", err)
	}

	fmt.Printf("    sculpt: generated mesh.bin from OpenSCAD\n")
	return nil
}

// generateScadCode creates OpenSCAD code from building data using templates.
func generateScadCode(b *mapdata.BuildingData, libDir string) string {
	var sb strings.Builder

	// Library include (use absolute path for OpenSCAD)
	absLib, _ := filepath.Abs(filepath.Join(libDir, "building_lib.scad"))
	sb.WriteString(fmt.Sprintf("use <%s>;\n\n", absLib))

	// Footprint polygon
	sb.WriteString("footprint = [\n")
	for i, pt := range b.Footprint {
		if i > 0 {
			sb.WriteString(",\n")
		}
		sb.WriteString(fmt.Sprintf("  [%.4f, %.4f]", pt[0], pt[1]))
	}
	sb.WriteString("\n];\n\n")

	// Building parameters
	height := b.Height
	if height <= 0 {
		height = 10
	}
	floors := b.Floors
	if floors <= 0 {
		floors = int(height / 3.0)
		if floors < 1 {
			floors = 1
		}
	}
	floorH := height / float32(floors)

	sb.WriteString(fmt.Sprintf("height = %.2f;\n", height))
	sb.WriteString(fmt.Sprintf("floors = %d;\n", floors))
	sb.WriteString(fmt.Sprintf("floor_h = %.2f;\n\n", floorH))

	// Determine building type from PLUTO class
	bldgType := classifyBuilding(b.BuildingClass, b.LandUse)

	// Find facade info (longest edge = likely street-facing)
	facadeIdx, facadeLen := longestEdge(b.Footprint)
	facadeAngle := edgeAngle(b.Footprint, facadeIdx)
	facadeOrigin := b.Footprint[facadeIdx]

	// Generate building based on type
	sb.WriteString("// Building shell with window recesses\n")
	sb.WriteString("difference() {\n")

	// Shell (possibly with setback for tall buildings)
	if floors > 8 && bldgType == "office" {
		sb.WriteString(fmt.Sprintf("  setback(footprint, height, %.2f, 2.0);\n", float32(floors-3)*floorH))
	} else {
		sb.WriteString("  building_shell(footprint, height);\n")
	}

	// Windows on each facade edge
	sb.WriteString("\n  // Window recesses\n")
	for i := 0; i < len(b.Footprint); i++ {
		eLen := edgeLength(b.Footprint, i)
		if eLen < 3.0 {
			continue // skip very short edges
		}
		eAngle := edgeAngle(b.Footprint, i)
		origin := b.Footprint[i]

		cols := int(eLen / 2.5)
		if cols < 1 {
			cols = 1
		}
		if cols > 10 {
			cols = 10
		}

		winW := float32(1.0)
		winH := float32(1.5)
		if bldgType == "office" {
			winW = 1.4
			winH = 1.8
		}

		rows := floors
		if bldgType == "commercial" && rows > 1 {
			rows = floors - 1 // ground floor is storefront
		}

		sb.WriteString(fmt.Sprintf("  wall_windows([%.4f, %.4f], %.2f, %.2f, height, %d, %d, %.2f, %.2f);\n",
			origin[0], origin[1], eAngle, eLen, rows, cols, winW, winH))
	}

	// Ground floor storefront for commercial buildings
	if bldgType == "commercial" && facadeLen > 3.0 {
		sb.WriteString(fmt.Sprintf("\n  // Storefront\n"))
		sb.WriteString(fmt.Sprintf("  translate([%.4f, %.4f, 0])\n", facadeOrigin[0], facadeOrigin[1]))
		sb.WriteString(fmt.Sprintf("    rotate([0, 0, %.2f])\n", facadeAngle))
		sfWidth := facadeLen * 0.7
		if sfWidth > 6 {
			sfWidth = 6
		}
		sb.WriteString(fmt.Sprintf("      storefront(%.2f, %.2f);\n", sfWidth, floorH))
	}

	sb.WriteString("}\n\n")

	// Additive elements (outside the difference block)

	// Cornice
	if bldgType != "industrial" {
		sb.WriteString("// Cornice\n")
		sb.WriteString(fmt.Sprintf("cornice(footprint, height, 0.25, 0.35);\n\n"))
	}

	// Parapet for certain types
	if bldgType == "industrial" || bldgType == "office" {
		sb.WriteString("// Parapet\n")
		sb.WriteString(fmt.Sprintf("parapet(footprint, height, 0.15, 0.6);\n\n"))
	}

	// Stoop for residential
	if bldgType == "residential" && facadeLen > 2.5 {
		mid := edgeMidpoint(b.Footprint, facadeIdx)
		// Normal direction (outward from CCW polygon)
		j := (facadeIdx + 1) % len(b.Footprint)
		dx := b.Footprint[j][0] - b.Footprint[facadeIdx][0]
		dy := b.Footprint[j][1] - b.Footprint[facadeIdx][1]
		nx := dy / facadeLen
		ny := -dx / facadeLen

		sb.WriteString("// Stoop\n")
		sb.WriteString(fmt.Sprintf("translate([%.4f, %.4f, 0])\n", mid[0]+nx*0.5, mid[1]+ny*0.5))
		sb.WriteString(fmt.Sprintf("  rotate([0, 0, %.2f])\n", facadeAngle+90))
		steps := 4
		if floors <= 2 {
			steps = 3
		}
		sb.WriteString(fmt.Sprintf("    stoop(2.5, 1.8, %d, 0.18);\n\n", steps))
	}

	// Fire escape for walk-ups
	if bldgType == "walkup" && floors > 2 && facadeLen > 2.5 {
		sb.WriteString("// Fire escape\n")
		sb.WriteString(fmt.Sprintf("translate([%.4f, %.4f, 0])\n", facadeOrigin[0], facadeOrigin[1]))
		sb.WriteString(fmt.Sprintf("  rotate([0, 0, %.2f])\n", facadeAngle))
		feWidth := float32(2.5)
		if facadeLen < 5 {
			feWidth = facadeLen * 0.4
		}
		sb.WriteString(fmt.Sprintf("    translate([%.2f, 0, 0])\n", (facadeLen-feWidth)*0.3))
		sb.WriteString(fmt.Sprintf("      fire_escape(%.2f, %d, %.2f);\n\n", feWidth, floors-1, floorH))
	}

	return sb.String()
}

// classifyBuilding maps PLUTO data to a building archetype.
func classifyBuilding(bldgClass, landUse string) string {
	if len(bldgClass) > 0 {
		switch bldgClass[0] {
		case 'A', 'B':
			return "residential"
		case 'C':
			return "walkup"
		case 'D', 'R':
			return "elevator"
		case 'E', 'F':
			return "industrial"
		case 'K', 'S':
			return "commercial"
		case 'O':
			return "office"
		}
	}
	switch landUse {
	case "1", "2":
		return "residential"
	case "3":
		return "elevator"
	case "4", "5":
		return "commercial"
	case "6":
		return "industrial"
	}
	return "residential"
}

func edgeLength(footprint [][2]float32, i int) float32 {
	j := (i + 1) % len(footprint)
	dx := footprint[j][0] - footprint[i][0]
	dy := footprint[j][1] - footprint[i][1]
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

func edgeAngle(footprint [][2]float32, i int) float32 {
	j := (i + 1) % len(footprint)
	dx := footprint[j][0] - footprint[i][0]
	dy := footprint[j][1] - footprint[i][1]
	return float32(math.Atan2(float64(dy), float64(dx)) * 180 / math.Pi)
}

func edgeMidpoint(footprint [][2]float32, i int) [2]float32 {
	j := (i + 1) % len(footprint)
	return [2]float32{
		(footprint[i][0] + footprint[j][0]) / 2,
		(footprint[i][1] + footprint[j][1]) / 2,
	}
}

func longestEdge(footprint [][2]float32) (int, float32) {
	bestIdx := 0
	bestLen := float32(0)
	for i := range footprint {
		l := edgeLength(footprint, i)
		if l > bestLen {
			bestLen = l
			bestIdx = i
		}
	}
	return bestIdx, bestLen
}
