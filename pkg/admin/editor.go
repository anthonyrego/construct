package admin

import (
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/building"
	"github.com/anthonyrego/construct/pkg/doodad"
	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/scene"
	"github.com/anthonyrego/construct/pkg/traffic"
)

// EditAction indicates what the caller should do after HandleEdit.
type EditAction int

const (
	EditNone  EditAction = iota
	EditDirty            // something changed, rebuild lights if signal
	EditSave             // Cmd+S: save + reload
)

const maxUndoSize = 20

type undoKind int

const (
	undoDefault  undoKind = iota
	undoPosition
)

type undoEntry struct {
	entityType EntityType
	kind       undoKind
	// Building
	buildingID building.BuildingID
	blockID    string
	oldHeight  float32
	oldHidden  bool
	// Signal
	signalIdx int
	intID     string
	oldDirDeg float32
	// Position (doodad or signal move)
	oldPosX    float32
	oldPosZ    float32
	doodadIdx  int
	doodadID   string
	doodadType string // "tree", "hydrant", or "" for signal
}

func (m *Mode) initEditing() {
	m.dirtyBlocks = make(map[string]bool)
	m.dirtyInts = make(map[string]bool)
	m.dirtyDoodads = make(map[string]bool)
	m.undoStack = nil
}

func (m *Mode) pushUndo(e undoEntry) {
	m.undoStack = append(m.undoStack, e)
	if len(m.undoStack) > maxUndoSize {
		m.undoStack = m.undoStack[1:]
	}
}

func (m *Mode) adjustHeight(delta float32, reg *building.Registry, objects []scene.Object, grid *scene.SpatialGrid, store *mapdata.Store) {
	if m.selection.Type != EntityBuilding {
		return
	}
	b := reg.Get(m.selection.BuildingID)
	if b == nil {
		return
	}

	// Push undo
	storeB, blockID := store.FindBuildingByBBL(b.BBL)
	m.pushUndo(undoEntry{
		entityType: EntityBuilding,
		buildingID: m.selection.BuildingID,
		blockID:    blockID,
		oldHeight:  b.Footprint.Height,
		oldHidden:  b.Hidden,
	})

	// Modify height
	newH := b.Footprint.Height + delta
	if newH < 1.0 {
		newH = 1.0
	}
	b.Footprint.Height = newH

	// Rebuild mesh
	m.rebuildBuildingMesh(b, reg, objects, grid)

	// Update store
	if storeB != nil {
		storeB.Height = newH
		m.dirtyBlocks[blockID] = true
	}

	// Update panel to reflect new height
	m.panel.setBuildingValues(m.rend, b.BBL, b.PLUTO.Address, b.PLUTO.BldgClass,
		b.PLUTO.LandUse, b.PLUTO.YearBuilt, b.PLUTO.NumFloors, b.Footprint.Height, b.Hidden)
}

func (m *Mode) toggleVisibility(reg *building.Registry, objects []scene.Object, grid *scene.SpatialGrid, store *mapdata.Store) {
	if m.selection.Type != EntityBuilding {
		return
	}
	b := reg.Get(m.selection.BuildingID)
	if b == nil {
		return
	}

	storeB, blockID := store.FindBuildingByBBL(b.BBL)
	m.pushUndo(undoEntry{
		entityType: EntityBuilding,
		buildingID: m.selection.BuildingID,
		blockID:    blockID,
		oldHeight:  b.Footprint.Height,
		oldHidden:  b.Hidden,
	})

	b.Hidden = !b.Hidden

	// Update scene object
	obj := findSceneObject(objects, m.selection.BuildingID)
	if obj != nil {
		obj.Hidden = b.Hidden
	}

	// Rebuild cell mesh (excludes hidden buildings)
	m.rebuildCell(b.CellKey, reg, grid)

	// Update store
	if storeB != nil {
		storeB.Visible = !b.Hidden
		m.dirtyBlocks[blockID] = true
	}

	// Update panel
	m.panel.setBuildingValues(m.rend, b.BBL, b.PLUTO.Address, b.PLUTO.BldgClass,
		b.PLUTO.LandUse, b.PLUTO.YearBuilt, b.PLUTO.NumFloors, b.Footprint.Height, b.Hidden)
}

