package admin

import (
	"math"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/camera"
	"github.com/anthonyrego/construct/pkg/doodad"
	"github.com/anthonyrego/construct/pkg/input"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/traffic"
)

type EntityType int

const (
	EntityNone EntityType = iota
	EntityBuilding
	EntitySignal
	EntityTree
	EntityHydrant
	EntityDoodad
)

func (s Selection) DoodadType() string {
	switch s.Type {
	case EntityTree:
		return "tree"
	case EntityHydrant:
		return "hydrant"
	default:
		return ""
	}
}

func doodadEntityType(typeName string) EntityType {
	switch typeName {
	case "tree":
		return EntityTree
	case "hydrant":
		return EntityHydrant
	default:
		return EntityDoodad
	}
}

type Selection struct {
	Type       EntityType
	BuildingID building.BuildingID
	SignalIdx  int
	DoodadIdx  int
	DoodadID   string
	Position   mgl32.Vec3
	Distance   float32
}

type Mode struct {
	active    bool
	selection Selection
	panel     *InfoPanel
	rend      *renderer.Renderer

	// Placement mode: entity follows crosshair on Y=0 plane
	placing      bool
	placeOriginX float32
	placeOriginZ float32

	// Editing state
	dirtyBlocks  map[string]bool
	dirtyInts    map[string]bool
	dirtyDoodads map[string]bool
	undoStack    []undoEntry
}

func New(r *renderer.Renderer, pixelScale int) *Mode {
	m := &Mode{
		selection: Selection{SignalIdx: -1, DoodadIdx: -1},
		panel:     newInfoPanel(r, pixelScale),
		rend:      r,
	}
	m.initEditing()
	return m
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
	m.selection = Selection{SignalIdx: -1, DoodadIdx: -1}
	m.placing = false
	m.panel.clearValues(m.rend)
}

// Update handles look-to-select and placement mode in admin mode.
// Uses screen-center raycast (cam.Position + cam.Forward()) for both selection and placement.
func (m *Mode) Update(cam *camera.Camera, grid *scene.SpatialGrid, objects []scene.Object,
	reg *building.Registry, trafficSys *traffic.System,
	doodads map[string]*doodad.System,
	inp *input.Input) {

	origin := cam.Position
	dir := cam.Forward()

	// Placement mode: entity follows crosshair on Y=0 plane
	if m.placing {
		hitX, hitZ, ok := rayToGroundPlane(origin, dir, 0)
		if ok {
			m.setPosition(hitX, hitZ, doodads, trafficSys)
		}
		return // confirm/cancel handled in HandleEdit (which has store access)
	}

	// Left-click: select entity at crosshair
	if inp.IsMouseLeftPressed() {
		bestSel := m.hitTest(origin, dir, cam.Position, grid, objects, reg, trafficSys, doodads)

		changed := bestSel.Type != m.selection.Type ||
			bestSel.BuildingID != m.selection.BuildingID ||
			bestSel.SignalIdx != m.selection.SignalIdx ||
			bestSel.DoodadIdx != m.selection.DoodadIdx ||
			bestSel.DoodadID != m.selection.DoodadID

		m.selection = bestSel

		if changed {
			m.updatePanel(reg, trafficSys, doodads)
		}
	}
}

