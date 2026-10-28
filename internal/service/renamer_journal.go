package service

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/omegaatt36/dub/internal/domain"
	"github.com/omegaatt36/dub/internal/port"
)

// The immutable intent is synced before the first move. Recovery restores the
// originals, even if a process stopped after the last move but before cleanup.
// This is process-crash recovery, not a power-loss transaction: filesystem and
// hardware ordering/durability guarantees vary, and ambiguous state is blocked.
type renameJournal struct {
	Version     int
	DirectoryID string
	Entries     []journalEntry
}

type journalEntry struct {
	Original string
	Target   string
	Temp     string
	SourceID string
}

type renameMove struct{ from, to, identity string }

func executeJournal(d port.RenameDirectory, directoryID string, previews []domain.RenamePreview) domain.RenameResult {
	j := renameJournal{Version: 1, DirectoryID: directoryID}
	for _, p := range previews {
		if p.OriginalName != p.NewName {
			j.Entries = append(j.Entries, journalEntry{Original: p.OriginalName, Target: p.NewName, Temp: ".dub-rename-" + rand.Text(), SourceID: p.SourceID})
		}
	}
	if len(j.Entries) == 0 {
		return domain.RenameResult{Success: true, Message: "Successfully renamed 0 files"}
	}
	data, err := json.Marshal(j)
	if err != nil {
		return renameFailure(err)
	}
	if len(data) > 4<<20 {
		return renameFailure(fmt.Errorf("rename journal exceeds size limit"))
	}
	if err := d.CreateJournal(data); err != nil {
		return renameFailure(fmt.Errorf("create rename journal: %w", err))
	}
	var moves []renameMove
	rollback := func(cause error) domain.RenameResult {
		result := renameFailure(cause)
		result.RolledBack = true
		for i := len(moves) - 1; i >= 0; i-- {
			m := moves[i]
			if err := checkedMove(d, m.to, m.from, m.identity); err != nil {
				result.RollbackErrors = append(result.RollbackErrors, err.Error())
			}
		}
		if len(result.RollbackErrors) == 0 {
			if err := d.RemoveJournal(); err != nil {
				result.RollbackErrors = append(result.RollbackErrors, err.Error())
			}
		}
		return result
	}
	move := func(from, to, id string) error {
		if err := verifyEntry(d, from, id); err != nil {
			return err
		}
		if err := d.Rename(from, to); err != nil {
			return err
		}
		// Verify again after staging: a writer can replace the source between
		// stat and rename. Only restore the entry actually moved, never guess.
		actual, err := d.EntryIdentity(to)
		if err != nil {
			// Keep the journal; a missing/unknown moved entry is not recoverable.
			moves = append(moves, renameMove{from, to, ""})
			return err
		}
		moves = append(moves, renameMove{from, to, actual})
		if actual != id {
			return fmt.Errorf("entry %q changed during rename", from)
		}
		return nil
	}
	for _, e := range j.Entries {
		if err := move(e.Original, e.Temp, e.SourceID); err != nil {
			return rollback(err)
		}
	}
	for _, e := range j.Entries {
		if err := move(e.Temp, e.Target, e.SourceID); err != nil {
			return rollback(err)
		}
	}
	if err := d.RemoveJournal(); err != nil {
		return renameFailure(fmt.Errorf("renames completed but journal cleanup failed: %w", err))
	}
	return domain.RenameResult{Success: true, RenamedCount: len(j.Entries), Message: fmt.Sprintf("Successfully renamed %d files", len(j.Entries))}
}

func checkedMove(d port.RenameDirectory, from, to, id string) error {
	if err := verifyEntry(d, from, id); err != nil {
		return err
	}
	if err := d.Rename(from, to); err != nil {
		return fmt.Errorf("restore %q from %q: %w", to, from, err)
	}
	return verifyEntry(d, to, id)
}

// RecoveryPending is read-only. Even an unreadable or malformed journal blocks
// new operations; the caller can show the error and an explicit recovery action.
func (s *RenamerService) RecoveryPending(directory string) (bool, error) {
	opener, ok := s.fs.(port.RenameDirectoryOpener)
	if !ok {
		// Legacy preview-only filesystems cannot create journals. Execution
		// still rejects these filesystems in openLocked.
		return false, nil
	}
	d, err := opener.OpenRenameDirectory(directory)
	if err != nil {
		return false, err
	}
	defer d.Close()
	data, err := d.ReadJournal()
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return true, err
	}
	_, err = decodeJournal(d, data)
	return true, err
}

