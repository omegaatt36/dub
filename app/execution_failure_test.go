package app

import (
	"net/url"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

type rejectingRenamer struct{ port.Renamer }

func (r *rejectingRenamer) ExecuteRename([]domain.RenamePreview) domain.RenameResult {
	return domain.RenameResult{Message: "source changed", Errors: []string{"source changed"}}
}

func TestRejectedExecutePreservesNamingWorkAndSelection(t *testing.T) {
	for _, method := range []string{"manual", "file", "template"} {
		t.Run(method, func(t *testing.T) {
			a, dir := newUXApp(t)
			renderPost(t, a, "/api/tick", url.Values{
				"path": {filepath.Join(dir, "c.pdf")}, "ticked": {"false"},
			})
			renderPost(t, a, "/api/names", url.Values{
				"method": {"manual"}, "action": {"update"},
				"name_0": {"keep-a"}, "name_1": {"keep-b"},
			})
			a.state.AppliedMethod = method
			a.state.NamingMethod = method
			if method == "file" {
				a.state.LoadedNames = append([]string(nil), a.state.NewNames...)
			}
			if method == "template" {
				renderPost(t, a, "/api/names/generate", url.Values{"template": {"keep_{index}"}})
			}
			before := a.snapshot().state
			require.NotEmpty(t, before.Previews)
			a.renamer = &rejectingRenamer{Renamer: a.renamer}

			body := renderPost(t, a, "/api/execute", nil)
			after := a.snapshot().state
			assert.Contains(t, body, "source changed")
			assert.Equal(t, before.NewNames, after.NewNames)
			assert.Equal(t, before.LoadedNames, after.LoadedNames)
			assert.Equal(t, before.ManualNames, after.ManualNames)
			assert.Equal(t, before.AppliedMethod, after.AppliedMethod)
			assert.Equal(t, before.Selected, after.Selected)
			assert.NotEmpty(t, after.Previews, "retry must use regenerated previews")
		})
	}
}