// hitTest raycasts against all entity types and returns the closest hit.
func (m *Mode) hitTest(origin, dir, camPos mgl32.Vec3, grid *scene.SpatialGrid, objects []scene.Object,
	reg *building.Registry, trafficSys *traffic.System,
	doodads map[string]*doodad.System) Selection {

	bestDist := float32(math.MaxFloat32)
	bestSel := Selection{SignalIdx: -1, DoodadIdx: -1}

	// Phase 1: collect sphere-hit building candidates
	type candidate struct {
		bid      building.BuildingID
		position mgl32.Vec3
	}
	var candidates []candidate

	nearby := grid.QueryRadius(camPos.X(), camPos.Z(), 200)
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
					DoodadIdx:  -1,
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
					DoodadIdx: -1,
					Position:  center,
					Distance:  t,
				}
			}
		}
	}

	// Test doodads
	for typeName, sys := range doodads {
		if sys == nil {
			continue
		}
		for i := range sys.Instances {
			inst := &sys.Instances[i]
			dx := inst.X - camPos.X()
			dz := inst.Z - camPos.Z()
			if dx*dx+dz*dz > 100*100 {
				continue
			}
			centerY, pickR := sys.CullSphere(i)
			center := mgl32.Vec3{inst.X, centerY, inst.Z}
			tt, hit := camera.RaySphereIntersect(origin, dir, center, pickR)
			if hit && tt < bestDist {
				bestDist = tt
				bestSel = Selection{
					Type:      doodadEntityType(typeName),
					SignalIdx: -1,
					DoodadIdx: i,
					DoodadID:  inst.ID,
					Position:  center,
					Distance:  tt,
				}
			}
		}
	}

	return bestSel
}

// updatePanel refreshes the info panel for the current selection.
func (m *Mode) updatePanel(reg *building.Registry, trafficSys *traffic.System,
	doodads map[string]*doodad.System) {
	switch m.selection.Type {
	case EntityBuilding:
		b := reg.Get(m.selection.BuildingID)
		if b != nil {
			m.panel.setBuildingValues(m.rend, b.BBL, b.PLUTO.Address, b.PLUTO.BldgClass, b.PLUTO.LandUse, b.PLUTO.YearBuilt, b.PLUTO.NumFloors, b.Footprint.Height, b.Hidden)
		}
	case EntitySignal:
		if trafficSys != nil && m.selection.SignalIdx >= 0 && m.selection.SignalIdx < len(trafficSys.Signals) {
			sig := trafficSys.Signals[m.selection.SignalIdx]
			m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
		}
	case EntityTree, EntityHydrant, EntityDoodad:
		typeName := m.selection.DoodadType()
		if sys := doodads[typeName]; sys != nil && m.selection.DoodadIdx >= 0 && m.selection.DoodadIdx < len(sys.Instances) {
			inst := &sys.Instances[m.selection.DoodadIdx]
			m.panel.setDoodadValues(m.rend, sys.TypeName, inst.ID, inst.X, inst.Z)
		}
	default:
		m.panel.clearValues(m.rend)
	}
}

// rayToGroundPlane intersects a ray with a horizontal plane at the given Y.
func rayToGroundPlane(origin, dir mgl32.Vec3, planeY float32) (x, z float32, ok bool) {
	if dir.Y() > -1e-6 && dir.Y() < 1e-6 {
		return 0, 0, false // parallel
	}
	t := (planeY - origin.Y()) / dir.Y()
	if t <= 0 {
		return 0, 0, false // behind camera
	}
	return origin.X() + t*dir.X(), origin.Z() + t*dir.Z(), true
}

func (m *Mode) Render(r *renderer.Renderer, cmdBuf *sdl.GPUCommandBuffer, swapchainTex *sdl.GPUTexture, screenW, screenH int) {
	m.panel.render(r, cmdBuf, swapchainTex, screenW, screenH, m.selection, m.HasDirty(), m.placing)
}

