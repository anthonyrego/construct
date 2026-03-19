package mapdata

import (
	"os"
	"path/filepath"
	"time"
)

// MapWatcher polls a map data directory for changes and signals when a reload is needed.
type MapWatcher struct {
	dir          string
	subdirs      []string
	state        map[string]dirSnapshot
	metaMod      time.Time
	lastPoll     time.Time
	pollInterval time.Duration
	changeTime   time.Time
	debounce     time.Duration
	pending      bool
}

type dirSnapshot struct {
	fileCount int
	newest    time.Time
}

// NewWatcher creates a watcher that monitors the given map data directory.
// It takes an initial snapshot so the first Check() won't trigger a reload.
func NewWatcher(dir string) *MapWatcher {
	w := &MapWatcher{
		dir:          dir,
		subdirs:      []string{"blocks", "intersections", "surfaces", "doodads"},
		state:        make(map[string]dirSnapshot),
		pollInterval: time.Second,
		debounce:     300 * time.Millisecond,
	}
	w.snapshot()
	w.lastPoll = time.Now()
	return w
}

// Check returns true when map data files have changed and a reload should occur.
// Call once per frame. Polls at most once per second, then debounces 300ms.
func (w *MapWatcher) Check() bool {
	now := time.Now()

	// If a change was detected and debounce has elapsed, trigger reload
	if w.pending && now.Sub(w.changeTime) >= w.debounce {
		w.pending = false
		w.snapshot() // take new baseline
		w.lastPoll = now
		return true
	}

	// Throttle polling to once per second
	if now.Sub(w.lastPoll) < w.pollInterval {
		return false
	}
	w.lastPoll = now

	if w.changed() {
		if !w.pending {
			w.pending = true
			w.changeTime = now
		}
	}

	return false
}

// snapshot captures the current state of all monitored directories.
func (w *MapWatcher) snapshot() {
	for _, sub := range w.subdirs {
		w.state[sub] = w.scanDir(filepath.Join(w.dir, sub))
	}
	// Also track meta.json
	if info, err := os.Stat(filepath.Join(w.dir, "meta.json")); err == nil {
		w.metaMod = info.ModTime()
	}
}

// changed returns true if any monitored directory differs from the stored snapshot.
func (w *MapWatcher) changed() bool {
	// Check meta.json
	if info, err := os.Stat(filepath.Join(w.dir, "meta.json")); err == nil {
		if info.ModTime().After(w.metaMod) {
			return true
		}
	}

	for _, sub := range w.subdirs {
		cur := w.scanDir(filepath.Join(w.dir, sub))
		prev := w.state[sub]
		if cur.fileCount != prev.fileCount || cur.newest != prev.newest {
			return true
		}
	}
	return false
}

// Reset takes a fresh snapshot and clears pending state.
// Use after an admin save to prevent double-triggering a reload.
func (w *MapWatcher) Reset() {
	w.snapshot()
	w.pending = false
}

// scanDir counts files and finds the newest modification time in a directory.
func (w *MapWatcher) scanDir(dir string) dirSnapshot {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dirSnapshot{}
	}

	var snap dirSnapshot
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		snap.fileCount++
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(snap.newest) {
			snap.newest = info.ModTime()
		}
	}
	return snap
}
