package scene

import (
	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/go-gl/mathgl/mgl32"
)

// Brownstone returns the objects for a single brownstone building.
// pos is the ground-level center of the building.
// facingX is +1 for left-side buildings (stoop faces +X) or -1 for right-side (stoop faces -X).
func Brownstone(bodyMesh, stoopMesh, corniceMesh *mesh.Mesh, pos mgl32.Vec3, height float32, facingX float32) []Object {
	return []Object{
		{
			Mesh:     bodyMesh,
			Position: mgl32.Vec3{pos.X(), height / 2, pos.Z()},
			Scale:    mgl32.Vec3{3.0, height, 3.0},
		},
		{
			Mesh:     stoopMesh,
			Position: mgl32.Vec3{pos.X() + facingX*1.8, 0.3, pos.Z()},
			Scale:    mgl32.Vec3{0.6, 0.6, 1.0},
		},
		{
			Mesh:     corniceMesh,
			Position: mgl32.Vec3{pos.X(), height + 0.1, pos.Z()},
			Scale:    mgl32.Vec3{3.2, 0.2, 3.2},
		},
	}
}

// StreetLight returns the objects and point light for a single street lamp.
// facingX is the direction the lantern extends toward the street.
func StreetLight(poleMesh, lanternMesh *mesh.Mesh, pos mgl32.Vec3, facingX float32, color mgl32.Vec3, intensity float32) ([]Object, PointLight) {
	objects := []Object{
		{
			Mesh:     poleMesh,
			Position: mgl32.Vec3{pos.X(), 1.5, pos.Z()},
			Scale:    mgl32.Vec3{0.15, 3.0, 0.15},
		},
		{
			Mesh:     lanternMesh,
			Position: mgl32.Vec3{pos.X() + facingX*0.3, 3.1, pos.Z()},
			Scale:    mgl32.Vec3{0.3, 0.3, 0.3},
		},
	}
	light := PointLight{
		Position:  mgl32.Vec3{pos.X() + facingX*0.3, 3.3, pos.Z()},
		Color:     color,
		Intensity: intensity,
	}
	return objects, light
}

// GroundStrip returns a single flat cube acting as a ground strip.
func GroundStrip(groundMesh *mesh.Mesh, centerX, width, length float32) Object {
	return Object{
		Mesh:     groundMesh,
		Position: mgl32.Vec3{centerX, -0.01, -length / 2},
		Scale:    mgl32.Vec3{width, 0.02, length},
	}
}