// HandleEdit processes editing key presses and returns what action the caller should take.
func (m *Mode) HandleEdit(inp *input.Input, reg *building.Registry,
	trafficSys *traffic.System, scn *scene.Scene,
	grid *scene.SpatialGrid, store *mapdata.Store,
	doodads map[string]*doodad.System) EditAction {

	// Placement mode: confirm or cancel
	if m.placing {
		if inp.IsMouseLeftPressed() {
			m.finalizePlace(doodads, trafficSys, store)
			return EditDirty
		}
		if inp.IsKeyPressed(sdl.K_ESCAPE) {
			m.cancelPlace(doodads, trafficSys)
			return EditNone
		}
		return EditNone
	}

	if m.selection.Type == EntityNone {
		// Only check Cmd+S / Cmd+Z even without selection
		cmdHeld := inp.IsKeyDown(sdl.K_LGUI) || inp.IsKeyDown(sdl.K_LCTRL)
		if cmdHeld && inp.IsKeyPressed(sdl.K_S) {
			return EditSave
		}
		if cmdHeld && inp.IsKeyPressed(sdl.K_Z) {
			if m.undo(reg, scn.Objects, grid, trafficSys, store, doodads) {
				return EditDirty
			}
			return EditNone
		}
		return EditNone
	}

	shift := inp.IsKeyDown(sdl.K_LSHIFT) || inp.IsKeyDown(sdl.K_RSHIFT)
	cmdHeld := inp.IsKeyDown(sdl.K_LGUI) || inp.IsKeyDown(sdl.K_LCTRL)

	// Cmd+S: save
	if cmdHeld && inp.IsKeyPressed(sdl.K_S) {
		return EditSave
	}

	// Cmd+Z: undo
	if cmdHeld && inp.IsKeyPressed(sdl.K_Z) {
		if m.undo(reg, scn.Objects, grid, trafficSys, store, doodads) {
			return EditDirty
		}
		return EditNone
	}

	// Doodad edits: arrow keys nudge position
	if m.selection.Type == EntityTree || m.selection.Type == EntityHydrant || m.selection.Type == EntityDoodad {
		step := float32(0.5)
		if shift {
			step = 0.05
		}
		if inp.IsKeyPressed(sdl.K_UP) {
			m.nudgePosition(0, -step, doodads, trafficSys, store)
			return EditNone
		}
		if inp.IsKeyPressed(sdl.K_DOWN) {
			m.nudgePosition(0, step, doodads, trafficSys, store)
			return EditNone
		}
		if inp.IsKeyPressed(sdl.K_LEFT) {
			m.nudgePosition(-step, 0, doodads, trafficSys, store)
			return EditNone
		}
		if inp.IsKeyPressed(sdl.K_RIGHT) {
			m.nudgePosition(step, 0, doodads, trafficSys, store)
			return EditNone
		}
	}

	// Building edits
	if m.selection.Type == EntityBuilding {
		if inp.IsKeyPressed(sdl.K_UP) {
			delta := float32(1.0)
			if shift {
				delta = 0.1
			}
			m.adjustHeight(delta, reg, scn.Objects, grid, store)
			return EditNone
		}
		if inp.IsKeyPressed(sdl.K_DOWN) {
			delta := float32(-1.0)
			if shift {
				delta = -0.1
			}
			m.adjustHeight(delta, reg, scn.Objects, grid, store)
			return EditNone
		}
		if inp.IsKeyPressed(sdl.K_V) {
			m.toggleVisibility(reg, scn.Objects, grid, store)
			return EditNone
		}
	}

	// G key: enter placement mode for any movable entity
	if inp.IsKeyPressed(sdl.K_G) {
		if m.selection.Type == EntityTree || m.selection.Type == EntityHydrant || m.selection.Type == EntityDoodad || m.selection.Type == EntitySignal {
			m.enterPlace(doodads, trafficSys)
			return EditNone
		}
	}

	// Signal edits: arrows rotate direction
	if m.selection.Type == EntitySignal && trafficSys != nil {
		if inp.IsKeyPressed(sdl.K_RIGHT) {
			delta := float32(5.0)
			if shift {
				delta = 1.0
			}
			m.rotateSignal(delta, trafficSys, store)
			return EditDirty
		}
		if inp.IsKeyPressed(sdl.K_LEFT) {
			delta := float32(-5.0)
			if shift {
				delta = -1.0
			}
			m.rotateSignal(delta, trafficSys, store)
			return EditDirty
		}
	}

	return EditNone
}

func (m *Mode) Destroy(r *renderer.Renderer) {
	m.panel.destroy(r)
}
