package admin

import (
	"math"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/camera"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/traffic"
)

type EntityType int

const (
	EntityNone EntityType = iota
	EntityBuilding
	EntitySignal
)

type Selection struct {
	Type       EntityType
	BuildingID building.BuildingID
	SignalIdx  int
	Position   mgl32.Vec3
	Distance   float32
}

type Mode struct {
	active    bool
	selection Selection
	panel     *InfoPanel
	rend      *renderer.Renderer
}

func New(r *renderer.Renderer, pixelScale int) *Mode {
	return &Mode{
		selection: Selection{SignalIdx: -1},
		panel:     newInfoPanel(r, pixelScale),
		rend:      r,
	}
}

func (m *Mode) Toggle() {
	m.active = !m.active
	if !m.active {
		m.ClearSelection()
	}
}

func (m *Mode) IsActive() bool {
	return m.active
}

func (m *Mode) Selection() Selection {
	return m.selection
}

func (m *Mode) SelectedBuildingID() building.BuildingID {
	if m.selection.Type == EntityBuilding {
		return m.selection.BuildingID
	}
	return 0
}

func (m *Mode) SelectedSignalIdx() int {
	if m.selection.Type == EntitySignal {
		return m.selection.SignalIdx
	}
	return -1
}

func (m *Mode) ClearSelection() {
	m.selection = Selection{SignalIdx: -1}
	m.panel.clearValues(m.rend)
}

// Update raycasts from screen center each frame and updates the selection.
func (m *Mode) Update(cam *camera.Camera, grid *scene.SpatialGrid, objects []scene.Object, reg *building.Registry, trafficSys *traffic.System) {
	// Ray from camera position along camera forward (center of screen)
	origin := cam.Position
	dir := cam.Forward()

	bestDist := float32(math.MaxFloat32)
	bestSel := Selection{SignalIdx: -1}

	// Phase 1: collect sphere-hit building candidates
	type candidate struct {
		bid      building.BuildingID
		position mgl32.Vec3
	}
	var candidates []candidate

	nearby := grid.QueryRadius(cam.Position.X(), cam.Position.Z(), 200)
	for _, idx := range nearby {
		obj := objects[idx]
		if obj.BuildingID == 0 {
			continue
		}
		bid := building.BuildingID(obj.BuildingID - 1)
		b := reg.Get(bid)
		if b == nil {
			continue
		}
		h := b.Footprint.Height
		halfH := h / 2
		center := mgl32.Vec3{obj.Position.X(), halfH, obj.Position.Z()}
		origR := obj.Radius
		xzRSq := origR*origR - h*h
		if xzRSq < 0 {
			xzRSq = 0
		}
		pickRadius := float32(math.Sqrt(float64(xzRSq + halfH*halfH)))

		_, hit := camera.RaySphereIntersect(origin, dir, center, pickRadius)
		if hit {
			candidates = append(candidates, candidate{bid: bid, position: obj.Position})
		}
	}

	// Phase 2: ray-triangle test on sphere-hit candidates
	for _, c := range candidates {
		b := reg.Get(c.bid)
		if b == nil || b.Raw == nil {
			continue
		}
		// Transform ray to building-local space (vertices are centroid-relative)
		localOrigin := origin.Sub(c.position)
		verts := b.Raw.Vertices
		indices := b.Raw.Indices
		for i := 0; i+2 < len(indices); i += 3 {
			v0 := mgl32.Vec3{verts[indices[i]].X, verts[indices[i]].Y, verts[indices[i]].Z}
			v1 := mgl32.Vec3{verts[indices[i+1]].X, verts[indices[i+1]].Y, verts[indices[i+1]].Z}
			v2 := mgl32.Vec3{verts[indices[i+2]].X, verts[indices[i+2]].Y, verts[indices[i+2]].Z}
			t, hit := camera.RayTriangleIntersect(localOrigin, dir, v0, v1, v2)
			if hit && t < bestDist {
				bestDist = t
				bestSel = Selection{
					Type:       EntityBuilding,
					BuildingID: c.bid,
					SignalIdx:  -1,
					Position:   c.position,
					Distance:   t,
				}
			}
		}
	}

	// Test traffic signals
	if trafficSys != nil {
		for i, sig := range trafficSys.Signals {
			center := mgl32.Vec3{sig.Position.X, traffic.PoleHeight / 2, sig.Position.Z}
			t, hit := camera.RaySphereIntersect(origin, dir, center, 5)
			if hit && t < bestDist {
				bestDist = t
				bestSel = Selection{
					Type:      EntitySignal,
					SignalIdx: i,
					Position:  center,
					Distance:  t,
				}
			}
		}
	}

	// Only rebuild panel meshes when selection actually changes
	changed := bestSel.Type != m.selection.Type ||
		bestSel.BuildingID != m.selection.BuildingID ||
		bestSel.SignalIdx != m.selection.SignalIdx

	m.selection = bestSel

	if !changed {
		return
	}

	if bestSel.Type == EntityBuilding {
		b := reg.Get(bestSel.BuildingID)
		if b != nil {
			m.panel.setBuildingValues(m.rend, b.BBL, b.PLUTO.Address, b.PLUTO.BldgClass, b.PLUTO.LandUse, b.PLUTO.YearBuilt, b.PLUTO.NumFloors)
		}
	} else if bestSel.Type == EntitySignal {
		sig := trafficSys.Signals[bestSel.SignalIdx]
		m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
	} else {
		m.panel.clearValues(m.rend)
	}
}

func (m *Mode) Render(r *renderer.Renderer, cmdBuf *sdl.GPUCommandBuffer, swapchainTex *sdl.GPUTexture, screenW, screenH int) {
	m.panel.render(r, cmdBuf, swapchainTex, screenW, screenH, m.selection)
}

func (m *Mode) Destroy(r *renderer.Renderer) {
	m.panel.destroy(r)
}
