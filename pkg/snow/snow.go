package snow

import (
	"math"
	"math/rand"

	"github.com/go-gl/mathgl/mgl32"
)

type Particle struct {
	Pos    mgl32.Vec3
	VelY   float32 // fall speed (negative)
	Phase  float32 // unique phase for wind sine
	Size   float32 // base width
	Aspect float32 // height/width ratio (0.4–1.0)
}

type System struct {
	Particles    []Particle
	WindTime     float32
	WindStrength float32
	FallSpeed    float32
	ParticleSize float32
	MinX, MaxX   float32
	MinZ, MaxZ   float32
	MinY, MaxY   float32
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
		MinX:         -9,
		MaxX:         9,
		MinZ:         -44,
		MaxZ:         4,
		MinY:         -0.5,
		MaxY:         16,
	}

	for i := range s.Particles {
		s.spawn(&s.Particles[i], true)
	}

	return s
}

func (s *System) spawn(p *Particle, randomY bool) {
	p.Pos = mgl32.Vec3{
		s.MinX + rand.Float32()*(s.MaxX-s.MinX),
		0,
		s.MinZ + rand.Float32()*(s.MaxZ-s.MinZ),
	}
	if randomY {
		p.Pos[1] = s.MinY + rand.Float32()*(s.MaxY-s.MinY)
	} else {
		p.Pos[1] = s.MaxY + rand.Float32()*3
	}
	// Vary fall speed per particle
	p.VelY = -(s.FallSpeed * (0.6 + rand.Float32()*0.8))
	p.Phase = rand.Float32() * math.Pi * 2
	// Vary size and aspect per particle
	p.Size = s.ParticleSize * (0.5 + rand.Float32())
	p.Aspect = 0.4 + rand.Float32()*0.6
}

func (s *System) Update(dt float32) {
	s.WindTime += dt

	ws := s.WindStrength
	t := s.WindTime

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

		// Respawn when below ground or out of bounds
		if p.Pos[1] < s.MinY {
			s.spawn(p, false)
		} else if p.Pos[0] < s.MinX-3 || p.Pos[0] > s.MaxX+3 ||
			p.Pos[2] < s.MinZ-3 || p.Pos[2] > s.MaxZ+3 {
			s.spawn(p, false)
		}
	}
}
