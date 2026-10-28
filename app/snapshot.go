package app

import (
	"maps"
	"slices"
)

// snapshot owns every mutable collection. Handlers run only on these private
// copies; the published state's lock is never held during IO or rendering.
func (a *App) snapshot() *App {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := *a.state
	s.AllFiles = slices.Clone(s.AllFiles)
	s.MatchedFiles = slices.Clone(s.MatchedFiles)
	s.Selected = maps.Clone(s.Selected)
	s.ManualNames = maps.Clone(s.ManualNames)
	s.Expanded = maps.Clone(s.Expanded)
	s.NewNames = slices.Clone(s.NewNames)
	s.LoadedNames = slices.Clone(s.LoadedNames)
	s.Previews = slices.Clone(s.Previews)
	s.LastRenameHistory = slices.Clone(s.LastRenameHistory)
	s.ExtraRoots = slices.Clone(s.ExtraRoots)
	cache := make(map[string]folderListing, len(a.folderCache))
	for path, listing := range a.folderCache {
		listing.folders = slices.Clone(listing.folders)
		cache[path] = listing
	}
	return &App{
		fs: a.fs, scanner: a.scanner, pattern: a.pattern, renamer: a.renamer,
		state: &s, pickDirectory: a.pickDirectory, home: a.home,
		logger: a.logger, folderCache: cache,
	}
}

func (a *App) publish(work *App) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state = work.state
	a.folderCache = work.folderCache
}
