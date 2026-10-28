package app

import (
	"net/http"

	"github.com/omegaatt36/dub/web/template"
)

// The desktop bridge flushes the editor before posting a native drop. This
// handler already runs inside the operation gate on a private snapshot.
func (a *App) handleDrop(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.state.Error = "Failed to read dropped paths"
	} else {
		a.dropFiles(r.Form["path"], r.FormValue("target"))
	}
	renderTempl(w, r, template.MainContent(a.buildPageData(nil)))
}
