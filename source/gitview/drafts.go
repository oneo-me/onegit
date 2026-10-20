package gitview

import (
	"maps"
	"time"
)

// saveDraft remembers the commit message typed for the shown worktree, or
// forgets it when empty. It saves the view's state only when the draft
// changed.
func (v *View) saveDraft() {
	if v.path == "" {
		return
	}
	message := v.messages[v.path]
	drafts := maps.Clone(v.state.Drafts)
	if drafts == nil {
		drafts = map[string]string{}
	}
	if message == "" {
		delete(drafts, v.path)
	} else {
		drafts[v.path] = message
	}
	if maps.Equal(drafts, v.state.Drafts) {
		return
	}
	v.state.Drafts = drafts
	v.saveState()
}

// scheduleDraftSave saves the shown worktree's draft after a pause, so
// typing does not write the state file on every key.
func (v *View) scheduleDraftSave() {
	v.draftGeneration++
	generation := v.draftGeneration
	time.AfterFunc(400*time.Millisecond, func() {
		v.dispatch(func() {
			if generation == v.draftGeneration && !v.closed {
				v.saveDraft()
			}
		})
	})
}
