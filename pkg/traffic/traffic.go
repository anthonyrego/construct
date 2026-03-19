package traffic

import (
	"fmt"
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
	ArmOffset    float32 = 0.5 // horizontal offset from pole for each signal head

	// Vertical positions for the 3 stacked lights (top to bottom: red, yellow, green)
	RedY    float32 = PoleHeight + LightBoxSize*2
	YellowY float32 = PoleHeight + LightBoxSize
	GreenY  float32 = PoleHeight

	// Housing (dark box behind lights to block side/rear view)
	HousingWidth  float32 = LightBoxSize + 0.1
	HousingHeight float32 = LightBoxSize*3 + 0.1
	HousingDepth  float32 = 0.25
	HousingY      float32 = PoleHeight + LightBoxSize // vertical center of the 3 lights
	LightForward  float32 = HousingDepth/2 + 0.01     // how far lights sit in front of housing

	// Sign mount heights (above lights, stacked)
	SignY1 float32 = RedY + LightBoxSize*1.5
	SignY2 float32 = SignY1 + 0.35

	greenDuration  float32 = 30.0
	yellowDuration float32 = 5.0

	// Coordinated cycle: Dir1 green/yellow, then Dir2 green/yellow
	// Dir1: green(30) → yellow(5) → red(35)
	// Dir2: red(35) → green(30) → yellow(5)
	coordCycleDuration float32 = 2 * (greenDuration + yellowDuration) // 70s

	// Half-width offsets from center to curb edge
	oneWayOffset float32 = 4.0
	twoWayOffset float32 = 8.0
)

// SignalHead represents one directional signal head at an intersection.
type SignalHead struct {
	X, Z   float32
	Phase  Phase
	Angle  float32
	Street string
}

// Signal represents a coordinated intersection with two directional signal heads.
type Signal struct {
	ID       string          // unique identifier for store linkage
	Position geojson.Point2D
	Phase1   Phase   // current phase for street1 direction
	Phase2   Phase   // current phase for street2 direction
	Street1  string  // on-street name
	Street2  string  // cross-street name
	DirAngle float32 // radians: direction the street1 head faces
	timer    float32
}

// Heads returns the two directional signal heads with their positions and phases.
func (s *Signal) Heads() [2]SignalHead {
	var heads [2]SignalHead
	phases := [2]Phase{s.Phase1, s.Phase2}
	streets := [2]string{s.Street1, s.Street2}

	for i := 0; i < 2; i++ {
		angle := s.DirAngle + float32(i)*math.Pi/2
		ox := float32(math.Sin(float64(angle))) * ArmOffset
		oz := float32(math.Cos(float64(angle))) * ArmOffset
		heads[i] = SignalHead{
			X:      s.Position.X + ox,
			Z:      s.Position.Z + oz,
			Phase:  phases[i],
			Angle:  angle,
			Street: streets[i],
		}
	}
	return heads
}

func (s *Signal) advance(dt float32) bool {
	s.timer += dt
	for s.timer >= coordCycleDuration {
		s.timer -= coordCycleDuration
	}

	old1, old2 := s.Phase1, s.Phase2

	t := s.timer
	switch {
	case t < greenDuration:
		s.Phase1 = Green
		s.Phase2 = Red
	case t < greenDuration+yellowDuration:
		s.Phase1 = Yellow
		s.Phase2 = Red
	case t < 2*greenDuration+yellowDuration:
		s.Phase1 = Red
		s.Phase2 = Green
	default:
		s.Phase1 = Red
		s.Phase2 = Yellow
	}

	return s.Phase1 != old1 || s.Phase2 != old2
}

