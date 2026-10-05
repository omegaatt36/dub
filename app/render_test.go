package app

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests assert on rendered markup rather than state. Templating bugs are
// invisible to state assertions: a chip whose onclick silently rendered empty
// still "works" as far as the backend is concerned.

func renderPost(t *testing.T, app *App, path string, form url.Values) string {
	t.Helper()
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	app.GetHandler().ServeHTTP(rec, req)
	return rec.Body.String()
}

func renderGet(t *testing.T, app *App, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	app.GetHandler().ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Body.String()
}

// TestChipHandlersRender guards a bug where templ emitted onclick={...} as an
// empty attribute plus a stray <script> block, leaving every insert chip dead.
func TestChipHandlersRender(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	form := url.Values{"method": {"template"}}
	body := renderPost(t, app, "/api/names", form)

	for _, token := range []string{"{index}", "{original}", "{ext}", "{date}", "{parent}", "{original|upper}"} {
		assert.Contains(t, body, "data-append-to-template=", "chip for "+token+" must carry its token")
	}
	assert.Contains(t, body, "data-append-to-template=\"{index}\"")

	// The regression itself: an empty handler attribute.
	assert.NotContains(t, body, `onclick=""`, "chips must not render an empty onclick")
	// templ must not have fallen back to emitting a script block for them.
	assert.NotContains(t, body, "<script>appendTo", "chips must not render as a script block")
}

func TestFindReplaceChipHandlersRender(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	body := renderPost(t, app, "/api/names", url.Values{"method": {"findreplace"}})

	assert.Contains(t, body, "data-append-field=")
	assert.Contains(t, body, "data-append-text=")
	assert.NotContains(t, body, `onclick=""`)
	assert.NotContains(t, body, "<script>appendTo")
}

// TestFileListRendersTicks verifies the selection column is actually emitted.
func TestFileListRendersTicks(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	body := renderGet(t, app, "/api/page")

	assert.Contains(t, body, `type="checkbox"`)
	assert.Contains(t, body, "/api/tick")
	assert.Contains(t, body, "/api/tick-all")
}

// TestActionBarSpansWindow verifies the destructive control lives in the
// always-visible footer with an explicit blast radius.
func TestActionBarSpansWindow(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	body := renderPost(t, app, "/api/names/generate", url.Values{"template": {"trip_{index}"}})

	assert.Contains(t, body, "Ready to rename")
	assert.Contains(t, body, `id="execute-btn"`)
	assert.Contains(t, body, `id="execute-confirm"`)
	// The confirm half must start hidden, so the destructive button is not the
	// initial state. Tailwind's preflight makes [hidden] win over display:flex.
	assert.Contains(t, body, `id="execute-confirm" hidden`)
	// The footer is rendered outside the two-column grid, so it spans the window.
	assert.Contains(t, body, "shrink-0 border-t")
}

// TestNoDuplicatePreviewList guards the redundancy that made the two panels
// disagree about whether the extension was included.
func TestNoDuplicatePreviewList(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	for _, method := range []string{"template", "findreplace", "file"} {
		body := renderPost(t, app, "/api/names", url.Values{"method": {method}})
		assert.NotContains(t, body, "PREVIEW", method+" must not duplicate the file list")
		assert.NotContains(t, body, "Preview (", method+" must not duplicate the file list")
	}
}

// TestScrollContainersAreMarked verifies the elements whose scroll offset
// bridge.js restores actually carry the marker it looks up.
func TestScrollContainersAreMarked(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	body := renderGet(t, app, "/api/page")

	assert.Contains(t, body, `data-dub-scroll="file-list"`)
	assert.Contains(t, body, `data-dub-scroll="names-panel"`)
}

// TestNoDeadPlaceholders verifies removed markup is actually gone.
func TestNoDeadPlaceholders(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	body := renderGet(t, app, "/api/page")

	assert.NotContains(t, body, "Select a directory first.", "empty state must reflect selection")
}

// TestTabsUseRovingTabindex verifies the tablist follows the ARIA pattern so
// arrow-key navigation can work.
func TestTabsUseRovingTabindex(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	body := renderPost(t, app, "/api/names", url.Values{"method": {"findreplace"}})

	require.Contains(t, body, `role="tablist"`)
	require.Contains(t, body, `role="tabpanel"`)
	assert.Contains(t, body, `aria-selected="true"`)
	// Exactly one tab is in the tab order.
	assert.Equal(t, 1, strings.Count(body, `tabindex="0"`), "only the active tab should be tabbable")
	assert.Equal(t, 3, strings.Count(body, `tabindex="-1"`))
}

// TestExtensionToggleRenders verifies the opt-in control exists and is off by
// default. Without a visible, disabled-by-default control, extension rewriting
// would be reachable but undiscoverable.
func TestExtensionToggleRenders(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)
	body := renderPost(t, app, "/api/names", url.Values{"method": {"template"}})

	assert.Contains(t, body, "/api/extension-policy")
	assert.Contains(t, body, "Allow changing the extension")
	// templ emits boolean attributes literally, so "off" is checked="false".
	assert.Contains(t, body, `checked="false"`, "extension conversion must default to off")
}

// TestExtensionWarningRenders verifies the confirm step warns about the real
// blast radius, not just the policy being enabled.
func TestExtensionWarningRenders(t *testing.T) {
	app := newSelectionApp()
	scanSelectionApp(t, app)

	// Enable conversion, then generate names that change the extension.
	renderPost(t, app, "/api/extension-policy", url.Values{"convert": {"true"}})
	body := renderPost(t, app, "/api/names/generate", url.Values{"template": {"trip_{index}.md"}})

	assert.Contains(t, body, "3 extensions will change")
	assert.Contains(t, body, "contents are not converted")
}
