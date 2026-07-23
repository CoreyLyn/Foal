package analyze

import (
	"sort"
	"strings"
)

// RankBrowseChildren returns a new slice of children ordered by latest observed
// logical bytes descending, with case-insensitive name ascending as the
// tie-break. Input is not modified. Selection identity is path-based; ranking
// only affects display order.
//
// Sub-0.1% freeze: when both sides are below 0.1% of the current observed
// location total (same bucket presented as "<0.1%"), they do not compete on
// raw bytes. That freezes tiny rows relative to each other during live re-rank
// so byte jitter does not reshuffle the list. Name ascending still orders that
// bucket (same as the equal-bytes tie-break). Once either side reaches at
// least 0.1%, full bytes-descending ranking applies again.
func RankBrowseChildren(children []BrowseChild) []BrowseChild {
	if len(children) == 0 {
		return nil
	}
	total := ObservedLocationBytes(children)
	out := append([]BrowseChild(nil), children...)
	sort.SliceStable(out, func(i, j int) bool {
		// total<=0 → every share is sub-0.1% → name order only.
		iSub := shareTenths(out[i].Bytes, total) < 1
		jSub := shareTenths(out[j].Bytes, total) < 1
		if iSub && jSub {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		}
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// IndexOfBrowsePath returns the index of path in children, or -1 if absent.
// Path comparison is exact string equality on the canonical child Path field.
func IndexOfBrowsePath(children []BrowseChild, path string) int {
	if path == "" {
		return -1
	}
	for i := range children {
		if children[i].Path == path {
			return i
		}
	}
	return -1
}
