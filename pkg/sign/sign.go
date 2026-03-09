package sign

import (
	"strings"

	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

const (
	charWidth  = 3
	charHeight = 5
	PixelSize  = float32(0.04) // meters per font pixel
	spacing    = 1             // pixels between characters
	padding    = 1             // pixels of padding around text

	SignHeight = float32(charHeight+2*padding) * PixelSize
)

// 3x5 pixel font. Each row: bit2=left, bit1=mid, bit0=right. Rows top to bottom.
var glyphs = map[byte][5]uint8{
	'A': {0b010, 0b101, 0b111, 0b101, 0b101},
	'B': {0b110, 0b101, 0b110, 0b101, 0b110},
	'C': {0b011, 0b100, 0b100, 0b100, 0b011},
	'D': {0b110, 0b101, 0b101, 0b101, 0b110},
	'E': {0b111, 0b100, 0b110, 0b100, 0b111},
	'F': {0b111, 0b100, 0b110, 0b100, 0b100},
	'G': {0b011, 0b100, 0b101, 0b101, 0b011},
	'H': {0b101, 0b101, 0b111, 0b101, 0b101},
	'I': {0b111, 0b010, 0b010, 0b010, 0b111},
	'J': {0b001, 0b001, 0b001, 0b101, 0b010},
	'K': {0b101, 0b101, 0b110, 0b101, 0b101},
	'L': {0b100, 0b100, 0b100, 0b100, 0b111},
	'M': {0b101, 0b111, 0b111, 0b101, 0b101},
	'N': {0b101, 0b111, 0b101, 0b101, 0b101},
	'O': {0b010, 0b101, 0b101, 0b101, 0b010},
	'P': {0b110, 0b101, 0b110, 0b100, 0b100},
	'Q': {0b010, 0b101, 0b101, 0b110, 0b011},
	'R': {0b110, 0b101, 0b110, 0b101, 0b101},
	'S': {0b011, 0b100, 0b010, 0b001, 0b110},
	'T': {0b111, 0b010, 0b010, 0b010, 0b010},
	'U': {0b101, 0b101, 0b101, 0b101, 0b010},
	'V': {0b101, 0b101, 0b101, 0b010, 0b010},
	'W': {0b101, 0b101, 0b111, 0b111, 0b101},
	'X': {0b101, 0b101, 0b010, 0b101, 0b101},
	'Y': {0b101, 0b101, 0b010, 0b010, 0b010},
	'Z': {0b111, 0b001, 0b010, 0b100, 0b111},
	'0': {0b111, 0b101, 0b101, 0b101, 0b111},
	'1': {0b010, 0b110, 0b010, 0b010, 0b111},
	'2': {0b110, 0b001, 0b010, 0b100, 0b111},
	'3': {0b110, 0b001, 0b010, 0b001, 0b110},
	'4': {0b101, 0b101, 0b111, 0b001, 0b001},
	'5': {0b111, 0b100, 0b110, 0b001, 0b110},
	'6': {0b011, 0b100, 0b111, 0b101, 0b111},
	'7': {0b111, 0b001, 0b010, 0b010, 0b010},
	'8': {0b111, 0b101, 0b010, 0b101, 0b111},
	'9': {0b111, 0b101, 0b111, 0b001, 0b110},
	' ': {0b000, 0b000, 0b000, 0b000, 0b000},
	'-': {0b000, 0b000, 0b111, 0b000, 0b000},
	'.': {0b000, 0b000, 0b000, 0b000, 0b010},
}

// NewMesh creates a flat sign mesh with pixel-font text.
// The sign is centered at origin along X, bottom edge at Y=0, facing +Z.
// Returns the mesh and the sign's total width in meters.
func NewMesh(r *renderer.Renderer, text string) (*mesh.Mesh, float32, error) {
	text = strings.ToUpper(text)

	nChars := len(text)
	if nChars == 0 {
		nChars = 1
		text = " "
	}

	textWidthPx := nChars*charWidth + (nChars-1)*spacing
	totalWidthPx := textWidthPx + 2*padding
	totalHeightPx := charHeight + 2*padding

	totalWidth := float32(totalWidthPx) * PixelSize
	totalHeight := float32(totalHeightPx) * PixelSize

	var vertices []renderer.LitVertex
	var indices []uint16

	// Background quad (dark green, like NYC street signs)
	addQuad(&vertices, &indices,
		-totalWidth/2, 0, totalWidth/2, totalHeight,
		-0.003, 0, 0, 1,
		0, 80, 40)

	// Back face of background
	addQuad(&vertices, &indices,
		totalWidth/2, 0, -totalWidth/2, totalHeight,
		-0.003, 0, 0, -1,
		0, 60, 30)

	// Text pixels (white)
	startX := -totalWidth/2 + float32(padding)*PixelSize
	startY := float32(padding) * PixelSize

	for ci := 0; ci < len(text); ci++ {
		ch := text[ci]
		glyph, ok := glyphs[ch]
		if !ok {
			continue
		}

		charX := startX + float32(ci*(charWidth+spacing))*PixelSize

		for row := 0; row < charHeight; row++ {
			bits := glyph[row]
			pixY := startY + float32(charHeight-1-row)*PixelSize

			for col := 0; col < charWidth; col++ {
				if bits&(1<<(charWidth-1-col)) != 0 {
					pixX := charX + float32(col)*PixelSize
					addQuad(&vertices, &indices,
						pixX, pixY, pixX+PixelSize, pixY+PixelSize,
						0, 0, 0, 1,
						255, 255, 255)
				}
			}
		}
	}

	vertexBuffer, err := r.CreateLitVertexBuffer(vertices)
	if err != nil {
		return nil, 0, err
	}

	indexBuffer, err := r.CreateIndexBuffer(indices)
	if err != nil {
		r.ReleaseBuffer(vertexBuffer)
		return nil, 0, err
	}

	return &mesh.Mesh{
		VertexBuffer: vertexBuffer,
		IndexBuffer:  indexBuffer,
		IndexCount:   uint32(len(indices)),
	}, totalWidth, nil
}

func addQuad(verts *[]renderer.LitVertex, idxs *[]uint16,
	x0, y0, x1, y1, z, nx, ny, nz float32,
	r, g, b uint8) {

	base := uint16(len(*verts))
	*verts = append(*verts,
		renderer.LitVertex{X: x0, Y: y0, Z: z, NX: nx, NY: ny, NZ: nz, R: r, G: g, B: b, A: 255},
		renderer.LitVertex{X: x1, Y: y0, Z: z, NX: nx, NY: ny, NZ: nz, R: r, G: g, B: b, A: 255},
		renderer.LitVertex{X: x1, Y: y1, Z: z, NX: nx, NY: ny, NZ: nz, R: r, G: g, B: b, A: 255},
		renderer.LitVertex{X: x0, Y: y1, Z: z, NX: nx, NY: ny, NZ: nz, R: r, G: g, B: b, A: 255},
	)
	*idxs = append(*idxs,
		base, base+1, base+2,
		base, base+2, base+3,
	)
}