// IntersectionsFromSegments derives intersection points from street centerline
// endpoints. Where 2+ segment endpoints cluster within snapDist meters, an
// intersection is created with the two most common street names.
func IntersectionsFromSegments(segments []geojson.StreetSegment, snapDist float32) []geojson.PointLocation {
	type node struct {
		pos   geojson.Point2D
		names map[string]bool
	}

	var nodes []node

	addEndpoint := func(pt geojson.Point2D, name string) {
		// Find existing node within snapDist
		for i := range nodes {
			dx := nodes[i].pos.X - pt.X
			dz := nodes[i].pos.Z - pt.Z
			if dx*dx+dz*dz < snapDist*snapDist {
				if name != "" {
					nodes[i].names[name] = true
				}
				return
			}
		}
		names := make(map[string]bool)
		if name != "" {
			names[name] = true
		}
		nodes = append(nodes, node{pos: pt, names: names})
	}

	for _, seg := range segments {
		if len(seg.Points) < 2 {
			continue
		}
		// Add both endpoints
		addEndpoint(seg.Points[0], seg.Name)
		addEndpoint(seg.Points[len(seg.Points)-1], seg.Name)
	}

	// Only keep nodes where 2+ distinct streets meet (actual intersections)
	var locs []geojson.PointLocation
	for _, n := range nodes {
		if len(n.names) < 2 {
			continue
		}
		// Pick first two names
		fields := map[string]string{}
		i := 0
		for name := range n.names {
			if i == 0 {
				fields["onstreetna"] = name
			} else if i == 1 {
				fields["fromstreet"] = name
			} else {
				break
			}
			i++
		}
		locs = append(locs, geojson.PointLocation{Point: n.pos, Fields: fields})
	}

	return locs
}

// System manages independent traffic signals.
type System struct {
	Signals        []Signal
	dirty          bool
	lightIntensity float32
}

// clusterPoints merges nearby points (within snapDist meters) into centroids.
func clusterPoints(points []geojson.Point2D, snapDist float32) []geojson.Point2D {
	type cluster struct {
		sumX, sumZ float32
		count      int
	}
	var clusters []cluster

	for _, pt := range points {
		merged := false
		for i := range clusters {
			cx := clusters[i].sumX / float32(clusters[i].count)
			cz := clusters[i].sumZ / float32(clusters[i].count)
			dx := cx - pt.X
			dz := cz - pt.Z
			if dx*dx+dz*dz < snapDist*snapDist {
				clusters[i].sumX += pt.X
				clusters[i].sumZ += pt.Z
				clusters[i].count++
				merged = true
				break
			}
		}
		if !merged {
			clusters = append(clusters, cluster{sumX: pt.X, sumZ: pt.Z, count: 1})
		}
	}

	result := make([]geojson.Point2D, len(clusters))
	for i, c := range clusters {
		result[i] = geojson.Point2D{
			X: c.sumX / float32(c.count),
			Z: c.sumZ / float32(c.count),
		}
	}
	return result
}

