package ui

import (
	"fmt"

	"github.com/Zyko0/go-sdl3/sdl"
	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/input"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/sign"
)

type MenuState int

const (
	Hidden MenuState = iota
	Main
	Settings
)

type Action int

const (
	ActionNone Action = iota
	ActionQuit
	ActionToggleFullscreen
	ActionSetResolution
)

type Resolution struct {
	W, H int
}

var Resolutions = []Resolution{
	{1280, 720},
	{1600, 900},
	{1920, 1080},
	{2560, 1440},
}

// textEntry holds white (selected) and gray (unselected) variants of a text mesh.
type textEntry struct {
	white *mesh.Mesh
	gray  *mesh.Mesh
	width float32
}

func (t *textEntry) meshFor(selected bool) *mesh.Mesh {
	if selected {
		return t.white
	}
	return t.gray
}

func (t *textEntry) destroy(r *renderer.Renderer) {
	if t.white != nil {
		t.white.Destroy(r)
	}
	if t.gray != nil {
		t.gray.Destroy(r)
	}
}

type PauseMenu struct {
	state    MenuState
	selIndex int
	ps       float32 // screen pixels per font pixel

	overlay *mesh.Mesh

	title textEntry // "PAUSED" (white only)
	arrow textEntry // ">" (white only)

	// Main menu: resume(0), settings(1), quit(2)
	mainItems [3]textEntry

	// Settings
	settingsTitle textEntry   // "SETTINGS" (white only)
	fsLabel       textEntry   // "FULLSCREEN"
	fsOn          textEntry   // "ON"
	fsOff         textEntry   // "OFF"
	resLabel      textEntry   // "RESOLUTION"
	resOpts       []textEntry // "1280X720", etc.
	back          textEntry   // "BACK"

	Fullscreen  bool
	ResIndex    int
	ResolutionW int
	ResolutionH int
}

func newEntry(r *renderer.Renderer, text string, ps float32) (textEntry, error) {
	w, width, err := NewTextMesh(r, text, ps, 255, 255, 255, 255)
	if err != nil {
		return textEntry{}, err
	}
	g, _, err := NewTextMesh(r, text, ps, 120, 120, 120, 255)
	if err != nil {
		w.Destroy(r)
		return textEntry{}, err
	}
	return textEntry{white: w, gray: g, width: width}, nil
}

func newWhiteOnly(r *renderer.Renderer, text string, ps float32) (textEntry, error) {
	w, width, err := NewTextMesh(r, text, ps, 255, 255, 255, 255)
	if err != nil {
		return textEntry{}, err
	}
	return textEntry{white: w, width: width}, nil
}

func NewPauseMenu(r *renderer.Renderer, pixelScale int) *PauseMenu {
	ps := float32(pixelScale)
	p := &PauseMenu{ps: ps}

	// Overlay: unit quad with semi-transparent black
	overlayVerts := []renderer.Vertex{
		{X: 0, Y: 0, Z: 0, R: 0, G: 0, B: 0, A: 160},
		{X: 1, Y: 0, Z: 0, R: 0, G: 0, B: 0, A: 160},
		{X: 1, Y: 1, Z: 0, R: 0, G: 0, B: 0, A: 160},
		{X: 0, Y: 1, Z: 0, R: 0, G: 0, B: 0, A: 160},
	}
	overlayIdx := []uint16{0, 1, 2, 0, 2, 3}
	vb, err := r.CreateVertexBuffer(overlayVerts)
	if err != nil {
		fmt.Println("Warning: UI overlay vertex buffer:", err)
		return p
	}
	ib, err := r.CreateIndexBuffer(overlayIdx)
	if err != nil {
		r.ReleaseBuffer(vb)
		fmt.Println("Warning: UI overlay index buffer:", err)
		return p
	}
	p.overlay = &mesh.Mesh{VertexBuffer: vb, IndexBuffer: ib, IndexCount: 6}

	// Build text meshes (errors are non-fatal, menu just won't render that item)
	p.title, _ = newWhiteOnly(r, "PAUSED", ps)
	p.arrow, _ = newWhiteOnly(r, ">", ps)

	p.mainItems[0], _ = newEntry(r, "RESUME", ps)
	p.mainItems[1], _ = newEntry(r, "SETTINGS", ps)
	p.mainItems[2], _ = newEntry(r, "QUIT", ps)

	p.settingsTitle, _ = newWhiteOnly(r, "SETTINGS", ps)
	p.fsLabel, _ = newEntry(r, "FULLSCREEN", ps)
	p.fsOn, _ = newEntry(r, "ON", ps)
	p.fsOff, _ = newEntry(r, "OFF", ps)
	p.resLabel, _ = newEntry(r, "RESOLUTION", ps)
	p.back, _ = newEntry(r, "BACK", ps)

	for _, res := range Resolutions {
		label := fmt.Sprintf("%dX%d", res.W, res.H)
		entry, _ := newEntry(r, label, ps)
		p.resOpts = append(p.resOpts, entry)
	}

	return p
}

func (p *PauseMenu) IsActive() bool {
	return p.state != Hidden
}

