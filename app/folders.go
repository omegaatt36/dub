package app

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
	"github.com/omegaatt36/dub/web/template"
)

// maxExtraRoots bounds the tree's roots outside home. The oldest one drops out
// first, so a long session cannot grow the sidebar without limit.
const maxExtraRoots = 5

// folderListing is one cached directory read: the subfolders, or why they
// could not be read.
type folderListing struct {
	folders []domain.FolderItem
	err     error
}

// readFolders lists path without touching app state, so callers can run it
// outside the lock: a folder on a slow or hung mount must not freeze the app.
func readFolders(scanner port.Scanner, path string) folderListing {
	folders, err := scanner.ListFolders(path)
	return folderListing{folders: folders, err: err}
}

// handleTreeToggle opens or closes one folder-tree node. It answers with the
// tree alone: the file list and the naming work are untouched by browsing.
//
// Opening a folder is the one moment the tree reads the disk, on the private
// operation snapshot. Every later render reuses that listing, so ticking a
// file or typing a name does not re-list the open folders.
func (a *App) handleTreeToggle(w http.ResponseWriter, r *http.Request) {
	path := r.FormValue("path")

	opening := path != "" && !a.state.Expanded[path]

	var listing folderListing
	if opening {
		listing = readFolders(a.scanner, path)
	}

	if path != "" {
		if a.state.Expanded[path] {
			delete(a.state.Expanded, path)
		} else {
			a.state.Expanded[path] = true
			if opening {
				a.cacheFolders(path, listing)
			}
		}
	}

	renderTempl(w, r, template.FolderTree(a.folderRoots()))
}