// NewFromPoints creates a traffic system from raw point positions (e.g. OSM).
// Nearby points are clustered into single intersections (OSM often has 2-4 nodes
// per intersection, one per traffic direction). Street names are derived from
// the two nearest distinct-named centerline segments.
func NewFromPoints(points []geojson.Point2D, lightIntensity float32, streets []geojson.StreetSegment) *System {
	// Cluster nearby OSM nodes into single intersection points
	merged := clusterPoints(points, 20)
	fmt.Printf("Clustered %d OSM nodes into %d intersections\n", len(points), len(merged))

	signals := make([]Signal, len(merged))
	for i, pt := range merged {
		pos := pt
		var dirAngle float32
		var street1, street2 string

		if len(streets) > 0 {
			pos, dirAngle = positionAndDirection(pt, streets)
			street1, street2 = nearestStreetNames(pt, streets)
		}

		offset := rand.Float32() * coordCycleDuration
		sig := Signal{
			Position: pos,
			Street1:  street1,
			Street2:  street2,
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

// NewFromMapData creates a traffic system directly from pre-processed intersection data.
// Position and direction are already computed (no clustering/snapping needed).
func NewFromMapData(intersections []MapIntersection, lightIntensity float32) *System {
	signals := make([]Signal, len(intersections))
	for i, d := range intersections {
		sig := Signal{
			ID:       d.ID,
			Position: geojson.Point2D{X: d.Position[0], Z: d.Position[1]},
			Street1:  d.Street1,
			Street2:  d.Street2,
			DirAngle: d.DirectionDeg * math.Pi / 180,
		}
		sig.advance(d.CycleOffsetSec)
		signals[i] = sig
	}
	return &System{
		Signals:        signals,
		dirty:          true,
		lightIntensity: lightIntensity,
	}
}

// MapIntersection holds the data needed to create a Signal from map data.
type MapIntersection struct {
	ID             string
	Position       [2]float32
	Street1        string
	Street2        string
	DirectionDeg   float32
	CycleOffsetSec float32
}

// nearestStreetNames finds the two closest segments with distinct names.
func nearestStreetNames(pt geojson.Point2D, streets []geojson.StreetSegment) (string, string) {
	type segDist struct {
		name string
		dist float32
	}
	var closest []segDist

	for _, seg := range streets {
		if seg.Name == "" {
			continue
		}
		for j := 0; j < len(seg.Points)-1; j++ {
			d, _ := pointToSegmentInfo(pt, seg.Points[j], seg.Points[j+1])
			closest = append(closest, segDist{seg.Name, d})
		}
	}

	// Find nearest, then nearest with a different name
	var name1, name2 string
	best1 := float32(math.MaxFloat32)
	best2 := float32(math.MaxFloat32)
	for _, c := range closest {
		if c.dist < best1 {
			best1 = c.dist
			name1 = c.name
		}
	}
	for _, c := range closest {
		if c.name != name1 && c.dist < best2 {
			best2 = c.dist
			name2 = c.name
		}
	}
	return name1, name2
}

// positionAndDirection finds the nearest street segment, offsets the point
// to the curb, and returns the segment direction angle.
func positionAndDirection(pt geojson.Point2D, streets []geojson.StreetSegment) (geojson.Point2D, float32) {
	bestDist := float32(math.MaxFloat32)
	var bestPerp geojson.Point2D
	var bestTrafDir string
	var bestDirAngle float32

	for _, seg := range streets {
		for j := 0; j < len(seg.Points)-1; j++ {
			a := seg.Points[j]
			b := seg.Points[j+1]

			dist, perp := pointToSegmentInfo(pt, a, b)
			if dist < bestDist {
				bestDist = dist
				bestPerp = perp
				bestTrafDir = seg.TrafDir
				// Segment direction angle (digitization direction: A→B)
				dx := b.X - a.X
				dz := b.Z - a.Z
				bestDirAngle = float32(math.Atan2(float64(dx), float64(dz)))
			}
		}
	}

	if bestDist > 50 {
		return pt, 0
	}

	// Signal faces AGAINST traffic so approaching drivers see the bulbs.
	// Digitization angle points A→B.
	// "FT" traffic flows A→B → flip π to face B→A (against traffic)
	// "TF" traffic flows B→A → A→B already faces against traffic, no flip
	// "TW" two-way → flip π (arbitrary but consistent)
	if bestTrafDir != "TF" {
		bestDirAngle += math.Pi
	}

	off := oneWayOffset
	if bestTrafDir == "TW" {
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

// SetDirty marks the system as needing a light uniform rebuild.
func (s *System) SetDirty() {
	s.dirty = true
}

// Dirty returns true if any signal changed phase since last call.
func (s *System) Dirty() bool {
	if s.dirty {
		s.dirty = false
		return true
	}
	return false
}

// Lights returns 6 point lights per signal (3 per directional head).
// Only the active light in each head has intensity; the others are zero.
func (s *System) Lights() []scene.PointLight {
	lights := make([]scene.PointLight, 0, len(s.Signals)*6)
	for _, sig := range s.Signals {
		heads := sig.Heads()
		for _, h := range heads {
			// Offset light position forward from housing
			sinA := float32(math.Sin(float64(h.Angle)))
			cosA := float32(math.Cos(float64(h.Angle)))
			lx := h.X + sinA*LightForward
			lz := h.Z + cosA*LightForward

			var redI, yellowI, greenI float32
			switch h.Phase {
			case Red:
				redI = s.lightIntensity
			case Yellow:
				yellowI = s.lightIntensity
			case Green:
				greenI = s.lightIntensity
			}

			lights = append(lights,
				scene.PointLight{
					Position:  mgl32.Vec3{lx, RedY, lz},
					Color:     mgl32.Vec3{1.0, 0.1, 0.0},
					Intensity: redI,
				},
				scene.PointLight{
					Position:  mgl32.Vec3{lx, YellowY, lz},
					Color:     mgl32.Vec3{1.0, 0.9, 0.0},
					Intensity: yellowI,
				},
				scene.PointLight{
					Position:  mgl32.Vec3{lx, GreenY, lz},
					Color:     mgl32.Vec3{0.0, 1.0, 0.3},
					Intensity: greenI,
				},
			)
		}
	}
	return lights
}