func (p *PauseMenu) HandleInput(inp *input.Input) Action {
	if inp.IsKeyPressed(sdl.K_TAB) {
		if p.state == Hidden {
			p.state = Main
			p.selIndex = 0
			return ActionNone
		}
		p.state = Hidden
		return ActionNone
	}

	if p.state == Hidden {
		return ActionNone
	}

	if p.state == Main {
		if inp.IsKeyPressed(sdl.K_ESCAPE) {
			p.state = Hidden
			return ActionNone
		}
		if inp.IsKeyPressed(sdl.K_UP) {
			p.selIndex--
			if p.selIndex < 0 {
				p.selIndex = 2
			}
		}
		if inp.IsKeyPressed(sdl.K_DOWN) {
			p.selIndex++
			if p.selIndex > 2 {
				p.selIndex = 0
			}
		}
		if inp.IsKeyPressed(sdl.K_RETURN) {
			switch p.selIndex {
			case 0: // Resume
				p.state = Hidden
			case 1: // Settings
				p.state = Settings
				p.selIndex = 2 // Start on BACK
			case 2: // Quit
				return ActionQuit
			}
		}
		return ActionNone
	}

	// Settings
	if inp.IsKeyPressed(sdl.K_ESCAPE) {
		p.state = Main
		p.selIndex = 1
		return ActionNone
	}
	if inp.IsKeyPressed(sdl.K_UP) {
		p.selIndex--
		if p.selIndex < 0 {
			p.selIndex = 2
		}
	}
	if inp.IsKeyPressed(sdl.K_DOWN) {
		p.selIndex++
		if p.selIndex > 2 {
			p.selIndex = 0
		}
	}
	if inp.IsKeyPressed(sdl.K_RETURN) {
		switch p.selIndex {
		case 0: // Fullscreen toggle
			p.Fullscreen = !p.Fullscreen
			return ActionToggleFullscreen
		case 2: // Back
			p.state = Main
			p.selIndex = 1
		}
	}
	// Left/Right for resolution cycling
	if p.selIndex == 1 {
		if inp.IsKeyPressed(sdl.K_LEFT) {
			p.ResIndex--
			if p.ResIndex < 0 {
				p.ResIndex = len(Resolutions) - 1
			}
			p.ResolutionW = Resolutions[p.ResIndex].W
			p.ResolutionH = Resolutions[p.ResIndex].H
			return ActionSetResolution
		}
		if inp.IsKeyPressed(sdl.K_RIGHT) {
			p.ResIndex++
			if p.ResIndex >= len(Resolutions) {
				p.ResIndex = 0
			}
			p.ResolutionW = Resolutions[p.ResIndex].W
			p.ResolutionH = Resolutions[p.ResIndex].H
			return ActionSetResolution
		}
	}

	return ActionNone
}

func (p *PauseMenu) Render(r *renderer.Renderer, cmdBuf *sdl.GPUCommandBuffer, swapchainTex *sdl.GPUTexture, screenW, screenH int) {
	if p.state == Hidden || p.overlay == nil {
		return
	}

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

	// Dark overlay
	draw(p.overlay, ortho.Mul4(mgl32.Scale3D(float32(screenW), float32(screenH), 1)))

	sw := float32(screenW)
	sh := float32(screenH)
	charH := p.ps * float32(sign.CharHeight)
	lineH := charH + p.ps*3

	at := func(x, y float32) mgl32.Mat4 {
		return ortho.Mul4(mgl32.Translate3D(x, y, 0))
	}

	drawArrow := func(x, y float32) {
		draw(p.arrow.white, at(x-p.arrow.width-p.ps*2, y))
	}

	if p.state == Main {
		// Title
		titleY := sh*0.30 - charH/2
		draw(p.title.white, at((sw-p.title.width)/2, titleY))

		// Menu items
		startY := sh * 0.45
		for i := range p.mainItems {
			y := startY + float32(i)*lineH
			sel := i == p.selIndex
			x := (sw - p.mainItems[i].width) / 2
			draw(p.mainItems[i].meshFor(sel), at(x, y))
			if sel {
				drawArrow(x, y)
			}
		}
	} else {
		// Settings title
		titleY := sh*0.30 - charH/2
		draw(p.settingsTitle.white, at((sw-p.settingsTitle.width)/2, titleY))

		startY := sh * 0.45
		gap := p.ps * 3

		// Item 0: Fullscreen
		{
			i := 0
			y := startY + float32(i)*lineH
			sel := p.selIndex == i
			val := &p.fsOff
			if p.Fullscreen {
				val = &p.fsOn
			}
			totalW := p.fsLabel.width + gap + val.width
			lx := (sw - totalW) / 2
			draw(p.fsLabel.meshFor(sel), at(lx, y))
			draw(val.meshFor(sel), at(lx+p.fsLabel.width+gap, y))
			if sel {
				drawArrow(lx, y)
			}
		}

		// Item 1: Resolution
		{
			i := 1
			y := startY + float32(i)*lineH
			sel := p.selIndex == i
			resIdx := p.ResIndex
			if resIdx < 0 || resIdx >= len(p.resOpts) {
				resIdx = 0
			}
			val := &p.resOpts[resIdx]
			totalW := p.resLabel.width + gap + val.width
			lx := (sw - totalW) / 2
			draw(p.resLabel.meshFor(sel), at(lx, y))
			draw(val.meshFor(sel), at(lx+p.resLabel.width+gap, y))
			if sel {
				drawArrow(lx, y)
			}
		}

		// Item 2: Back
		{
			i := 2
			y := startY + float32(i)*lineH
			sel := p.selIndex == i
			x := (sw - p.back.width) / 2
			draw(p.back.meshFor(sel), at(x, y))
			if sel {
				drawArrow(x, y)
			}
		}
	}

	r.EndUIPass(pass)
}

func (p *PauseMenu) Destroy(r *renderer.Renderer) {
	if p.overlay != nil {
		p.overlay.Destroy(r)
	}
	p.title.destroy(r)
	p.arrow.destroy(r)
	for i := range p.mainItems {
		p.mainItems[i].destroy(r)
	}
	p.settingsTitle.destroy(r)
	p.fsLabel.destroy(r)
	p.fsOn.destroy(r)
	p.fsOff.destroy(r)
	p.resLabel.destroy(r)
	p.back.destroy(r)
	for i := range p.resOpts {
		p.resOpts[i].destroy(r)
	}
}
