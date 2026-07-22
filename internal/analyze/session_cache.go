package analyze

import (
	"path/filepath"
	"strings"
	"sync"
)

// BrowseSessionCache holds durable terminal child summaries for one Analyze TUI
// session. It is process-memory only: never persisted, never written to History,
// and never consumed by cleanup execution.
//
// Durable terminals: Complete, Partial, Skipped, and hard-limit Incomplete.
// Navigation-cancel Incomplete is not stored; it remains missing work on return.
type BrowseSessionCache struct {
	mu sync.Mutex
	// location root (cleaned) → child path (cleaned) → terminal summary
	byLocation map[string]map[string]BrowseChild
}

// NewBrowseSessionCache returns an empty session cache.
func NewBrowseSessionCache() *BrowseSessionCache {
	return &BrowseSessionCache{
		byLocation: make(map[string]map[string]BrowseChild),
	}
}

// cacheKey normalizes a path for session-cache map lookup.
func cacheKey(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return strings.ToLower(filepath.Clean(path))
	}
	return strings.ToLower(filepath.Clean(abs))
}

// IsDurableCachedChild reports whether a child summary may be retained and
// reused on return to a location without re-measurement.
func IsDurableCachedChild(child BrowseChild) bool {
	switch child.State {
	case BrowseStateComplete, BrowseStatePartial, BrowseStateSkipped:
		return true
	case BrowseStateIncomplete:
		// Hard-limit Incomplete is terminal. Nav-cancel Incomplete is not durable.
		return hasSkipReason(child.SkipAggregates, SkipReasonHardLimit)
	default:
		return false
	}
}

func hasSkipReason(aggs []SkipAggregate, reason string) bool {
	for _, a := range aggs {
		if a.Reason == reason && a.Count > 0 {
			return true
		}
	}
	return false
}

// Put stores a durable terminal child under locationRoot. Non-durable children
// (Scanning, nav-cancel Incomplete) are ignored.
func (c *BrowseSessionCache) Put(locationRoot string, child BrowseChild) {
	if c == nil || !IsDurableCachedChild(child) || child.Path == "" {
		return
	}
	loc := cacheKey(locationRoot)
	if loc == "" {
		return
	}
	key := cacheKey(child.Path)
	if key == "" {
		return
	}
	// Store a defensive copy (aggregates included).
	stored := child
	if len(child.SkipAggregates) > 0 {
		stored.SkipAggregates = append([]SkipAggregate(nil), child.SkipAggregates...)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byLocation == nil {
		c.byLocation = make(map[string]map[string]BrowseChild)
	}
	locMap := c.byLocation[loc]
	if locMap == nil {
		locMap = make(map[string]BrowseChild)
		c.byLocation[loc] = locMap
	}
	locMap[key] = stored
}

// PutAll stores every durable terminal child from the inventory for locationRoot.
func (c *BrowseSessionCache) PutAll(locationRoot string, children []BrowseChild) {
	for _, child := range children {
		c.Put(locationRoot, child)
	}
}

// KnownFor returns durable cached children for locationRoot (copy).
// Missing locations return nil.
func (c *BrowseSessionCache) KnownFor(locationRoot string) []BrowseChild {
	if c == nil {
		return nil
	}
	loc := cacheKey(locationRoot)
	c.mu.Lock()
	defer c.mu.Unlock()
	locMap := c.byLocation[loc]
	if len(locMap) == 0 {
		return nil
	}
	out := make([]BrowseChild, 0, len(locMap))
	for _, child := range locMap {
		cp := child
		if len(child.SkipAggregates) > 0 {
			cp.SkipAggregates = append([]SkipAggregate(nil), child.SkipAggregates...)
		}
		out = append(out, cp)
	}
	return out
}

// ClearLocation discards the session cache for one browse location (Refresh).
func (c *BrowseSessionCache) ClearLocation(locationRoot string) {
	if c == nil {
		return
	}
	loc := cacheKey(locationRoot)
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.byLocation, loc)
}

// Clear removes every cached location (end of Analyze session).
func (c *BrowseSessionCache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.byLocation = make(map[string]map[string]BrowseChild)
}
