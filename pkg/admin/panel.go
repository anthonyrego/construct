package admin

import (
	"fmt"
	"strings"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/sign"
	"github.com/anthonyrego/construct/pkg/ui"
)

// InfoPanel renders the admin mode indicator and entity details.
type InfoPanel struct {
	ps float32 // screen pixels per font pixel

	// Static label meshes (created once)
	modeLabel *mesh.Mesh // "ADMIN MODE"
	crosshair *mesh.Mesh // center-screen crosshair
	overlay   *mesh.Mesh // semi-transparent background quad

	buildingLabel *mesh.Mesh
	bblLabel      *mesh.Mesh
	addressLabel  *mesh.Mesh
	classLabel    *mesh.Mesh
	landUseLabel  *mesh.Mesh
	yearLabel     *mesh.Mesh
	floorsLabel   *mesh.Mesh
	heightLabel   *mesh.Mesh
	visibleLabel  *mesh.Mesh

	signalLabel  *mesh.Mesh
	street1Label *mesh.Mesh
	street2Label *mesh.Mesh
	dirLabel     *mesh.Mesh

	treeLabel     *mesh.Mesh
	hydrantLabel  *mesh.Mesh
	idLabel       *mesh.Mesh
	positionLabel *mesh.Mesh
	doodadHeightLabel *mesh.Mesh
	spreadLabel   *mesh.Mesh

	// Key hint labels
	bldgHint    *mesh.Mesh
	signalHint  *mesh.Mesh
	doodadHint  *mesh.Mesh
	placingHint *mesh.Mesh
	commonHint  *mesh.Mesh

	bldgHintW    float32
	signalHintW  float32
	doodadHintW  float32
	placingHintW float32
	commonHintW  float32

	maxTreeLabelW    float32
	maxHydrantLabelW float32

	// Dynamic value meshes (recreated on selection change)
	valueMeshes []*mesh.Mesh
	valueWidths []float32

	modeLabelWidth  float32
	dirtyLabelWidth float32
	dirtyLabel      *mesh.Mesh
	maxBldgLabelW   float32 // widest building label (including header)
	maxSignalLabelW float32 // widest signal label (including header)
}

