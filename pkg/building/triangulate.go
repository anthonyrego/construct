package building

import (
	"github.com/anthonyrego/construct/pkg/geojson"
)

// Triangulate returns indices into the points slice forming triangles
// via ear-clipping. Input must be CCW wound.
func Triangulate(points []geojson.Point2D) []uint16 {
	n := len(points)
	if n < 3 {
		return nil
	}

	// Build active vertex index list
	active := make([]int, n)
	for i := range active {
		active[i] = i
	}

	var indices []uint16

	for len(active) > 2 {
		earFound := false
		m := len(active)
		for i := 0; i < m; i++ {
			prev := active[(i-1+m)%m]
			curr := active[i]
			next := active[(i+1)%m]

			a := points[prev]
			b := points[curr]
			c := points[next]

			// Must be convex (positive cross product for CCW)
			if cross2D(a, b, c) <= 0 {
				continue
			}

			// Check no other active vertex is inside this triangle
			ear := true
			for j := 0; j < m; j++ {
				if j == (i-1+m)%m || j == i || j == (i+1)%m {
					continue
				}
				if pointInTriangle(points[active[j]], a, b, c) {
					ear = false
					break
				}
			}

			if ear {
				indices = append(indices, uint16(prev), uint16(curr), uint16(next))
				// Remove curr from active list
				active = append(active[:i], active[i+1:]...)
				earFound = true
				break
			}
		}

		if !earFound {
			// Degenerate polygon — emit remaining as fan to avoid infinite loop
			for i := 1; i < len(active)-1; i++ {
				indices = append(indices, uint16(active[0]), uint16(active[i]), uint16(active[i+1]))
			}
			break
		}
	}

	return indices
}

func cross2D(o, a, b geojson.Point2D) float32 {
	return (a.X-o.X)*(b.Z-o.Z) - (a.Z-o.Z)*(b.X-o.X)
}

func pointInTriangle(p, a, b, c geojson.Point2D) bool {
	d1 := cross2D(a, b, p)
	d2 := cross2D(b, c, p)
	d3 := cross2D(c, a, p)

	hasNeg := (d1 < 0) || (d2 < 0) || (d3 < 0)
	hasPos := (d1 > 0) || (d2 > 0) || (d3 > 0)

	return !(hasNeg && hasPos)
}