func (m *Mode) rotateSignal(deltaDeg float32, trafficSys *traffic.System, store *mapdata.Store) {
	if m.selection.Type != EntitySignal || m.selection.SignalIdx < 0 {
		return
	}
	sig := &trafficSys.Signals[m.selection.SignalIdx]

	m.pushUndo(undoEntry{
		entityType: EntitySignal,
		signalIdx:  m.selection.SignalIdx,
		intID:      sig.ID,
		oldDirDeg:  sig.DirAngle * 180 / math.Pi,
	})

	sig.DirAngle += deltaDeg * math.Pi / 180
	trafficSys.SetDirty()

	// Update store
	if sig.ID != "" {
		if d, ok := store.Intersections[sig.ID]; ok {
			d.DirectionDeg = sig.DirAngle * 180 / math.Pi
			m.dirtyInts[sig.ID] = true
		}
	}

	// Update panel
	m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
}

// enterPlace records the entity's current position and enters placement mode.
func (m *Mode) enterPlace(doodads map[string]*doodad.System, trafficSys *traffic.System) {
	sel := m.selection
	switch sel.Type {
	case EntityTree, EntityHydrant, EntityDoodad:
		typeName := sel.DoodadType()
		sys := doodads[typeName]
		if sys == nil || sel.DoodadIdx < 0 || sel.DoodadIdx >= len(sys.Instances) {
			return
		}
		inst := sys.Instances[sel.DoodadIdx]
		m.placeOriginX = inst.X
		m.placeOriginZ = inst.Z
	case EntitySignal:
		if trafficSys == nil || sel.SignalIdx < 0 || sel.SignalIdx >= len(trafficSys.Signals) {
			return
		}
		sig := trafficSys.Signals[sel.SignalIdx]
		m.placeOriginX = sig.Position.X
		m.placeOriginZ = sig.Position.Z
	default:
		return
	}
	m.placing = true
}

// cancelPlace restores the entity's original position and exits placement mode.
func (m *Mode) cancelPlace(doodads map[string]*doodad.System, trafficSys *traffic.System) {
	m.setPosition(m.placeOriginX, m.placeOriginZ, doodads, trafficSys)
	m.placing = false
}

// setPosition moves the selected entity to an absolute position (used during placement).
// Does not push undo or mark dirty — that happens on finalizePlace.
func (m *Mode) setPosition(x, z float32,
	doodads map[string]*doodad.System,
	trafficSys *traffic.System) {
	sel := m.selection
	switch sel.Type {
	case EntityTree, EntityHydrant, EntityDoodad:
		typeName := sel.DoodadType()
		sys := doodads[typeName]
		if sys == nil || sel.DoodadIdx < 0 || sel.DoodadIdx >= len(sys.Instances) {
			return
		}
		inst := &sys.Instances[sel.DoodadIdx]
		inst.X = x
		inst.Z = z
		m.panel.setDoodadValues(m.rend, sys.TypeName, inst.ID, inst.X, inst.Z)
	case EntitySignal:
		if trafficSys == nil || sel.SignalIdx < 0 || sel.SignalIdx >= len(trafficSys.Signals) {
			return
		}
		sig := &trafficSys.Signals[sel.SignalIdx]
		sig.Position.X = x
		sig.Position.Z = z
		trafficSys.SetDirty()
		m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
	}
}

