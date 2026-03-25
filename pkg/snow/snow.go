package snow

import (
	"math"
	"math/rand"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/anthonyrego/construct/pkg/mesh"
	"github.com/anthonyrego/construct/pkg/renderer"
)

type Particle struct {
	Pos    mgl32.Vec3
	VelY   float32 // fall speed (negative)
	Phase  float32 // unique phase for wind sine
	Size   float32 // base width
	Aspect float32 // height/width ratio (0.4–1.0)
}

type System struct {
	Particles      []Particle
	WindTime       float32
	WindStrength   float32
	FallSpeed      float32
	ParticleSize   float32
	Radius         float32 // half-size of the snow area around center
	HeightRange    float32 // vertical range above/below center
	CenterX        float32
	CenterY        float32
	CenterZ        float32
}

func (s *System) SetFallSpeed(speed float32) {
	if speed == s.FallSpeed {
		return
	}
	old := s.FallSpeed
	s.FallSpeed = speed
	for i := range s.Particles {
		s.Particles[i].VelY = s.Particles[i].VelY / old * speed
	}
}

func (s *System) SetParticleSize(size float32) {
	if size == s.ParticleSize {
		return
	}
	old := s.ParticleSize
	s.ParticleSize = size
	for i := range s.Particles {
		s.Particles[i].Size = s.Particles[i].Size / old * size
	}
}

func (s *System) SetCount(count int) {
	if count == len(s.Particles) {
		return
	}
	old := len(s.Particles)
	if count < old {
		s.Particles = s.Particles[:count]
		return
	}
	s.Particles = append(s.Particles, make([]Particle, count-old)...)
	for i := old; i < count; i++ {
		s.spawn(&s.Particles[i], true)
	}
}

func New(count int) *System {
	s := &System{
		Particles:    make([]Particle, count),
		WindStrength: 0.4,
		FallSpeed:    1.5,
		ParticleSize: 0.06,
		Radius:      30,
		HeightRange: 15,
	}

	for i := range s.Particles {
		s.spawn(&s.Particles[i], true)
	}

	return s
}

func (s *System) spawn(p *Particle, randomY bool) {
	p.Pos = mgl32.Vec3{
		s.CenterX - s.Radius + rand.Float32()*2*s.Radius,
		0,
		s.CenterZ - s.Radius + rand.Float32()*2*s.Radius,
	}
	minY := s.CenterY - s.HeightRange
	maxY := s.CenterY + s.HeightRange
	if randomY {
		p.Pos[1] = minY + rand.Float32()*(maxY-minY)
	} else {
		p.Pos[1] = maxY + rand.Float32()*3
	}
	// Vary fall speed per particle
	p.VelY = -(s.FallSpeed * (0.6 + rand.Float32()*0.8))
	p.Phase = rand.Float32() * math.Pi * 2
	// Vary size and aspect per particle
	p.Size = s.ParticleSize * (0.5 + rand.Float32())
	p.Aspect = 0.75 + rand.Float32()*0.25
}

func (s *System) SetCenter(x, y, z float32) {
	s.CenterX = x
	s.CenterY = y
	s.CenterZ = z
}

func (s *System) Update(dt float32) {
	s.WindTime += dt

	ws := s.WindStrength
	t := s.WindTime

	minX := s.CenterX - s.Radius
	maxX := s.CenterX + s.Radius
	minZ := s.CenterZ - s.Radius
	maxZ := s.CenterZ + s.Radius

	for i := range s.Particles {
		p := &s.Particles[i]

		// Layered sine wind — calm, organic drift
		// Broad slow sway + faster ripple, unique per particle
		windX := ws * (float32(math.Sin(float64(t*0.7+p.Phase))) +
			0.5*float32(math.Sin(float64(t*1.3+p.Phase*1.7))))
		windZ := ws * 0.3 * float32(math.Sin(float64(t*0.5+p.Phase*2.1)))

		p.Pos[0] += windX * dt
		p.Pos[1] += p.VelY * dt
		p.Pos[2] += windZ * dt

		// Respawn when out of bounds
		minY := s.CenterY - s.HeightRange
		if p.Pos[1] < minY {
			s.spawn(p, false) // fell below — respawn at top
		} else if p.Pos[0] < minX-3 || p.Pos[0] > maxX+3 ||
			p.Pos[2] < minZ-3 || p.Pos[2] > maxZ+3 {
			s.spawn(p, true) // drifted out horizontally — respawn at random height
		}
	}
}

// Render draws all snow particles as billboarded quads facing the camera.
func (s *System) Render(rend *renderer.Renderer, frame renderer.RenderFrame, snowMesh *mesh.Mesh) {
	for i := range s.Particles {
		p := &s.Particles[i]
		sx := p.Size
		sy := p.Size * p.Aspect
		r := frame.CamRight.Mul(sx)
		u := frame.CamUp.Mul(sy)
		f := frame.CamRight.Cross(frame.CamUp).Mul(p.Size * 0.05)
		model := mgl32.Mat4{
			r[0], r[1], r[2], 0,
			u[0], u[1], u[2], 0,
			f[0], f[1], f[2], 0,
			p.Pos[0], p.Pos[1], p.Pos[2], 1,
		}
		rend.DrawLit(frame.CmdBuf, frame.ScenePass, renderer.LitDrawCall{
			VertexBuffer: snowMesh.VertexBuffer,
			IndexBuffer:  snowMesh.IndexBuffer,
			IndexCount:   snowMesh.IndexCount,
			MVP:          frame.ViewProj.Mul4(model),
			Model:        model,
		})
	}
}
