package app

import (
	"errors"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

type recoveryRenamer struct {
	port.Renamer
	pending     bool
	statusError error
	success     bool
	rolledBack  bool
	directory   string
}

func (r *recoveryRenamer) RecoveryPending(string) (bool, error) {
	return r.pending, r.statusError
}

func (r *recoveryRenamer) RecoverRename(dir string) domain.RenameResult {
	r.directory = dir
	if r.success {
		r.pending = false
	}
	return domain.RenameResult{Success: r.success, RolledBack: r.rolledBack}
}

func TestScanAndRefreshCheckRecovery(t *testing.T) {
	a := newTestApp()
	renamer := &recoveryRenamer{Renamer: a.renamer, pending: true}
	a.renamer = renamer
	body := renderPost(t, a, "/api/scan", url.Values{"path": {"/dir"}})
	assert.True(t, a.snapshot().state.RecoveryPending)
	assert.Contains(t, body, `hx-post="/api/recover"`)
	assert.Contains(t, body, "An interrupted rename needs recovery.")
	assert.NotContains(t, body, `hx-post="/api/execute"`)
	renamer.pending = false
	body = renderPost(t, a, "/api/refresh", nil)
	assert.False(t, a.snapshot().state.RecoveryPending)
	assert.NotContains(t, body, `hx-post="/api/recover"`)
	renamer.statusError = errors.New("cannot inspect journal")
	renderPost(t, a, "/api/refresh", nil)
	assert.True(t, a.snapshot().state.RecoveryPending, "status errors must fail closed")
	assert.Contains(t, a.snapshot().state.Error, "cannot inspect journal")
}

func TestRecoveryClearsDraftOnlyAfterActualSuccessfulRecovery(t *testing.T) {
	for _, test := range []struct {
		name                                  string
		pending, success, rolledBack, cleared bool
	}{
		{"recovered", true, true, true, true},
		{"failed", true, false, false, false},
		{"no journal", false, true, false, false},
		{"another process removed journal", true, true, false, false},
		{"another process created journal", false, true, true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := newTestApp()
			renamer := &recoveryRenamer{
				Renamer: a.renamer, pending: test.pending,
				success: test.success, rolledBack: test.rolledBack,
			}
			a.renamer = renamer
			a.state.SelectedDirectory = "/dir"
			namingDraft(a)
			a.state.CanUndo = true
			a.state.LastRenameHistory = a.state.Previews
			a.state.LastRenameDir = "/dir"
			renderPost(t, a, "/api/recover", nil)
			assert.Equal(t, "/dir", renamer.directory)
			if test.cleared {
				assert.Empty(t, a.snapshot().state.NewNames)
				assert.Empty(t, a.snapshot().state.LastRenameHistory)
				assert.False(t, a.snapshot().state.CanUndo)
			} else {
				assert.Equal(t, []string{"keep.txt"}, a.snapshot().state.NewNames)
				assert.True(t, a.snapshot().state.CanUndo)
			}
			assert.Equal(t, test.pending && !test.success, a.snapshot().state.RecoveryPending)
		})
	}
}

func TestRecoveryPreservesUndoInAnotherDirectory(t *testing.T) {
	a := newTestApp()
	a.renamer = &recoveryRenamer{Renamer: a.renamer, pending: true, success: true, rolledBack: true}
	a.state.SelectedDirectory = "/recovery-dir"
	a.state.CanUndo = true
	a.state.LastRenameDir = "/other-dir"
	history := []domain.RenamePreview{{OriginalPath: "/other-dir/a", NewPath: "/other-dir/b", SourceID: "owned"}}
	a.state.LastRenameHistory = history

	renderPost(t, a, "/api/recover", nil)
	state := a.snapshot().state
	assert.True(t, state.CanUndo)
	assert.Equal(t, "/other-dir", state.LastRenameDir)
	assert.Equal(t, history, state.LastRenameHistory)
}