func newInfoPanel(r *renderer.Renderer, pixelScale int) *InfoPanel {
	ps := float32(pixelScale)
	p := &InfoPanel{ps: ps}

	// Overlay quad (unit quad, scaled at render time)
	overlayVerts := []renderer.Vertex{
		{X: 0, Y: 0, Z: 0, R: 0, G: 0, B: 0, A: 180},
		{X: 1, Y: 0, Z: 0, R: 0, G: 0, B: 0, A: 180},
		{X: 1, Y: 1, Z: 0, R: 0, G: 0, B: 0, A: 180},
		{X: 0, Y: 1, Z: 0, R: 0, G: 0, B: 0, A: 180},
	}
	overlayIdx := []uint16{0, 1, 2, 0, 2, 3}
	vb, err := r.CreateVertexBuffer(overlayVerts)
	if err == nil {
		ib, err2 := r.CreateIndexBuffer(overlayIdx)
		if err2 == nil {
			p.overlay = &mesh.Mesh{VertexBuffer: vb, IndexBuffer: ib, IndexCount: 6}
		} else {
			r.ReleaseBuffer(vb)
		}
	}

	// Create static labels
	mkLabel := func(text string) (*mesh.Mesh, float32) {
		m, w, err := ui.NewTextMesh(r, text, ps, 180, 180, 180, 255)
		if err != nil {
			return nil, 0
		}
		return m, w
	}

	// Crosshair: small + shape centered at origin
	{
		s := ps // arm length in pixels
		t := ps * 0.5 // arm thickness
		if t < 1 {
			t = 1
		}
		cr, cg, cb, ca := uint8(255), uint8(255), uint8(255), uint8(180)
		chVerts := []renderer.Vertex{
			// Horizontal bar
			{X: -s, Y: -t / 2, Z: 0, R: cr, G: cg, B: cb, A: ca},
			{X: s, Y: -t / 2, Z: 0, R: cr, G: cg, B: cb, A: ca},
			{X: s, Y: t / 2, Z: 0, R: cr, G: cg, B: cb, A: ca},
			{X: -s, Y: t / 2, Z: 0, R: cr, G: cg, B: cb, A: ca},
			// Vertical bar
			{X: -t / 2, Y: -s, Z: 0, R: cr, G: cg, B: cb, A: ca},
			{X: t / 2, Y: -s, Z: 0, R: cr, G: cg, B: cb, A: ca},
			{X: t / 2, Y: s, Z: 0, R: cr, G: cg, B: cb, A: ca},
			{X: -t / 2, Y: s, Z: 0, R: cr, G: cg, B: cb, A: ca},
		}
		chIdx := []uint16{0, 1, 2, 0, 2, 3, 4, 5, 6, 4, 6, 7}
		cvb, err := r.CreateVertexBuffer(chVerts)
		if err == nil {
			cib, err2 := r.CreateIndexBuffer(chIdx)
			if err2 == nil {
				p.crosshair = &mesh.Mesh{VertexBuffer: cvb, IndexBuffer: cib, IndexCount: 12}
			} else {
				r.ReleaseBuffer(cvb)
			}
		}
	}

	p.modeLabel, p.modeLabelWidth, _ = ui.NewTextMesh(r, "ADMIN MODE", ps, 255, 200, 100, 255)
	p.dirtyLabel, p.dirtyLabelWidth, _ = ui.NewTextMesh(r, "*", ps, 255, 100, 100, 255)

	var w float32
	p.buildingLabel, w = mkLabel("BUILDING")
	p.maxBldgLabelW = w
	p.bblLabel, w = mkLabel("BBL")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.addressLabel, w = mkLabel("ADDRESS")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.classLabel, w = mkLabel("CLASS")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.landUseLabel, w = mkLabel("LAND USE")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.yearLabel, w = mkLabel("YEAR")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.floorsLabel, w = mkLabel("FLOORS")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.heightLabel, w = mkLabel("HEIGHT")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }
	p.visibleLabel, w = mkLabel("VISIBLE")
	if w > p.maxBldgLabelW { p.maxBldgLabelW = w }

	p.signalLabel, w = mkLabel("SIGNAL")
	p.maxSignalLabelW = w
	p.street1Label, w = mkLabel("STREET 1")
	if w > p.maxSignalLabelW { p.maxSignalLabelW = w }
	p.street2Label, w = mkLabel("STREET 2")
	if w > p.maxSignalLabelW { p.maxSignalLabelW = w }
	p.dirLabel, w = mkLabel("DIRECTION")
	if w > p.maxSignalLabelW { p.maxSignalLabelW = w }

	p.treeLabel, w = mkLabel("TREE")
	p.maxTreeLabelW = w
	p.hydrantLabel, w = mkLabel("HYDRANT")
	p.maxHydrantLabelW = w
	p.idLabel, w = mkLabel("ID")
	if w > p.maxTreeLabelW { p.maxTreeLabelW = w }
	if w > p.maxHydrantLabelW { p.maxHydrantLabelW = w }
	p.positionLabel, w = mkLabel("POSITION")
	if w > p.maxTreeLabelW { p.maxTreeLabelW = w }
	if w > p.maxHydrantLabelW { p.maxHydrantLabelW = w }
	p.doodadHeightLabel, w = mkLabel("HEIGHT")
	if w > p.maxTreeLabelW { p.maxTreeLabelW = w }
	p.spreadLabel, w = mkLabel("SPREAD")
	if w > p.maxTreeLabelW { p.maxTreeLabelW = w }

	// Key hints (dimmer color)
	mkHint := func(text string) (*mesh.Mesh, float32) {
		m, w, err := ui.NewTextMesh(r, text, ps, 140, 140, 140, 200)
		if err != nil {
			return nil, 0
		}
		return m, w
	}
	p.bldgHint, p.bldgHintW = mkHint("UP/DN HEIGHT  V TOGGLE")
	p.signalHint, p.signalHintW = mkHint("L/R DIR  G PLACE")
	p.doodadHint, p.doodadHintW = mkHint("ARROWS MOVE  G PLACE")
	p.placingHint, p.placingHintW = mkHint("CLICK PLACE  ESC CANCEL")
	p.commonHint, p.commonHintW = mkHint("^S SAVE  ^Z UNDO")

	return p
}

