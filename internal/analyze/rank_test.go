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
	// Total=251; tiny is sub-0.1% (1/251), so it sits in the name bucket after
	// all rows that reach at least 0.1% (big/alpha/Beta/mid).
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

func TestRankBrowseChildrenSubTenthFrozenByNameNotBytes(t *testing.T) {
	// Large total so distinct tiny byte counts all stay <0.1% and must not
	// re-order by those bytes during live scanning.
	children := []BrowseChild{
		{Name: "zulu", Path: `/root/zulu`, Bytes: 50},   // larger but still <0.1% of 100_000
		{Name: "alpha", Path: `/root/alpha`, Bytes: 5},  // smaller
		{Name: "mid", Path: `/root/mid`, Bytes: 20},
		{Name: "big", Path: `/root/big`, Bytes: 99_925}, // ~99%
	}
	ranked := RankBrowseChildren(children)
	wantNames := []string{"big", "alpha", "mid", "zulu"}
	for i, want := range wantNames {
		if ranked[i].Name != want {
			t.Fatalf("rank[%d] = %q, want %q (full %#v)", i, ranked[i].Name, want, ranked)
		}
	}
	// Same sub-0.1% set after byte jitter must keep name order.
	children[0].Bytes = 1  // zulu shrinks
	children[1].Bytes = 80 // alpha grows (still <0.1% of ~100k)
	ranked2 := RankBrowseChildren(children)
	for i, want := range wantNames {
		if ranked2[i].Name != want {
			t.Fatalf("after jitter rank[%d] = %q, want %q", i, ranked2[i].Name, want)
		}
	}
}

func TestRankBrowseChildrenPositiveShareStillBytesDescending(t *testing.T) {
	// Both sides >=0.1% compete on full bytes (not frozen).
	children := []BrowseChild{
		{Name: "smaller", Path: `/root/smaller`, Bytes: 40},
		{Name: "larger", Path: `/root/larger`, Bytes: 60},
	}
	ranked := RankBrowseChildren(children)
	if ranked[0].Name != "larger" || ranked[1].Name != "smaller" {
		t.Fatalf("positive-share rows must rank by bytes: %#v", ranked)
	}
}

func TestRankBrowseChildrenTenthBeatsSubTenth(t *testing.T) {
	// One child reaches at least 0.1%; it must rise above pure sub-0.1% rows
	// even if those names sort earlier alphabetically.
	// total = 100_000+5+4 = 100_009; need >= ceil(total/1000)=101 for 0.1%.
	children := []BrowseChild{
		{Name: "aaa-sub", Path: `/root/aaa`, Bytes: 5},
		{Name: "zzz-tenth", Path: `/root/zzz`, Bytes: 101},
		{Name: "bbb-sub", Path: `/root/bbb`, Bytes: 4},
		{Name: "big", Path: `/root/big`, Bytes: 100_000},
	}
	ranked := RankBrowseChildren(children)
	wantNames := []string{"big", "zzz-tenth", "aaa-sub", "bbb-sub"}
	for i, want := range wantNames {
		if ranked[i].Name != want {
			t.Fatalf("rank[%d] = %q, want %q (full %#v)", i, ranked[i].Name, want, ranked)
		}
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