// handleTreeVisibility shows or hides the sidebar.
func (a *App) handleTreeVisibility(w http.ResponseWriter, r *http.Request) {
	a.state.HideTree = r.FormValue("hide") == "true"
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// cacheFolders remembers a directory listing on a private snapshot.
func (a *App) cacheFolders(path string, listing folderListing) {
	if a.folderCache == nil {
		a.folderCache = make(map[string]folderListing)
	}
	a.folderCache[path] = listing
}

// listFolders returns path's subfolders from the cache, reading them once if
// they are not there yet. The listing is refreshed when the user opens the
// folder or asks for a refresh. GET snapshots never read the filesystem.
func (a *App) listFolders(path string) folderListing {
	if listing, ok := a.folderCache[path]; ok {
		return listing
	}
	if a.readOnly {
		return folderListing{}
	}
	listing := readFolders(a.scanner, path)
	a.cacheFolders(path, listing)
	return listing
}

// handleScanCancel drops a parked directory request and keeps the user where
// they are.
func (a *App) handleScanCancel(w http.ResponseWriter, r *http.Request) {
	a.state.PendingDir = ""
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// handleScanCancelFast clears PendingDir without queuing behind a slow scan.
func (a *App) handleScanCancelFast(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	a.state.PendingDir = ""
	a.mu.Unlock()
	work := a.snapshot()
	work.readOnly = true
	renderTempl(w, r, template.MainContent(work.buildPageData(nil)))
}

// requestScan opens path unless that would throw away names the user typed by
// hand: those exist nowhere else, while a template or a search rule can be
// re-applied. In that case the request is parked in PendingDir and the page asks
// first. One click in the folder tree is far cheaper than the old dialog, so the
// loss is far easier to cause.
func (a *App) requestScan(path string) {
	if a.hasTypedNames() {
		a.state.PendingDir = a.resolveDir(path)
		return
	}
	a.scanPath(path)
}

// hasTypedNames reports whether the user's own typing is still the source of
// the names, whether for a ticked file or for one they have since unticked. It
// does not look at the active naming tab: switching to Template for a moment
// leaves the typed names in place and in the previews, until a rule is applied
// over them.
func (a *App) hasTypedNames() bool {
	if !a.state.NamesTyped {
		return false
	}
	for _, name := range a.state.NewNames {
		if name != "" {
			return true
		}
	}
	for _, name := range a.state.ManualNames {
		if name != "" {
			return true
		}
	}
	return false
}

// pendingDir is the directory waiting on the user's say-so, or "" once nothing
// typed is left to lose: names deleted or replaced by a rule since the question
// was asked make it moot.
func (a *App) pendingDir() string {
	if a.hasTypedNames() {
		return a.state.PendingDir
	}
	return ""
}

// undoDir names the folder Undo would act in when that is not the one in view.
func (a *App) undoDir() string {
	if a.undoElsewhere() {
		return a.state.LastRenameDir
	}
	return ""
}

// revealInTree opens the chain of folders from the tree root down to path, so
// the directory in view and its subfolders are visible. A path outside the home
// folder becomes an extra root of the tree, unless an earlier one already covers
// it; roots it contains are folded into it.
func (a *App) revealInTree(path string) {
	root, ok := a.treeRootFor(path)
	if !ok {
		a.state.ExtraRoots = slices.DeleteFunc(a.state.ExtraRoots, func(r string) bool {
			return isWithin(path, r)
		})
		a.state.ExtraRoots = append(a.state.ExtraRoots, path)
		if len(a.state.ExtraRoots) > maxExtraRoots {
			a.state.ExtraRoots = a.state.ExtraRoots[len(a.state.ExtraRoots)-maxExtraRoots:]
		}
		root = path
	}

	for dir := path; ; dir = filepath.Dir(dir) {
		a.state.Expanded[dir] = true
		if dir == root {
			return
		}
	}
}

// treeRootFor returns the tree root that contains path.
func (a *App) treeRootFor(path string) (string, bool) {
	for _, root := range append([]string{a.home}, a.state.ExtraRoots...) {
		if root != "" && isWithin(root, path) {
			return root, true
		}
	}
	return "", false
}

// isWithin reports whether path is root or lies below it.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(rel)
}

// folderRoots builds the folder tree: the home folder first, then one root for
// each place the user opened outside it. Exactly one node is the keyboard's
// way in: the folder in view, or the first root when it is not on screen.
func (a *App) folderRoots() []template.FolderNode {
	var roots []template.FolderNode
	if a.home != "" {
		roots = append(roots, a.folderNode(a.home, "Home"))
	}
	for _, root := range a.state.ExtraRoots {
		roots = append(roots, a.folderNode(root, filepath.Base(root)))
	}

	if !markTabStop(roots) && len(roots) > 0 {
		roots[0].TabStop = true
	}
	return roots
}

// markTabStop flags the node for the folder in view and reports whether it
// found one.
func markTabStop(nodes []template.FolderNode) bool {
	for i := range nodes {
		if nodes[i].Current {
			nodes[i].TabStop = true
			return true
		}
		if markTabStop(nodes[i].Children) {
			return true
		}
	}
	return false
}

// folderNode builds a node, listing its children only while it is open (and
// from the cache once it has been), so the tree costs one directory read per
// folder the user opened however large the disk is. A hidden folder stays in
// view when it holds the folder being browsed, or the tree would lose the way
// to where the user is.
func (a *App) folderNode(path, name string) template.FolderNode {
	node := template.FolderNode{
		Name:     name,
		Path:     path,
		Current:  path == a.state.SelectedDirectory,
		Pending:  path == a.pendingDir(),
		Expanded: a.state.Expanded[path],
	}
	if !node.Expanded {
		return node
	}

	listing := a.listFolders(path)
	if listing.err != nil {
		node.Unreadable = true
		return node
	}

	for _, f := range listing.folders {
		onTheWay := a.state.SelectedDirectory != "" && isWithin(f.Path, a.state.SelectedDirectory)
		if f.IsHidden() && !a.state.ShowHiddenFiles && !onTheWay {
			continue
		}
		node.Children = append(node.Children, a.folderNode(f.Path, f.Name))
	}
	return node
}

// breadcrumbs splits dir into clickable segments. A directory under the home
// folder starts at "Home", as the tree names it, anything else at the
// filesystem root.
func breadcrumbs(dir, home string) []template.Crumb {
	if dir == "" {
		return nil
	}

	var (
		crumbs []template.Crumb
		rest   string
	)
	if home != "" && isWithin(home, dir) {
		crumbs = append(crumbs, template.Crumb{Name: "Home", Path: home})
		rest, _ = filepath.Rel(home, dir)
	} else {
		volume := filepath.VolumeName(dir)
		root := volume + string(filepath.Separator)
		crumbs = append(crumbs, template.Crumb{Name: root, Path: root})
		rest = strings.TrimPrefix(dir, root)
	}

	current := crumbs[0].Path
	for seg := range strings.SplitSeq(rest, string(filepath.Separator)) {
		if seg == "" || seg == "." {
			continue
		}
		current = filepath.Join(current, seg)
		crumbs = append(crumbs, template.Crumb{Name: seg, Path: current})
	}
	return crumbs
}
