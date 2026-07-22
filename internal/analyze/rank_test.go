package analyze

import (
	"testing"
)

func TestRankBrowseChildrenBytesDescendingNameTieBreak(t *testing.T) {
	children := []BrowseChild{
		{Name: "mid", Path: `/root/mid`, Bytes: 50},
		{Name: "big", Path: `/root/big`, Bytes: 100},
		{Name: "alpha", Path: `/root/alpha`, Bytes: 50},
		{Name: "tiny", Path: `/root/tiny`, Bytes: 1},
		{Name: "Beta", Path: `/root/Beta`, Bytes: 50},
	}
	ranked := RankBrowseChildren(children)
	if len(ranked) != 5 {
		t.Fatalf("len = %d", len(ranked))
	}
	// Input must not be mutated.
	if children[0].Name != "mid" {
		t.Fatalf("input mutated: %#v", children)
	}
	wantNames := []string{"big", "alpha", "Beta", "mid", "tiny"}
	for i, want := range wantNames {
		if ranked[i].Name != want {
			t.Fatalf("rank[%d] = %q, want %q (full %#v)", i, ranked[i].Name, want, ranked)
		}
	}
}

func TestRankBrowseChildrenEmpty(t *testing.T) {
	if RankBrowseChildren(nil) != nil {
		t.Fatal("nil in → nil out")
	}
	if RankBrowseChildren([]BrowseChild{}) != nil {
		t.Fatal("empty in → nil out")
	}
}

func TestIndexOfBrowsePath(t *testing.T) {
	children := []BrowseChild{
		{Name: "a", Path: `C:\a`},
		{Name: "b", Path: `C:\b`},
	}
	if got := IndexOfBrowsePath(children, `C:\b`); got != 1 {
		t.Fatalf("index = %d", got)
	}
	if got := IndexOfBrowsePath(children, `C:\missing`); got != -1 {
		t.Fatalf("missing = %d", got)
	}
	if got := IndexOfBrowsePath(children, ""); got != -1 {
		t.Fatalf("empty path = %d", got)
	}
}