func decodeJournal(d port.RenameDirectory, data []byte) (renameJournal, error) {
	var j renameJournal
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&j); err != nil {
		return j, fmt.Errorf("malformed rename journal: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return j, fmt.Errorf("trailing data in rename journal")
	}
	// Only our canonical encoding is accepted. Besides bounding the format,
	// this rejects duplicate JSON keys, which encoding/json otherwise accepts
	// with last-key-wins semantics and could make recovery ambiguous.
	canonical, err := json.Marshal(j)
	if err != nil || !bytes.Equal(data, canonical) {
		return j, fmt.Errorf("noncanonical or ambiguous rename journal")
	}
	id, err := d.Identity()
	if err != nil {
		return j, err
	}
	if j.Version != 1 || j.DirectoryID == "" || j.DirectoryID != id || len(j.Entries) == 0 {
		return j, fmt.Errorf("unsupported journal or directory identity mismatch")
	}
	originals, targets, temps, ids := map[string]bool{}, map[string]bool{}, map[string]bool{}, map[string]bool{}
	value, known := d.CaseSensitive()
	sensitive := value && known
	for _, e := range j.Entries {
		original, target, temp := nameKey(e.Original, sensitive), nameKey(e.Target, sensitive), nameKey(e.Temp, sensitive)
		if validateFileName(e.Original) != nil || validateFileName(e.Target) != nil ||
			e.Original == e.Target || e.SourceID == "" || ids[e.SourceID] ||
			!strings.HasPrefix(e.Temp, ".dub-rename-") || e.Temp == ".dub-rename-journal" ||
			strings.ContainsAny(e.Temp, "/\\\x00") || originals[original] || targets[target] || temps[temp] {
			return j, fmt.Errorf("invalid or ambiguous journal entries")
		}
		originals[original], targets[target], temps[temp], ids[e.SourceID] = true, true, true, true
	}
	return j, nil
}

// RecoverRename explicitly rolls back an interrupted operation. It checks all
// entries before moving any, refuses outsiders/ambiguous identities, and leaves
// the journal in place whenever recovery cannot complete.
func (s *RenamerService) RecoverRename(directory string) domain.RenameResult {
	d, err := s.openLocked(directory)
	if err != nil {
		return renameFailure(err)
	}
	defer d.Close()
	data, err := d.ReadJournal()
	if os.IsNotExist(err) {
		return domain.RenameResult{Success: true, Message: "No interrupted rename"}
	}
	if err != nil {
		return renameFailure(err)
	}
	j, err := decodeJournal(d, data)
	if err != nil {
		return renameFailure(err)
	}
	if err := restoreJournal(d, j); err != nil {
		result := renameFailure(err)
		result.RollbackErrors = []string{err.Error()}
		return result
	}
	return domain.RenameResult{Success: true, RolledBack: true, Message: "Interrupted rename restored"}
}

func restoreJournal(d port.RenameDirectory, j renameJournal) error {
	// Enumerate actual spellings so case-only aliases are not counted twice.
	entries, err := d.ReadDir()
	if err != nil {
		return err
	}
	expected := map[string]int{}
	for i, e := range j.Entries {
		expected[e.SourceID] = i
	}
	locations := make([]string, len(j.Entries))
	for _, e := range entries {
		if e.Name() == ".dub-rename-journal" {
			continue
		}
		id, err := d.EntryIdentity(e.Name())
		if err != nil {
			return err
		}
		if i, ok := expected[id]; ok {
			if locations[i] != "" {
				return fmt.Errorf("ambiguous identity in journal paths")
			}
			locations[i] = e.Name()
		}
	}
	for i, e := range j.Entries {
		if locations[i] == "" {
			return fmt.Errorf("journal source %q missing or replaced", e.Original)
		}
		for _, name := range []string{e.Original, e.Temp} {
			id, err := d.EntryIdentity(name)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			if err == nil {
				if _, owned := expected[id]; !owned {
					return fmt.Errorf("outside entry blocks recovery at %q", name)
				}
			}
		}
		// An identity at another entry's temp/name is ambiguous, not permission
		// to move it from arbitrary locations.
		atExpectedPath := false
		for _, name := range []string{e.Original, e.Target, e.Temp} {
			id, err := d.EntryIdentity(name)
			if err != nil && !os.IsNotExist(err) {
				return err
			}
			atExpectedPath = atExpectedPath || err == nil && id == e.SourceID
		}
		if !atExpectedPath {
			return fmt.Errorf("journal identity at unexpected location")
		}
	}
	// Restage all moved entries before restoring originals, supporting swaps
	// and recovery interrupted during an earlier recovery attempt.
	for i, e := range j.Entries {
		if locations[i] != e.Temp {
			if err := checkedMove(d, locations[i], e.Temp, e.SourceID); err != nil {
				return err
			}
		}
	}
	for _, e := range j.Entries {
		if err := checkedMove(d, e.Temp, e.Original, e.SourceID); err != nil {
			return err
		}
	}
	return d.RemoveJournal()
}