// finalizePlace pushes an undo entry, syncs the final position to the store, and marks dirty.
func (m *Mode) finalizePlace(doodads map[string]*doodad.System,
	trafficSys *traffic.System, store *mapdata.Store) {
	sel := m.selection
	switch sel.Type {
	case EntityTree, EntityHydrant, EntityDoodad:
		typeName := sel.DoodadType()
		sys := doodads[typeName]
		if sys == nil || sel.DoodadIdx < 0 || sel.DoodadIdx >= len(sys.Instances) {
			return
		}
		inst := &sys.Instances[sel.DoodadIdx]
		m.pushUndo(undoEntry{
			entityType: sel.Type,
			kind:       undoPosition,
			oldPosX:    m.placeOriginX,
			oldPosZ:    m.placeOriginZ,
			doodadIdx:  sel.DoodadIdx,
			doodadID:   sel.DoodadID,
			doodadType: typeName,
		})
		if store != nil {
			if item, _ := store.FindDoodadByID(typeName, sel.DoodadID); item != nil {
				item.Position[0] = inst.X
				item.Position[1] = inst.Z
				m.dirtyDoodads[typeName] = true
			}
		}
	case EntitySignal:
		if trafficSys == nil || sel.SignalIdx < 0 || sel.SignalIdx >= len(trafficSys.Signals) {
			return
		}
		sig := &trafficSys.Signals[sel.SignalIdx]
		m.pushUndo(undoEntry{
			entityType: EntitySignal,
			kind:       undoPosition,
			signalIdx:  sel.SignalIdx,
			intID:      sig.ID,
			oldPosX:    m.placeOriginX,
			oldPosZ:    m.placeOriginZ,
		})
		if store != nil && sig.ID != "" {
			if d, ok := store.Intersections[sig.ID]; ok {
				d.Position[0] = sig.Position.X
				d.Position[1] = sig.Position.Z
				m.dirtyInts[sig.ID] = true
			}
		}
	}
	m.placing = false
}

func (m *Mode) nudgePosition(dx, dz float32,
	doodads map[string]*doodad.System,
	trafficSys *traffic.System, store *mapdata.Store) {

	sel := m.selection

	switch sel.Type {
	case EntityTree, EntityHydrant, EntityDoodad:
		typeName := sel.DoodadType()
		sys := doodads[typeName]
		if sys == nil || sel.DoodadIdx < 0 || sel.DoodadIdx >= len(sys.Instances) {
			return
		}
		inst := &sys.Instances[sel.DoodadIdx]
		m.pushUndo(undoEntry{
			entityType: sel.Type,
			kind:       undoPosition,
			oldPosX:    inst.X,
			oldPosZ:    inst.Z,
			doodadIdx:  sel.DoodadIdx,
			doodadID:   sel.DoodadID,
			doodadType: typeName,
		})
		inst.X += dx
		inst.Z += dz
		// Update store
		if item, _ := store.FindDoodadByID(typeName, sel.DoodadID); item != nil {
			item.Position[0] = inst.X
			item.Position[1] = inst.Z
			m.dirtyDoodads[typeName] = true
		}
		m.panel.setDoodadValues(m.rend, sys.TypeName, inst.ID, inst.X, inst.Z)

	case EntitySignal:
		if trafficSys == nil || sel.SignalIdx < 0 || sel.SignalIdx >= len(trafficSys.Signals) {
			return
		}
		sig := &trafficSys.Signals[sel.SignalIdx]
		m.pushUndo(undoEntry{
			entityType: EntitySignal,
			kind:       undoPosition,
			signalIdx:  sel.SignalIdx,
			intID:      sig.ID,
			oldPosX:    sig.Position.X,
			oldPosZ:    sig.Position.Z,
		})
		sig.Position.X += dx
		sig.Position.Z += dz
		trafficSys.SetDirty()
		// Update store
		if sig.ID != "" {
			if d, ok := store.Intersections[sig.ID]; ok {
				d.Position[0] = sig.Position.X
				d.Position[1] = sig.Position.Z
				m.dirtyInts[sig.ID] = true
			}
		}
		m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
	}
}

