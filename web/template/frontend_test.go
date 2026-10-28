package template

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/omegaatt36/dub/internal/domain"
)

func renderFrontend(t *testing.T, component templ.Component) string {
	t.Helper()
	var output bytes.Buffer
	if err := component.Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestPreviewRetainsVisibleFilesAndSelection(t *testing.T) {
	files := []domain.FileItem{{Name: "a.txt", Path: "/a.txt"}, {Name: "b.txt", Path: "/b.txt"}, {Name: "c.txt", Path: "/c.txt"}}
	html := renderFrontend(t, FileList(PageData{VisibleFiles: files, SelectedPaths: map[string]bool{"/a.txt": true, "/c.txt": true}, Previews: []domain.RenamePreview{{OriginalName: "c.txt", OriginalPath: "/c.txt", NewName: "new-c.txt"}, {OriginalName: "a.txt", OriginalPath: "/a.txt", NewName: "new-a.txt"}}}))
	for _, file := range files {
		if !strings.Contains(html, `aria-label="Include `+file.Name+` in the rename"`) {
			t.Errorf("missing checkbox for %s", file.Name)
		}
	}
	rows := strings.Split(html, "<tr")
	for _, pair := range [][2]string{{"a.txt", "new-a.txt"}, {"b.txt", ""}, {"c.txt", "new-c.txt"}} {
		found := false
		for _, row := range rows {
			if !strings.Contains(row, `title="`+pair[0]+`"`) {
				continue
			}
			found = true
			if pair[1] != "" && !strings.Contains(row, pair[1]) {
				t.Errorf("preview not matched to %s", pair[0])
			}
			if pair[0] == "b.txt" && strings.Contains(row, "checked") {
				t.Error("excluded file selected")
			}
		}
		if !found {
			t.Errorf("missing visible row %s", pair[0])
		}
	}
}

func TestManualEditorIdentifiesOriginalFile(t *testing.T) {
	html := renderFrontend(t, ManualEditor([]domain.FileItem{{Name: "original.pdf", Path: "/original.pdf", Extension: ".pdf"}}, []string{"draft"}))
	if !strings.Contains(html, `aria-label="New name for original.pdf"`) {
		t.Error("manual input accessible name must identify original")
	}
	if !strings.Contains(html, `>original.pdf</`) {
		t.Error("original filename must remain visible after typing")
	}
}

func TestFileUploadHasAccessibleNameAndFocus(t *testing.T) {
	html := renderFrontend(t, FileEditor(nil))
	if !strings.Contains(html, `aria-label="Upload names from a text or CSV file"`) {
		t.Error("upload requires accessible name")
	}
	if !strings.Contains(html, "focus-within:ring-2") {
		t.Error("invisible file input requires visible focus on container")
	}
}

func TestDisabledPatternUsesNativeDisable(t *testing.T) {
	html := renderFrontend(t, PatternInput("", 0, 0, "", false))
	if !strings.Contains(html, "<fieldset disabled") {
		t.Error("pattern controls must be disabled for keyboard users")
	}
}

func TestOptionalCheckboxesUseNativeBooleanState(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, component := range []templ.Component{extensionToggle(enabled), FileList(PageData{ShowHiddenFiles: enabled})} {
			html := renderFrontend(t, component)
			if strings.Contains(html, `checked="false"`) {
				t.Error("false boolean attribute still enables checkbox")
			}
			if enabled && !strings.Contains(html, " checked") {
				t.Error("enabled checkbox must be checked")
			}
		}
	}
}

func TestRecoveryRequiresExplicitActionAndBlocksExecution(t *testing.T) {
	html := renderFrontend(t, MainContent(PageData{
		RecoveryPending: true,
		Previews:        []domain.RenamePreview{{OriginalName: "a", NewName: "b"}},
	}))
	if !strings.Contains(html, `hx-post="/api/recover"`) || !strings.Contains(html, "Review recovery") {
		t.Error("pending recovery must offer a discoverable explicit action")
	}
	if strings.Contains(html, `hx-post="/api/execute"`) {
		t.Error("pending recovery must not offer execution")
	}
	if !strings.Contains(html, "clears the current naming draft") {
		t.Error("recovery must explain which work it discards")
	}
	if html := renderFrontend(t, RecoveryPrompt(false)); html != "" {
		t.Error("normal directories must not show recovery controls")
	}
}

func TestRecoveryBlocksUndoOnlyInAffectedDirectory(t *testing.T) {
	for _, undoDir := range []string{"", "/elsewhere"} {
		for _, result := range []*domain.RenameResult{nil, {Success: true, Message: "No interrupted rename"}} {
			html := renderFrontend(t, Actions(PageData{
				RecoveryPending: true, CanUndo: true, UndoDir: undoDir, Result: result,
			}))
			if strings.Contains(html, `hx-post="/api/undo"`) != (undoDir != "") {
				t.Fatalf("undo availability does not match recovery scope %q with result %+v", undoDir, result)
			}
		}
	}
}
