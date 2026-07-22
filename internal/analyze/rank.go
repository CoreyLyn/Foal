package analyze

import (
	"sort"
	"strings"
)

// RankBrowseChildren returns a new slice of children ordered by latest observed
// logical bytes descending, with case-insensitive name ascending as the
// tie-break. Input is not modified. Selection identity is path-based; ranking
// only affects display order.
func RankBrowseChildren(children []BrowseChild) []BrowseChild {
	if len(children) == 0 {
		return nil
	}
	out := append([]BrowseChild(nil), children...)
	sort.SliceStable(out, func(i, j int) bool {
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