// landUseName returns a human-readable land use category name.
func landUseName(code string) string {
	switch code {
	case "1":
		return "1-2 FAMILY"
	case "2":
		return "WALK-UP"
	case "3":
		return "ELEVATOR"
	case "4":
		return "MIXED"
	case "5":
		return "COMMERCIAL"
	case "6":
		return "INDUSTRIAL"
	case "7":
		return "TRANSPORT"
	case "8":
		return "PUBLIC"
	case "9":
		return "OPEN SPACE"
	case "10":
		return "PARKING"
	case "11":
		return "VACANT"
	default:
		return code
	}
}

func (p *InfoPanel) clearValues(r *renderer.Renderer) {
	for _, m := range p.valueMeshes {
		if m != nil {
			m.Destroy(r)
		}
	}
	p.valueMeshes = nil
	p.valueWidths = nil
}

func (p *InfoPanel) addValue(r *renderer.Renderer, text string) {
	m, w, err := ui.NewTextMesh(r, text, p.ps, 255, 255, 255, 255)
	if err != nil {
		p.valueMeshes = append(p.valueMeshes, nil)
		p.valueWidths = append(p.valueWidths, 0)
		return
	}
	p.valueMeshes = append(p.valueMeshes, m)
	p.valueWidths = append(p.valueWidths, w)
}

func (p *InfoPanel) setBuildingValues(r *renderer.Renderer, bbl, address, class, landUse string, year int, floors float32, height float32, hidden bool) {
	p.clearValues(r)
	p.addValue(r, bbl)
	if address != "" {
		p.addValue(r, address)
	} else {
		p.addValue(r, "-")
	}
	p.addValue(r, class)
	p.addValue(r, landUseName(landUse))
	if year > 0 {
		p.addValue(r, fmt.Sprintf("%d", year))
	} else {
		p.addValue(r, "-")
	}
	if floors > 0 {
		p.addValue(r, fmt.Sprintf("%d", int(floors)))
	} else {
		p.addValue(r, "-")
	}
	p.addValue(r, fmt.Sprintf("%.1f M", height))
	if hidden {
		p.addValue(r, "NO")
	} else {
		p.addValue(r, "YES")
	}
}

func (p *InfoPanel) setDoodadValues(r *renderer.Renderer, typeName, id string, x, z float32) {
	p.clearValues(r)
	p.addValue(r, strings.ToUpper(typeName))
	if len(id) > 12 {
		p.addValue(r, id[:12])
	} else {
		p.addValue(r, id)
	}
	p.addValue(r, fmt.Sprintf("%.1f, %.1f", x, z))
}

func (p *InfoPanel) setSignalValues(r *renderer.Renderer, street1, street2 string, angle float32) {
	p.clearValues(r)
	if street1 != "" {
		p.addValue(r, street1)
	} else {
		p.addValue(r, "-")
	}
	if street2 != "" {
		p.addValue(r, street2)
	} else {
		p.addValue(r, "-")
	}
	p.addValue(r, fmt.Sprintf("%.0f DEG", angle*180/3.14159))
}

