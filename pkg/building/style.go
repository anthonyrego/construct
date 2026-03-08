package building

import "github.com/anthonyrego/construct/pkg/geojson"

// Color holds an RGB triplet.
type Color struct {
	R, G, B uint8
}

// StyleColor returns a color based on PLUTO building class and land use.
// Falls back to a neutral gray if no data is available.
func StyleColor(pluto geojson.PLUTOData) Color {
	// Try building class first (first letter = category)
	if len(pluto.BldgClass) > 0 {
		switch pluto.BldgClass[0] {
		case 'A': // One-family dwellings
			return Color{140, 100, 70} // warm brown
		case 'B': // Two-family dwellings
			return Color{130, 95, 65} // slightly lighter brown
		case 'C': // Walk-up apartments
			return Color{120, 85, 60} // classic brownstone
		case 'D': // Elevator apartments
			return Color{150, 140, 130} // concrete/stone gray
		case 'E': // Warehouses
			return Color{95, 85, 80} // dark industrial
		case 'F': // Factory/industrial
			return Color{100, 90, 85} // industrial gray-brown
		case 'G': // Garages
			return Color{90, 88, 85} // dark gray
		case 'H': // Hotels
			return Color{160, 145, 120} // warm beige
		case 'I': // Hospitals/health
			return Color{170, 165, 160} // light gray
		case 'J': // Theatres
			return Color{145, 110, 90} // warm terracotta
		case 'K': // Stores
			return Color{135, 120, 105} // medium warm
		case 'L': // Lofts
			return Color{110, 100, 90} // dark warm
		case 'M': // Religious
			return Color{155, 140, 120} // sandstone
		case 'N': // Asylums
			return Color{140, 135, 130} // institutional gray
		case 'O': // Office buildings
			return Color{160, 160, 165} // blue-gray steel/glass
		case 'P': // Indoor recreation
			return Color{130, 125, 115} // neutral warm
		case 'Q': // Outdoor recreation
			return Color{110, 120, 100} // greenish
		case 'R': // Condos
			return Color{155, 145, 135} // modern beige
		case 'S': // Mixed residential/commercial
			return Color{135, 115, 95} // warm mixed
		case 'W': // Educational
			return Color{145, 135, 125} // institutional warm
		}
	}

	// Fall back to land use category
	switch pluto.LandUse {
	case "1": // One & two family
		return Color{130, 95, 65}
	case "2": // Multi-family walk-up
		return Color{120, 85, 60}
	case "3": // Multi-family elevator
		return Color{150, 140, 130}
	case "4": // Mixed residential/commercial
		return Color{135, 115, 95}
	case "5": // Commercial/office
		return Color{160, 160, 165}
	case "6": // Industrial/manufacturing
		return Color{100, 90, 85}
	case "7": // Transportation/utility
		return Color{110, 108, 105}
	case "8": // Public facilities/institutions
		return Color{145, 140, 135}
	case "9": // Open space/recreation
		return Color{110, 120, 100}
	case "10": // Parking
		return Color{90, 88, 85}
	case "11": // Vacant land
		return Color{105, 100, 95}
	}

	// Default
	return Color{125, 115, 105}
}
