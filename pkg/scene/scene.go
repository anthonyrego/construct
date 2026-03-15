package scene

import (
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/go-gl/mathgl/mgl32"
)

type Object struct {
	Mesh     *mesh.Mesh
	Position mgl32.Vec3
	Scale    mgl32.Vec3
	Radius   float32 // Bounding sphere radius for frustum culling
}

type PointLight struct {
	Position  mgl32.Vec3
	Color     mgl32.Vec3
	Intensity float32
}

type Scene struct {
	Objects []Object
	Lights  []PointLight
}

func (s *Scene) Add(objects ...Object) {
	s.Objects = append(s.Objects, objects...)
}

func (s *Scene) AddLight(lights ...PointLight) {
	s.Lights = append(s.Lights, lights...)
}
