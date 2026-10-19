package app

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/omegaatt36/dub/web/template"
)

// handleTreeToggle opens or closes one folder-tree node. It answers with the
// tree alone: the file list and the naming work are untouched by browsing.
func (a *App) handleTreeToggle(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if path := r.FormValue("path"); path != "" {
		if a.state.Expanded[path] {
			delete(a.state.Expanded, path)
		} else {
			a.state.Expanded[path] = true
		}
	}

	renderTempl(w, r, template.FolderTree(a.folderRoots()))
}

// handleScanCancel drops a parked directory request and keeps the user where
// they are.
func (a *App) handleScanCancel(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.PendingDir = ""
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}

// requestScan opens path unless that would throw away names the user typed by
// hand: those exist nowhere else, while a template or a search rule can be
// re-applied. In that case the request is parked in PendingDir and the page asks
// first. One click in the folder tree is far cheaper than the old dialog, so the
// loss is far easier to cause. Callers hold a.mu.
func (a *App) requestScan(path string) {
	if a.hasTypedNames() {
		a.state.PendingDir = a.resolveDir(path)
		return
	}
	a.scanPath(path)
}

// hasTypedNames reports whether the manual editor holds any name the user
// entered, whether for a ticked file or for one they have since unticked.
func (a *App) hasTypedNames() bool {
	if a.state.NamingMethod != "manual" {
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

// revealInTree opens the chain of folders from the tree root down to path, so
// the directory in view and its subfolders are visible. A path outside the home
// folder becomes the tree's extra root, unless an earlier one already covers it.
// Callers hold a.mu.
func (a *App) revealInTree(path string) {
	root, ok := a.treeRootFor(path)
	if !ok {
		a.state.ExtraRoot = path
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
	for _, root := range []string{a.home, a.state.ExtraRoot} {
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

// folderRoots builds the folder tree: the home folder first, then the extra
// root when the user opened something outside it.
func (a *App) folderRoots() []template.FolderNode {
	var roots []template.FolderNode
	if a.home != "" {
		roots = append(roots, a.folderNode(a.home, "Home"))
	}
	if a.state.ExtraRoot != "" {
		roots = append(roots, a.folderNode(a.state.ExtraRoot, filepath.Base(a.state.ExtraRoot)))
	}
	return roots
}

// folderNode reads a node's children only while it is open, so the tree costs
// one directory read per open folder however large the disk is. Folders are
// matched by name and never followed through symlinks, which rules out cycles.
func (a *App) folderNode(path, name string) template.FolderNode {
	node := template.FolderNode{
		Name:     name,
		Path:     path,
		Current:  path == a.state.SelectedDirectory,
		Expanded: a.state.Expanded[path],
	}
	if !node.Expanded {
		return node
	}

	folders, err := a.scanner.ListFolders(path)
	if err != nil {
		node.Unreadable = true
		return node
	}

	for _, f := range folders {
		if f.IsHidden() && !a.state.ShowHiddenFiles {
			continue
		}
		node.Children = append(node.Children, a.folderNode(f.Path, f.Name))
	}
	return node
}

// breadcrumbs splits dir into clickable segments. A directory under the home
// folder starts at "~", anything else at the filesystem root.
func breadcrumbs(dir, home string) []template.Crumb {
	if dir == "" {
		return nil
	}

	var (
		crumbs []template.Crumb
		rest   string
	)
	if home != "" && isWithin(home, dir) {
		crumbs = append(crumbs, template.Crumb{Name: "~", Path: home})
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
