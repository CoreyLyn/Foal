package analyze

import (
	"path/filepath"
	"sync/atomic"
)

// BrowseFocus reports the path the user currently wants preferred for the next
// free directory-measurement slot. Implementations must be safe for concurrent
// use from the UI and the browse scheduler.
//
// Focus never cancels or preempts an already active measurement; it only
// influences which queued directory is assigned when a worker becomes free.
type BrowseFocus interface {
	// FocusedPath returns the canonical child path to promote, or "" for pure
	// name-order scheduling among remaining work.
	FocusedPath() string
}

// AtomicBrowseFocus is a concurrent focus holder for TUI and tests.
type AtomicBrowseFocus struct {
	path atomic.Value // string
}

// NewAtomicBrowseFocus returns an empty focus holder.
func NewAtomicBrowseFocus() *AtomicBrowseFocus {
	return &AtomicBrowseFocus{}
}

// Set stores the preferred queued path (may be "" to clear promotion).
// Non-empty paths are filepath.Clean'd so promotion matches seed paths.
func (f *AtomicBrowseFocus) Set(path string) {
	if f == nil {
		return
	}
	if path != "" {
		path = filepath.Clean(path)
	}
	f.path.Store(path)
}

// FocusedPath implements BrowseFocus.
func (f *AtomicBrowseFocus) FocusedPath() string {
	if f == nil {
		return ""
	}
	v := f.path.Load()
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}
