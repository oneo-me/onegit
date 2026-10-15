package main

import (
	"context"
)

// loadState owns one asynchronous Git read. Callers keep one per dataset and
// never copy it after the first start. version guards against stale results
// after the worktree or project changed while a read was running.
type loadState[T any] struct {
	value   T
	path    string
	loaded  bool
	loading bool
	problem string
	version uint64
	cancel  context.CancelFunc
}

// reset cancels an in-flight read and clears any result for the previous path.
func (s *loadState[T]) reset() {
	if s.cancel != nil {
		s.cancel()
	}
	*s = loadState[T]{version: s.version + 1}
}

// needs reports whether a read for path should start, so callers can decide
// per frame without restarting reads that are already running.
func (s *loadState[T]) needs(path string) bool {
	if path == "" {
		return false
	}
	if s.path != path {
		return true
	}
	return !s.loaded && !s.loading
}

// start runs fetch on a background goroutine and applies the result through
// dispatch. done runs on the same main-thread turn after the result lands.
func (s *loadState[T]) start(dispatch func(func()), path string, fetch func(context.Context) (T, error), done func()) {
	if s.cancel != nil {
		s.cancel()
	}
	s.version++
	version := s.version
	if s.path != path {
		s.loaded = false
		var zero T
		s.value = zero
	}
	s.path = path
	s.loading = true
	s.problem = ""
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	go func() {
		defer cancel()
		value, err := fetch(ctx)
		dispatch(func() {
			if s.version != version {
				return
			}
			s.loading = false
			if err != nil {
				s.problem = err.Error()
			} else {
				s.value = value
				s.loaded = true
			}
			if done != nil {
				done()
			}
		})
	}()
}