func (m *Mode) undo(reg *building.Registry, objects []scene.Object, grid *scene.SpatialGrid,
	trafficSys *traffic.System, store *mapdata.Store,
	doodads map[string]*doodad.System) bool {
	if len(m.undoStack) == 0 {
		return false
	}
	e := m.undoStack[len(m.undoStack)-1]
	m.undoStack = m.undoStack[:len(m.undoStack)-1]

	switch e.entityType {
	case EntityBuilding:
		b := reg.Get(e.buildingID)
		if b == nil {
			return false
		}
		b.Footprint.Height = e.oldHeight
		b.Hidden = e.oldHidden

		// Rebuild mesh
		m.rebuildBuildingMesh(b, reg, objects, grid)

		// Update scene object hidden state
		obj := findSceneObject(objects, e.buildingID)
		if obj != nil {
			obj.Hidden = b.Hidden
		}

		// Rebuild cell for hidden change
		m.rebuildCell(b.CellKey, reg, grid)

		// Update store
		if storeB, blockID := store.FindBuildingByBBL(b.BBL); storeB != nil {
			storeB.Height = e.oldHeight
			storeB.Visible = !e.oldHidden
			m.dirtyBlocks[blockID] = true
		}

		// Update panel if this building is still selected
		if m.selection.Type == EntityBuilding && m.selection.BuildingID == e.buildingID {
			m.panel.setBuildingValues(m.rend, b.BBL, b.PLUTO.Address, b.PLUTO.BldgClass,
				b.PLUTO.LandUse, b.PLUTO.YearBuilt, b.PLUTO.NumFloors, b.Footprint.Height, b.Hidden)
		}
		return false

	case EntitySignal:
		if trafficSys == nil || e.signalIdx < 0 || e.signalIdx >= len(trafficSys.Signals) {
			return false
		}
		sig := &trafficSys.Signals[e.signalIdx]

		if e.kind == undoPosition {
			sig.Position.X = e.oldPosX
			sig.Position.Z = e.oldPosZ
			trafficSys.SetDirty()
			if sig.ID != "" {
				if d, ok := store.Intersections[sig.ID]; ok {
					d.Position[0] = e.oldPosX
					d.Position[1] = e.oldPosZ
					m.dirtyInts[sig.ID] = true
				}
			}
		} else {
			sig.DirAngle = e.oldDirDeg * math.Pi / 180
			trafficSys.SetDirty()
			if sig.ID != "" {
				if d, ok := store.Intersections[sig.ID]; ok {
					d.DirectionDeg = e.oldDirDeg
					m.dirtyInts[sig.ID] = true
				}
			}
		}

		if m.selection.Type == EntitySignal && m.selection.SignalIdx == e.signalIdx {
			m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
		}
		return true // signal changed, rebuild lights

	case EntityTree, EntityHydrant, EntityDoodad:
		typeName := e.doodadType
		sys := doodads[typeName]
		if sys == nil || e.doodadIdx < 0 || e.doodadIdx >= len(sys.Instances) {
			return false
		}
		inst := &sys.Instances[e.doodadIdx]
		inst.X = e.oldPosX
		inst.Z = e.oldPosZ
		if item, _ := store.FindDoodadByID(typeName, e.doodadID); item != nil {
			item.Position[0] = e.oldPosX
			item.Position[1] = e.oldPosZ
			m.dirtyDoodads[typeName] = true
		}
		if (m.selection.Type == e.entityType) && m.selection.DoodadIdx == e.doodadIdx {
			m.panel.setDoodadValues(m.rend, sys.TypeName, inst.ID, inst.X, inst.Z)
		}
		return false
	}
	return false
}

func (m *Mode) rebuildBuildingMesh(b *building.Building, reg *building.Registry, objects []scene.Object, grid *scene.SpatialGrid) {
	raw, err := building.ExtrudeRaw(b.Footprint, b.Color.R, b.Color.G, b.Color.B)
	if err != nil {
		fmt.Println("ExtrudeRaw error:", err)
		return
	}
	if err := reg.ReplaceMesh(b.ID, raw); err != nil {
		fmt.Println("ReplaceMesh error:", err)
		return
	}

	// Update scene object
	obj := findSceneObject(objects, b.ID)
	if obj != nil {
		obj.Mesh = b.Mesh
		obj.Position = b.Position
		obj.Radius = b.Radius
	}

	// Rebuild cell mesh
	m.rebuildCell(b.CellKey, reg, grid)
}