func (p *InfoPanel) render(r *renderer.Renderer, cmdBuf *sdl.GPUCommandBuffer, swapchainTex *sdl.GPUTexture, screenW, screenH int, sel Selection, dirty bool, placing bool) {
	ortho := mgl32.Ortho2D(0, float32(screenW), float32(screenH), 0)
	pass := r.BeginUIPass(cmdBuf, swapchainTex)

	draw := func(m *mesh.Mesh, transform mgl32.Mat4) {
		if m == nil {
			return
		}
		r.DrawUI(cmdBuf, pass, renderer.DrawCall{
			VertexBuffer: m.VertexBuffer,
			IndexBuffer:  m.IndexBuffer,
			IndexCount:   m.IndexCount,
			Transform:    transform,
		})
	}

	at := func(x, y float32) mgl32.Mat4 {
		return ortho.Mul4(mgl32.Translate3D(x, y, 0))
	}

	sw := float32(screenW)
	charH := p.ps * float32(sign.CharHeight)
	lineH := charH + p.ps*3
	margin := p.ps * 4

	sh := float32(screenH)

	// Crosshair at screen center
	draw(p.crosshair, at(sw/2, sh/2))

	// "ADMIN MODE" indicator in top-right (with dirty "*" if needed)
	modeLabelX := sw - p.modeLabelWidth - margin
	if dirty {
		modeLabelX = sw - p.modeLabelWidth - p.dirtyLabelWidth - margin - p.ps
		draw(p.dirtyLabel, at(sw-p.dirtyLabelWidth-margin, margin))
	}
	draw(p.modeLabel, at(modeLabelX, margin))

	// Info panel on right side (only if something is selected)
	if sel.Type == EntityNone {
		r.EndUIPass(pass)
		return
	}

	pad := p.ps * 3
	valIndent := p.ps * 4

	// Compute panel width from actual content
	var maxLabelW float32
	switch sel.Type {
	case EntityBuilding:
		maxLabelW = p.maxBldgLabelW
	case EntitySignal:
		maxLabelW = p.maxSignalLabelW
	case EntityTree:
		maxLabelW = p.maxTreeLabelW
	case EntityHydrant:
		maxLabelW = p.maxHydrantLabelW
	case EntityDoodad:
		maxLabelW = p.maxHydrantLabelW // generic doodads use same labels as hydrant
	}
	maxContentW := maxLabelW
	for _, vw := range p.valueWidths {
		if valIndent+vw > maxContentW {
			maxContentW = valIndent + vw
		}
	}
	// Account for hint widths
	if sel.Type == EntityBuilding && p.bldgHintW > maxContentW {
		maxContentW = p.bldgHintW
	}
	if sel.Type == EntitySignal && p.signalHintW > maxContentW {
		maxContentW = p.signalHintW
	}
	if (sel.Type == EntityTree || sel.Type == EntityHydrant || sel.Type == EntityDoodad) && p.doodadHintW > maxContentW {
		maxContentW = p.doodadHintW
	}
	if placing && p.placingHintW > maxContentW {
		maxContentW = p.placingHintW
	}
	if p.commonHintW > maxContentW {
		maxContentW = p.commonHintW
	}
	panelW := maxContentW + pad*2
	panelX := sw - panelW - margin
	panelY := margin + lineH*2

	// Count lines for panel height
	var numLines float32
	switch sel.Type {
	case EntityBuilding:
		numLines = 1.5 + 8*2 + 2 // header + 8 label/value pairs + hint lines
	case EntitySignal:
		numLines = 1.5 + 3*2 + 2 // header + 3 label/value pairs + hint lines
	case EntityTree, EntityHydrant, EntityDoodad:
		numLines = 1.5 + float32(len(p.valueMeshes))*2 + 2 // header + label/value pairs + hint lines
	}

	// Draw panel background
	if p.overlay != nil {
		panelH := lineH*numLines + pad*2
		draw(p.overlay, ortho.Mul4(mgl32.Translate3D(panelX, panelY, 0)).Mul4(mgl32.Scale3D(panelW, panelH, 1)))
	}

	contentX := panelX + pad
	valX := contentX + valIndent
	y := panelY + pad

	if sel.Type == EntityBuilding {
		draw(p.buildingLabel, at(contentX, y))
		y += lineH * 1.5

		labels := []*mesh.Mesh{p.bblLabel, p.addressLabel, p.classLabel, p.landUseLabel, p.yearLabel, p.floorsLabel, p.heightLabel, p.visibleLabel}
		for i, label := range labels {
			draw(label, at(contentX, y))
			y += lineH
			if i < len(p.valueMeshes) {
				draw(p.valueMeshes[i], at(valX, y))
			}
			y += lineH
		}

		// Key hints
		y += lineH * 0.5
		draw(p.bldgHint, at(contentX, y))
		y += lineH
		draw(p.commonHint, at(contentX, y))
	} else if sel.Type == EntitySignal {
		draw(p.signalLabel, at(contentX, y))
		y += lineH * 1.5

		labels := []*mesh.Mesh{p.street1Label, p.street2Label, p.dirLabel}
		for i, label := range labels {
			draw(label, at(contentX, y))
			y += lineH
			if i < len(p.valueMeshes) {
				draw(p.valueMeshes[i], at(valX, y))
			}
			y += lineH
		}

		// Key hints
		y += lineH * 0.5
		if placing {
			draw(p.placingHint, at(contentX, y))
		} else {
			draw(p.signalHint, at(contentX, y))
		}
		y += lineH
		draw(p.commonHint, at(contentX, y))
	} else if sel.Type == EntityTree || sel.Type == EntityHydrant || sel.Type == EntityDoodad {
		// First value mesh is the type header (e.g. "TREE", "HYDRANT")
		// Remaining are ID, position, etc.
		labels := []*mesh.Mesh{p.idLabel, p.positionLabel}

		// Draw type name as header (first value)
		if len(p.valueMeshes) > 0 {
			draw(p.valueMeshes[0], at(contentX, y))
		}
		y += lineH * 1.5

		for i, label := range labels {
			draw(label, at(contentX, y))
			y += lineH
			vi := i + 1 // offset by 1 since valueMeshes[0] is the type header
			if vi < len(p.valueMeshes) {
				draw(p.valueMeshes[vi], at(valX, y))
			}
			y += lineH
		}

		y += lineH * 0.5
		if placing {
			draw(p.placingHint, at(contentX, y))
		} else {
			draw(p.doodadHint, at(contentX, y))
		}
		y += lineH
		draw(p.commonHint, at(contentX, y))
	}

	r.EndUIPass(pass)
}

