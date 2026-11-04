package align

import (
	"context"
	"slices"
	"testing"
)

func TestChainExtractionKeepsEmbeddedRepeat(t *testing.T) {
	s := dna(100000, 9001)
	copy(s[49999:51000], s[999:2000])
	g := genome(t, s)
	o := DefaultOptions()
	o.EndExtension = EndExtensionAdaptive
	o.Redundancy = RedundancyNear
	ms := compare(t, g, g, o)
	for _, p := range [][2]int{{0, 0}, {999, 49999}, {49999, 999}} {
		n := 1001
		if p == [2]int{0, 0} {
			n = 100000
		}
		found := false
		for _, m := range ms {
			if !m.Reverse && pathCoversPair(m, p[0], p[1], n) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing paired placement %v length %d", p, n)
		}
	}
}

// The strongest chain is extracted first. The alternative branch stops at the
// shared anchor, which it keeps as boundary context.
func TestChainExtractionSharedFork(t *testing.T) {
	a := []anchor{{0, 0, 1000}, {1100, 1110, 400}, {1150, 1150, 2000}}
	paths, err := chainAnchors(context.Background(), a, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || !slices.Equal(paths[0], []int{0, 2}) || !slices.Equal(paths[1], []int{0, 1}) {
		t.Fatalf("wrong chains: %v", paths)
	}
}