func (m *Mode) rebuildCell(cellKey uint64, reg *building.Registry, grid *scene.SpatialGrid) {
	cell, err := reg.RebuildCellMesh(cellKey)
	if err != nil {
		fmt.Println("RebuildCellMesh error:", err)
		return
	}
	if cell != nil {
		grid.CellMeshes[cellKey] = &scene.CellMesh{Mesh: cell.Mesh, CellX: cell.CellX, CellZ: cell.CellZ}
	} else {
		delete(grid.CellMeshes, cellKey)
	}
}

// SaveDirty writes all modified blocks, intersections, and doodads to disk.
func (m *Mode) SaveDirty(store *mapdata.Store) error {
	for blockID := range m.dirtyBlocks {
		if err := store.SaveBlock(blockID); err != nil {
			return fmt.Errorf("saving block %s: %w", blockID, err)
		}
	}
	for intID := range m.dirtyInts {
		if err := store.SaveIntersection(intID); err != nil {
			return fmt.Errorf("saving intersection %s: %w", intID, err)
		}
	}
	for typ := range m.dirtyDoodads {
		if err := store.SaveDoodad(typ); err != nil {
			return fmt.Errorf("saving doodad %s: %w", typ, err)
		}
	}
	return nil
}

// HasDirty returns true if there are unsaved changes.
func (m *Mode) HasDirty() bool {
	return len(m.dirtyBlocks) > 0 || len(m.dirtyInts) > 0 || len(m.dirtyDoodads) > 0
}

// ResetEditing clears dirty state and undo stack (e.g. after external file change).
func (m *Mode) ResetEditing() {
	m.dirtyBlocks = make(map[string]bool)
	m.dirtyInts = make(map[string]bool)
	m.dirtyDoodads = make(map[string]bool)
	m.undoStack = nil
}

// Reselect finds and selects the same entity after a reload.
func (m *Mode) Reselect(bbl string, intID string, doodadID string, doodadType string,
	reg *building.Registry, trafficSys *traffic.System,
	doodads map[string]*doodad.System) {
	if bbl != "" {
		if bid, ok := reg.Lookup(bbl); ok {
			b := reg.Get(bid)
			if b != nil {
				m.selection = Selection{
					Type:       EntityBuilding,
					BuildingID: bid,
					SignalIdx:  -1,
					DoodadIdx:  -1,
					Position:   b.Position,
				}
				m.panel.setBuildingValues(m.rend, b.BBL, b.PLUTO.Address, b.PLUTO.BldgClass,
					b.PLUTO.LandUse, b.PLUTO.YearBuilt, b.PLUTO.NumFloors, b.Footprint.Height, b.Hidden)
				return
			}
		}
	}
	if intID != "" && trafficSys != nil {
		for i, sig := range trafficSys.Signals {
			if sig.ID == intID {
				m.selection = Selection{
					Type:      EntitySignal,
					SignalIdx: i,
					DoodadIdx: -1,
					Position:  mgl32.Vec3{sig.Position.X, traffic.PoleHeight / 2, sig.Position.Z},
				}
				m.panel.setSignalValues(m.rend, sig.Street1, sig.Street2, sig.DirAngle)
				return
			}
		}
	}
	if doodadID != "" && doodadType != "" {
		if sys := doodads[doodadType]; sys != nil {
			for i, inst := range sys.Instances {
				if inst.ID == doodadID {
					m.selection = Selection{
						Type:      doodadEntityType(doodadType),
						SignalIdx: -1,
						DoodadIdx: i,
						DoodadID:  doodadID,
						Position:  mgl32.Vec3{inst.X, 0, inst.Z},
					}
					m.updatePanel(reg, trafficSys, doodads)
					return
				}
			}
		}
	}
}

func findSceneObject(objects []scene.Object, bid building.BuildingID) *scene.Object {
	target := uint32(bid) + 1
	for i := range objects {
		if objects[i].BuildingID == target {
			return &objects[i]
		}
	}
	return nil
}