func (p *InfoPanel) destroy(r *renderer.Renderer) {
	p.clearValues(r)

	destroyMesh := func(m *mesh.Mesh) {
		if m != nil {
			m.Destroy(r)
		}
	}

	destroyMesh(p.overlay)
	destroyMesh(p.crosshair)
	destroyMesh(p.modeLabel)
	destroyMesh(p.dirtyLabel)
	destroyMesh(p.buildingLabel)
	destroyMesh(p.bblLabel)
	destroyMesh(p.addressLabel)
	destroyMesh(p.classLabel)
	destroyMesh(p.landUseLabel)
	destroyMesh(p.yearLabel)
	destroyMesh(p.floorsLabel)
	destroyMesh(p.heightLabel)
	destroyMesh(p.visibleLabel)
	destroyMesh(p.signalLabel)
	destroyMesh(p.street1Label)
	destroyMesh(p.street2Label)
	destroyMesh(p.dirLabel)
	destroyMesh(p.treeLabel)
	destroyMesh(p.hydrantLabel)
	destroyMesh(p.idLabel)
	destroyMesh(p.positionLabel)
	destroyMesh(p.doodadHeightLabel)
	destroyMesh(p.spreadLabel)
	destroyMesh(p.bldgHint)
	destroyMesh(p.signalHint)
	destroyMesh(p.doodadHint)
	destroyMesh(p.placingHint)
	destroyMesh(p.commonHint)
}
