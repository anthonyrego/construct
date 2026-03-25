package mapdata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store holds all map data loaded from a data/map/ directory.
type Store struct {
	Dir           string
	Meta          MapMeta
	Blocks        map[string]*BlockData
	Intersections map[string]*IntersectionData
	Surfaces      map[string]*SurfaceFileData
	Doodads       map[string]*DoodadFileData
}

// Load reads all JSON files from a map data directory.
func Load(dir string) (*Store, error) {
	s := &Store{
		Dir:           dir,
		Blocks:        make(map[string]*BlockData),
		Intersections: make(map[string]*IntersectionData),
		Surfaces:      make(map[string]*SurfaceFileData),
		Doodads:       make(map[string]*DoodadFileData),
	}

	// meta.json
	if err := readJSON(filepath.Join(dir, "meta.json"), &s.Meta); err != nil {
		return nil, fmt.Errorf("reading meta.json: %w", err)
	}

	// blocks/*.json
	blockFiles, _ := filepath.Glob(filepath.Join(dir, "blocks", "*.json"))
	for _, f := range blockFiles {
		var b BlockData
		if err := readJSON(f, &b); err != nil {
			fmt.Printf("Warning: skipping block file %s: %v\n", f, err)
			continue
		}
		s.Blocks[b.Block] = &b
	}

	// intersections/*.json
	intFiles, _ := filepath.Glob(filepath.Join(dir, "intersections", "*.json"))
	for _, f := range intFiles {
		var d IntersectionData
		if err := readJSON(f, &d); err != nil {
			fmt.Printf("Warning: skipping intersection file %s: %v\n", f, err)
			continue
		}
		s.Intersections[d.ID] = &d
	}

	// surfaces/*.json
	surfFiles, _ := filepath.Glob(filepath.Join(dir, "surfaces", "*.json"))
	for _, f := range surfFiles {
		var d SurfaceFileData
		if err := readJSON(f, &d); err != nil {
			fmt.Printf("Warning: skipping surface file %s: %v\n", f, err)
			continue
		}
		s.Surfaces[d.Type] = &d
	}

	// doodads/*.json
	doodadFiles, _ := filepath.Glob(filepath.Join(dir, "doodads", "*.json"))
	for _, f := range doodadFiles {
		var d DoodadFileData
		if err := readJSON(f, &d); err != nil {
			fmt.Printf("Warning: skipping doodad file %s: %v\n", f, err)
			continue
		}
		s.Doodads[d.Type] = &d
	}

	return s, nil
}

// SaveBlock writes a single block file.
func (s *Store) SaveBlock(blockID string) error {
	b, ok := s.Blocks[blockID]
	if !ok {
		return fmt.Errorf("block %q not found", blockID)
	}
	return writeJSON(filepath.Join(s.Dir, "blocks", blockID+".json"), b)
}

// SaveIntersection writes a single intersection file.
func (s *Store) SaveIntersection(id string) error {
	d, ok := s.Intersections[id]
	if !ok {
		return fmt.Errorf("intersection %q not found", id)
	}
	return writeJSON(filepath.Join(s.Dir, "intersections", id+".json"), d)
}

// SaveSurface writes a single surface file.
func (s *Store) SaveSurface(typ string) error {
	d, ok := s.Surfaces[typ]
	if !ok {
		return fmt.Errorf("surface type %q not found", typ)
	}
	return writeJSON(filepath.Join(s.Dir, "surfaces", typ+"s.json"), d)
}

// SaveDoodad writes a single doodad file.
func (s *Store) SaveDoodad(typ string) error {
	d, ok := s.Doodads[typ]
	if !ok {
		return fmt.Errorf("doodad type %q not found", typ)
	}
	return writeJSON(filepath.Join(s.Dir, "doodads", typ+"s.json"), d)
}

// FindDoodadByID searches doodad items of the given type by ID.
// Returns the item pointer and index, or nil/-1 if not found.
func (s *Store) FindDoodadByID(typ, id string) (*DoodadItem, int) {
	d, ok := s.Doodads[typ]
	if !ok {
		return nil, -1
	}
	for i := range d.Items {
		if d.Items[i].ID == id {
			return &d.Items[i], i
		}
	}
	return nil, -1
}

// FindBuildingByBBL looks up a building by its BBL identifier.
// Returns the building and its block ID, or nil if not found.
func (s *Store) FindBuildingByBBL(bbl string) (*BuildingData, string) {
	for blockID, block := range s.Blocks {
		for i := range block.Buildings {
			if block.Buildings[i].BBL == bbl {
				return &block.Buildings[i], blockID
			}
		}
	}
	return nil, ""
}

// FindBuildingByAddress searches for a building by address substring (case-insensitive).
func (s *Store) FindBuildingByAddress(query string) (*BuildingData, string) {
	q := strings.ToLower(query)
	for blockID, block := range s.Blocks {
		for i := range block.Buildings {
			if strings.Contains(strings.ToLower(block.Buildings[i].Address), q) {
				return &block.Buildings[i], blockID
			}
		}
	}
	return nil, ""
}

// FindIntersection looks up an intersection by street names (order-independent).
func (s *Store) FindIntersection(street1, street2 string) *IntersectionData {
	s1 := strings.ToLower(street1)
	s2 := strings.ToLower(street2)
	for _, d := range s.Intersections {
		ds1 := strings.ToLower(d.Street1)
		ds2 := strings.ToLower(d.Street2)
		if (strings.Contains(ds1, s1) && strings.Contains(ds2, s2)) ||
			(strings.Contains(ds1, s2) && strings.Contains(ds2, s1)) {
			return d
		}
	}
	return nil
}

func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
