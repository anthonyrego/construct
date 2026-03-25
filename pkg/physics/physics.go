package physics

import "math"

// Body represents a physical entity in the world.
type Body struct {
	X, Y, Z          float32 // feet position
	VelX, VelY, VelZ float32 // velocity in m/s
	Radius           float32 // XZ collision circle
	Height           float32 // body height (for eye offset by caller)
	Grounded         bool
	Gravity          float32 // downward acceleration (positive value)
	StepHeight       float32 // max height the body can step up
}

// GroundHeightFunc returns the ground Y at position (x, z).
// Returns (y, true) if a surface exists, or (0, false) if not.
type GroundHeightFunc func(x, z float32) (y float32, ok bool)

// CollisionFunc tests whether moving from (oldX, oldZ) to (newX, newZ)
// with the given radius causes a collision. Returns the corrected position
// after wall sliding.
type CollisionFunc func(oldX, oldZ, newX, newZ, radius float32) (x, z float32, collided bool)

// World manages physics simulation.
type World struct {
	GroundHeight GroundHeightFunc
	Collision    CollisionFunc
	DefaultY     float32 // ground level when no surface found
}

// Step advances the body by dt seconds: gravity, integration, collision, ground clamping.
func (w *World) Step(b *Body, dt float32) {
	// Gravity
	if !b.Grounded {
		b.VelY -= b.Gravity * dt
	}

	// Integrate Y
	newY := b.Y + b.VelY*dt

	// Integrate XZ
	newX := b.X + b.VelX*dt
	newZ := b.Z + b.VelZ*dt

	// Building collision (XZ slide)
	if w.Collision != nil {
		slidX, slidZ, _ := w.Collision(b.X, b.Z, newX, newZ, b.Radius)
		newX = slidX
		newZ = slidZ
	}

	// Ground query at resolved XZ position
	groundY := w.DefaultY
	if w.GroundHeight != nil {
		if gy, ok := w.GroundHeight(newX, newZ); ok {
			groundY = gy
		}
	}

	// Step-up check: if moving onto higher ground within StepHeight, allow it.
	// If the height difference exceeds StepHeight, reject the XZ move.
	if b.Grounded && groundY > b.Y+b.StepHeight {
		// Too tall to step up — treat as wall, keep old position
		newX = b.X
		newZ = b.Z
		// Re-query ground at old position
		groundY = w.DefaultY
		if w.GroundHeight != nil {
			if gy, ok := w.GroundHeight(newX, newZ); ok {
				groundY = gy
			}
		}
	}

	// Clamp to ground
	if newY <= groundY {
		newY = groundY
		b.VelY = 0
		b.Grounded = true
	} else if newY > groundY+0.05 {
		b.Grounded = false
	}

	// Commit position
	b.X = newX
	b.Y = newY
	b.Z = newZ

	// Zero horizontal velocity (caller sets it each frame from input)
	b.VelX = 0
	b.VelZ = 0
}

// PointInPolygon tests if point (px, pz) is inside a polygon using ray casting.
// polygon is a slice of [2]float32 where [0]=X, [1]=Z.
func PointInPolygon(px, pz float32, polygon [][2]float32) bool {
	n := len(polygon)
	if n < 3 {
		return false
	}
	inside := false
	j := n - 1
	for i := 0; i < n; i++ {
		ix, iz := polygon[i][0], polygon[i][1]
		jx, jz := polygon[j][0], polygon[j][1]

		if (iz > pz) != (jz > pz) &&
			px < (jx-ix)*(pz-iz)/(jz-iz)+ix {
			inside = !inside
		}
		j = i
	}
	return inside
}

// SegmentCircleIntersect tests if a circle at (cx, cz) with radius r
// intersects line segment from (ax, az) to (bx, bz).
// Returns the push-out vector that moves the circle center outside the segment.
func SegmentCircleIntersect(cx, cz, r, ax, az, bx, bz float32) (pushX, pushZ float32, hit bool) {
	// Find closest point on segment to circle center
	dx := bx - ax
	dz := bz - az
	lenSq := dx*dx + dz*dz
	if lenSq < 1e-8 {
		// Degenerate segment
		ddx := cx - ax
		ddz := cz - az
		dist := float32(math.Sqrt(float64(ddx*ddx + ddz*ddz)))
		if dist < r && dist > 1e-6 {
			pen := r - dist
			return ddx / dist * pen, ddz / dist * pen, true
		}
		return 0, 0, false
	}

	// Project circle center onto segment line
	t := ((cx-ax)*dx + (cz-az)*dz) / lenSq
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	// Closest point on segment
	closestX := ax + t*dx
	closestZ := az + t*dz

	// Distance from circle center to closest point
	toX := cx - closestX
	toZ := cz - closestZ
	dist := float32(math.Sqrt(float64(toX*toX + toZ*toZ)))

	if dist < r && dist > 1e-6 {
		pen := r - dist
		return toX / dist * pen, toZ / dist * pen, true
	}

	return 0, 0, false
}
