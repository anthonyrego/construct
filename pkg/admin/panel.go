package admin

import (
	"fmt"

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

	signalLabel  *mesh.Mesh
	street1Label *mesh.Mesh
	street2Label *mesh.Mesh
	dirLabel     *mesh.Mesh

	// Dynamic value meshes (recreated on selection change)
	valueMeshes []*mesh.Mesh
	valueWidths []float32

	modeLabelWidth  float32
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

	p.signalLabel, w = mkLabel("SIGNAL")
	p.maxSignalLabelW = w
	p.street1Label, w = mkLabel("STREET 1")
	if w > p.maxSignalLabelW { p.maxSignalLabelW = w }
	p.street2Label, w = mkLabel("STREET 2")
	if w > p.maxSignalLabelW { p.maxSignalLabelW = w }
	p.dirLabel, w = mkLabel("DIRECTION")
	if w > p.maxSignalLabelW { p.maxSignalLabelW = w }

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

func (p *InfoPanel) setBuildingValues(r *renderer.Renderer, bbl, address, class, landUse string, year int, floors float32) {
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

func (p *InfoPanel) render(r *renderer.Renderer, cmdBuf *sdl.GPUCommandBuffer, swapchainTex *sdl.GPUTexture, screenW, screenH int, sel Selection) {
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

	// "ADMIN MODE" indicator in top-right
	draw(p.modeLabel, at(sw-p.modeLabelWidth-margin, margin))

	// Info panel on right side (only if something is selected)
	if sel.Type == EntityNone {
		r.EndUIPass(pass)
		return
	}

	pad := p.ps * 3
	valIndent := p.ps * 4

	// Compute panel width from actual content
	var maxLabelW float32
	if sel.Type == EntityBuilding {
		maxLabelW = p.maxBldgLabelW
	} else {
		maxLabelW = p.maxSignalLabelW
	}
	maxContentW := maxLabelW
	for _, vw := range p.valueWidths {
		if valIndent+vw > maxContentW {
			maxContentW = valIndent + vw
		}
	}
	panelW := maxContentW + pad*2
	panelX := sw - panelW - margin
	panelY := margin + lineH*2

	// Count lines for panel height
	var numLines float32
	if sel.Type == EntityBuilding {
		numLines = 1.5 + 6*2 // header + 6 label/value pairs
	} else {
		numLines = 1.5 + 3*2 // header + 3 label/value pairs
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

		labels := []*mesh.Mesh{p.bblLabel, p.addressLabel, p.classLabel, p.landUseLabel, p.yearLabel, p.floorsLabel}
		for i, label := range labels {
			draw(label, at(contentX, y))
			y += lineH
			if i < len(p.valueMeshes) {
				draw(p.valueMeshes[i], at(valX, y))
			}
			y += lineH
		}
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
	destroyMesh(p.buildingLabel)
	destroyMesh(p.bblLabel)
	destroyMesh(p.addressLabel)
	destroyMesh(p.classLabel)
	destroyMesh(p.landUseLabel)
	destroyMesh(p.yearLabel)
	destroyMesh(p.floorsLabel)
	destroyMesh(p.signalLabel)
	destroyMesh(p.street1Label)
	destroyMesh(p.street2Label)
	destroyMesh(p.dirLabel)
}
