package traffic

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
	"github.com/anthonyrego/construct/pkg/sign"
)

// createMeshes allocates all shared GPU meshes for rendering signals.
func (s *System) createMeshes(rend *renderer.Renderer) error {
	var err error

	newCube := func(r, g, b uint8) (*mesh.Mesh, error) {
		return mesh.NewLitCube(rend, r, g, b)
	}

	if s.poleMesh, err = newCube(30, 30, 30); err != nil {
		return err
	}
	if s.housingMesh, err = newCube(20, 20, 20); err != nil {
		return err
	}
	if s.greenOn, err = newCube(0, 255, 76); err != nil {
		return err
	}
	if s.yellowOn, err = newCube(255, 230, 0); err != nil {
		return err
	}
	if s.redOn, err = newCube(255, 25, 0); err != nil {
		return err
	}
	if s.greenOff, err = newCube(0, 30, 9); err != nil {
		return err
	}
	if s.yellowOff, err = newCube(30, 27, 0); err != nil {
		return err
	}
	if s.redOff, err = newCube(30, 3, 0); err != nil {
		return err
	}

	// Create sign meshes for unique street names
	s.signMeshes = make(map[string]*mesh.Mesh)
	for _, sig := range s.Signals {
		for _, name := range []string{sig.Street1, sig.Street2} {
			if name == "" {
				continue
			}
			if _, exists := s.signMeshes[name]; exists {
				continue
			}
			m, _, err := sign.NewMesh(rend, name)
			if err != nil {
				continue
			}
			s.signMeshes[name] = m
		}
	}

	return nil
}

// Destroy releases all owned GPU meshes.
func (s *System) Destroy(rend *renderer.Renderer) {
	for _, m := range []*mesh.Mesh{
		s.poleMesh, s.housingMesh,
		s.greenOn, s.yellowOn, s.redOn,
		s.greenOff, s.yellowOff, s.redOff,
	} {
		if m != nil {
			m.Destroy(rend)
		}
	}
	for _, m := range s.signMeshes {
		m.Destroy(rend)
	}
	s.signMeshes = nil
}

// Render draws all traffic signals: poles, housings, light cubes, and street signs.
// highlightIdx is the signal index to highlight for admin mode (-1 for none).
func (s *System) Render(rend *renderer.Renderer, frame renderer.RenderFrame, highlightIdx int) {
	if s == nil {
		return
	}

	for sigIdx, sig := range s.Signals {
		x, z := sig.Position.X, sig.Position.Z

		// Frustum cull entire intersection (generous 10m radius)
		if !frame.Frustum.SphereVisible(mgl32.Vec3{x, PoleHeight / 2, z}, 10) {
			continue
		}

		var sigHighlight float32
		if sigIdx == highlightIdx {
			sigHighlight = 1.0
		}

		// Pole (one per intersection)
		poleModel := mgl32.Translate3D(x, PoleHeight/2, z)
		poleModel = poleModel.Mul4(mgl32.Scale3D(PoleWidth, PoleHeight, PoleWidth))
		rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
			VertexBuffer: s.poleMesh.VertexBuffer,
			IndexBuffer:  s.poleMesh.IndexBuffer,
			IndexCount:   s.poleMesh.IndexCount,
			MVP:          frame.ViewProj.Mul4(poleModel),
			Model:        poleModel,
			Highlight:    sigHighlight,
		})

		// Two directional signal heads per intersection
		heads := sig.Heads()
		for _, h := range heads {
			// Forward direction for this head
			sinA := float32(math.Sin(float64(h.Angle)))
			cosA := float32(math.Cos(float64(h.Angle)))

			// Housing (dark box behind lights, blocks side/rear view)
			hModel := mgl32.Translate3D(h.X, HousingY, h.Z)
			hModel = hModel.Mul4(mgl32.HomogRotate3DY(h.Angle))
			hModel = hModel.Mul4(mgl32.Scale3D(HousingWidth, HousingHeight, HousingDepth))
			rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
				VertexBuffer: s.housingMesh.VertexBuffer,
				IndexBuffer:  s.housingMesh.IndexBuffer,
				IndexCount:   s.housingMesh.IndexCount,
				MVP:          frame.ViewProj.Mul4(hModel),
				Model:        hModel,
				Highlight:    sigHighlight,
			})

			// Light cubes offset forward from housing
			fwd := LightForward
			lx := h.X + sinA*fwd
			lz := h.Z + cosA*fwd
			s.drawLightBox(rend, frame, h.Phase == Red, s.redOn, s.redOff, lx, RedY, lz, sigHighlight)
			s.drawLightBox(rend, frame, h.Phase == Yellow, s.yellowOn, s.yellowOff, lx, YellowY, lz, sigHighlight)
			s.drawLightBox(rend, frame, h.Phase == Green, s.greenOn, s.greenOff, lx, GreenY, lz, sigHighlight)
		}

		// Street name signs (stacked on the pole, not on the heads)
		if sig.Street1 != "" {
			s.drawSign(rend, frame, sig.Street1, x, SignY1, z, sig.DirAngle+math.Pi/2)
		}
		if sig.Street2 != "" {
			s.drawSign(rend, frame, sig.Street2, x, SignY2, z, sig.DirAngle+math.Pi)
		}
	}
}

func (s *System) drawLightBox(rend *renderer.Renderer, frame renderer.RenderFrame,
	on bool, onMesh, offMesh *mesh.Mesh, x, y, z, highlight float32) {
	m := offMesh
	if on {
		m = onMesh
	}
	model := mgl32.Translate3D(x, y, z)
	model = model.Mul4(mgl32.Scale3D(LightBoxSize, LightBoxSize, LightBoxSize))
	rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
		VertexBuffer: m.VertexBuffer,
		IndexBuffer:  m.IndexBuffer,
		IndexCount:   m.IndexCount,
		MVP:          frame.ViewProj.Mul4(model),
		Model:        model,
		Highlight:    highlight,
	})
}

func (s *System) drawSign(rend *renderer.Renderer, frame renderer.RenderFrame,
	name string, x, y, z, angle float32) {
	sm, ok := s.signMeshes[name]
	if !ok {
		return
	}
	model := mgl32.Translate3D(x, y, z).Mul4(mgl32.HomogRotate3DY(angle))
	rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
		VertexBuffer: sm.VertexBuffer,
		IndexBuffer:  sm.IndexBuffer,
		IndexCount:   sm.IndexCount,
		MVP:          frame.ViewProj.Mul4(model),
		Model:        model,
	})
}
