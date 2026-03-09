package traffic

import (
	"math"
	"math/rand"

	"github.com/anthonyrego/construct/pkg/geojson"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/go-gl/mathgl/mgl32"
)

// Phase represents a traffic light phase.
type Phase int

const (
	Red Phase = iota
	Yellow
	Green
)

const (
	PoleHeight   float32 = 3.5
	PoleWidth    float32 = 0.1
	LightBoxSize float32 = 0.2

	// Vertical positions for the 3 stacked lights (top to bottom: red, yellow, green)
	RedY    float32 = PoleHeight + LightBoxSize*2
	YellowY float32 = PoleHeight + LightBoxSize
	GreenY  float32 = PoleHeight

	// Sign mount heights (above lights)
	SignY1 float32 = RedY + LightBoxSize*1.5
	SignY2 float32 = SignY1 + 0.35

	greenDuration  float32 = 30.0
	yellowDuration float32 = 5.0
	redDuration    float32 = 30.0
	cycleDuration  float32 = greenDuration + yellowDuration + redDuration

	// Half-width offsets from center to curb edge
	oneWayOffset float32 = 4.0
	twoWayOffset float32 = 8.0
)

// Signal represents a single traffic signal with independent timing.
type Signal struct {
	Position geojson.Point2D
	Phase    Phase
	Street1  string  // on-street name
	Street2  string  // cross-street name
	DirAngle float32 // radians: direction the first sign faces
	timer    float32
}

func (s *Signal) advance(dt float32) bool {
	s.timer += dt
	oldPhase := s.Phase
	for {
		var dur float32
		switch s.Phase {
		case Green:
			dur = greenDuration
		case Yellow:
			dur = yellowDuration
		case Red:
			dur = redDuration
		}
		if s.timer < dur {
			break
		}
		s.timer -= dur
		switch s.Phase {
		case Green:
			s.Phase = Yellow
		case Yellow:
			s.Phase = Red
		case Red:
			s.Phase = Green
		}
	}
	return s.Phase != oldPhase
}

// System manages independent traffic signals.
type System struct {
	Signals        []Signal
	dirty          bool
	lightIntensity float32
}

// New creates a traffic system from point locations with street names.
// Each signal gets a random cycle offset and is offset to the curb edge.
func New(locs []geojson.PointLocation, lightIntensity float32, streets []geojson.StreetSegment) *System {
	signals := make([]Signal, len(locs))
	for i, loc := range locs {
		pos := loc.Point
		var dirAngle float32

		if len(streets) > 0 {
			pos, dirAngle = positionAndDirection(loc.Point, streets)
		}

		offset := rand.Float32() * cycleDuration
		sig := Signal{
			Position: pos,
			Street1:  loc.Fields["onstreetna"],
			Street2:  loc.Fields["fromstreet"],
			DirAngle: dirAngle,
		}
		sig.advance(offset)
		signals[i] = sig
	}
	return &System{
		Signals:        signals,
		dirty:          true,
		lightIntensity: lightIntensity,
	}
}

// positionAndDirection finds the nearest street segment, offsets the point
// to the curb, and returns the segment direction angle.
func positionAndDirection(pt geojson.Point2D, streets []geojson.StreetSegment) (geojson.Point2D, float32) {
	bestDist := float32(math.MaxFloat32)
	var bestPerp geojson.Point2D
	var bestTwoWay bool
	var bestDirAngle float32

	for _, seg := range streets {
		for j := 0; j < len(seg.Points)-1; j++ {
			a := seg.Points[j]
			b := seg.Points[j+1]

			dist, perp := pointToSegmentInfo(pt, a, b)
			if dist < bestDist {
				bestDist = dist
				bestPerp = perp
				bestTwoWay = seg.TwoWay
				// Segment direction angle
				dx := b.X - a.X
				dz := b.Z - a.Z
				bestDirAngle = float32(math.Atan2(float64(dx), float64(dz)))
			}
		}
	}

	if bestDist > 50 {
		return pt, 0
	}

	off := oneWayOffset
	if bestTwoWay {
		off = twoWayOffset
	}

	pos := geojson.Point2D{
		X: pt.X + bestPerp.X*off,
		Z: pt.Z + bestPerp.Z*off,
	}

	return pos, bestDirAngle
}

// pointToSegmentInfo returns the distance from point p to segment a-b,
// and the unit perpendicular vector pointing from the segment toward p.
func pointToSegmentInfo(p, a, b geojson.Point2D) (float32, geojson.Point2D) {
	dx := b.X - a.X
	dz := b.Z - a.Z
	lenSq := dx*dx + dz*dz
	if lenSq < 1e-8 {
		d := ptDist(p, a)
		return d, geojson.Point2D{}
	}

	t := ((p.X-a.X)*dx + (p.Z-a.Z)*dz) / lenSq
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	closest := geojson.Point2D{
		X: a.X + t*dx,
		Z: a.Z + t*dz,
	}

	toP := geojson.Point2D{X: p.X - closest.X, Z: p.Z - closest.Z}
	d := float32(math.Sqrt(float64(toP.X*toP.X + toP.Z*toP.Z)))

	if d < 1e-6 {
		segLen := float32(math.Sqrt(float64(lenSq)))
		return 0, geojson.Point2D{X: -dz / segLen, Z: dx / segLen}
	}

	return d, geojson.Point2D{X: toP.X / d, Z: toP.Z / d}
}

func ptDist(a, b geojson.Point2D) float32 {
	dx := a.X - b.X
	dz := a.Z - b.Z
	return float32(math.Sqrt(float64(dx*dx + dz*dz)))
}

// Update advances all signals and marks dirty if any phase changed.
func (s *System) Update(dt float32) {
	for i := range s.Signals {
		if s.Signals[i].advance(dt) {
			s.dirty = true
		}
	}
}

// Dirty returns true if any signal changed phase since last call.
func (s *System) Dirty() bool {
	if s.dirty {
		s.dirty = false
		return true
	}
	return false
}

// Lights returns 3 point lights per signal (red, yellow, green positions).
// Only the active light has intensity; the others are zero.
func (s *System) Lights() []scene.PointLight {
	lights := make([]scene.PointLight, 0, len(s.Signals)*3)
	for _, sig := range s.Signals {
		x, z := sig.Position.X, sig.Position.Z

		var redI, yellowI, greenI float32
		switch sig.Phase {
		case Red:
			redI = s.lightIntensity
		case Yellow:
			yellowI = s.lightIntensity
		case Green:
			greenI = s.lightIntensity
		}

		lights = append(lights,
			scene.PointLight{
				Position:  mgl32.Vec3{x, RedY, z},
				Color:     mgl32.Vec3{1.0, 0.1, 0.0},
				Intensity: redI,
			},
			scene.PointLight{
				Position:  mgl32.Vec3{x, YellowY, z},
				Color:     mgl32.Vec3{1.0, 0.9, 0.0},
				Intensity: yellowI,
			},
			scene.PointLight{
				Position:  mgl32.Vec3{x, GreenY, z},
				Color:     mgl32.Vec3{0.0, 1.0, 0.3},
				Intensity: greenI,
			},
		)
	}
	return lights
}
