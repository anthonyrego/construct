package doodad

import (
	"fmt"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/mapdata"
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

// Instance holds per-entity data. All doodad types use the same struct.
// Fields that don't apply to a type stay at zero/identity values.
type Instance struct {
	ID       string
	X, Z     float32
	ScaleX   float32 // 1 = no scale
	ScaleY   float32
	ScaleZ   float32
	Rotation float32 // Y-axis radians, 0 = no rotation
	TransY   float32 // Y translation offset
}

// TypeConfig defines how a doodad type builds its mesh, converts items
// to instances, and computes frustum cull parameters.
type TypeConfig struct {
	BuildMesh     func(r *renderer.Renderer) (*mesh.Mesh, error)
	BuildInstance func(d mapdata.DoodadItem) (Instance, bool)
	CullCenter    func(inst Instance) (centerY, radius float32)
}

// System holds all instances of one doodad type and the shared mesh.
type System struct {
	TypeName  string
	Instances []Instance
	Mesh      *mesh.Mesh
	config    TypeConfig
}

// New creates a System from map data items using the given type config.
func New(r *renderer.Renderer, typeName string, items []mapdata.DoodadItem, cfg TypeConfig) (*System, error) {
	instances := make([]Instance, 0, len(items))
	for _, d := range items {
		inst, ok := cfg.BuildInstance(d)
		if !ok {
			continue
		}
		// Default zero scales to 1
		if inst.ScaleX == 0 {
			inst.ScaleX = 1
		}
		if inst.ScaleY == 0 {
			inst.ScaleY = 1
		}
		if inst.ScaleZ == 0 {
			inst.ScaleZ = 1
		}
		instances = append(instances, inst)
	}

	m, err := cfg.BuildMesh(r)
	if err != nil {
		return nil, fmt.Errorf("doodad %s mesh: %w", typeName, err)
	}

	return &System{
		TypeName:  typeName,
		Instances: instances,
		Mesh:      m,
		config:    cfg,
	}, nil
}

// Render draws all instances with frustum and distance culling.
func (s *System) Render(rend *renderer.Renderer, frame renderer.RenderFrame) {
	if s == nil {
		return
	}
	m := s.Mesh
	for i := range s.Instances {
		inst := &s.Instances[i]

		// Frustum cull
		cy, cr := s.config.CullCenter(*inst)
		if !frame.Frustum.SphereVisible(mgl32.Vec3{inst.X, cy, inst.Z}, cr) {
			continue
		}

		// Distance cull
		dx := inst.X - frame.CamPos.X()
		dz := inst.Z - frame.CamPos.Z()
		if dx*dx+dz*dz > frame.CullDistSq {
			continue
		}

		// Build model matrix
		model := mgl32.Translate3D(inst.X, inst.TransY, inst.Z)
		if inst.Rotation != 0 {
			model = model.Mul4(mgl32.HomogRotate3DY(inst.Rotation))
		}
		if inst.ScaleX != 1 || inst.ScaleY != 1 || inst.ScaleZ != 1 {
			model = model.Mul4(mgl32.Scale3D(inst.ScaleX, inst.ScaleY, inst.ScaleZ))
		}

		rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
			VertexBuffer: m.VertexBuffer,
			IndexBuffer:  m.IndexBuffer,
			IndexCount:   m.IndexCount,
			MVP:          frame.ViewProj.Mul4(model),
			Model:        model,
		})
	}
}

// CullSphere returns the frustum-cull sphere center Y and radius for the instance at idx.
func (s *System) CullSphere(idx int) (centerY, radius float32) {
	return s.config.CullCenter(s.Instances[idx])
}

// Destroy releases the shared mesh GPU buffers.
func (s *System) Destroy(r *renderer.Renderer) {
	if s != nil && s.Mesh != nil {
		s.Mesh.Destroy(r)
	}
}
