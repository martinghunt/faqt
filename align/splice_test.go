package align

import (
	"context"
	"testing"
)

// One placement split into a long path and a short path that carries its
// true start: the short path shares the long path's first run and continues
// 166 bp further on the same diagonal.
func TestSpliceEndOverlapJoinsSplitPlacement(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, startSide := range []bool{true, false} {
			const n = 6000
			q := dna(n, 77)
			target := append([]byte(nil), q...)
			if reverse {
				target = reverseComplement(target)
			}
			a, b := genome(t, q), genome(t, target)
			o := DefaultOptions()
			t0 := 0
			if reverse {
				t0 = -n
			}
			var big, small Match
			if startSide {
				big = containmentFixture(166, t0+166, reverse, []Op{{'=', n - 166}})
				small = containmentFixture(0, t0, reverse, []Op{{'=', 2927}})
			} else {
				big = containmentFixture(0, t0, reverse, []Op{{'=', n - 166}})
				small = containmentFixture(n-2927, t0+n-2927, reverse, []Op{{'=', 2927}})
			}
			got, err := cleanRedundantMatches(context.Background(), a, b, []Match{small, big}, o)
			if err != nil {
				t.Fatal(err)
			}
			want := containmentFixture(0, t0, reverse, []Op{{'=', n}})
			if len(got) != 1 || got[0].A != want.A || got[0].B != want.B || got[0].CIGAR() != want.CIGAR() || got[0].Score != want.Score {
				t.Fatalf("reverse=%v start=%v: got %+v, want one %+v", reverse, startSide, got, want)
			}
			replay(t, a, b, got[0], o)
		}
	}
}

// A lower-scoring overhang is not joined: the result would score less than
// the long path alone.
func TestSpliceEndOverlapRejectsWorseExtension(t *testing.T) {
	const n = 6000
	q := dna(n, 78)
	target := append([]byte(nil), q...)
	for i := 0; i < 100; i++ {
		target[i] = map[byte]byte{'A': 'C', 'C': 'G', 'G': 'T', 'T': 'A'}[target[i]]
	}
	a, b := genome(t, q), genome(t, target)
	big := containmentFixture(100, 100, false, []Op{{'=', n - 100}})
	small := containmentFixture(0, 0, false, []Op{{'X', 100}, {'=', 1400}})
	got, err := cleanRedundantMatches(context.Background(), a, b, []Match{small, big}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("worse extension was joined: %+v", got)
	}
}

// Overlapping paths on different diagonals are distinct placements.
func TestSpliceEndOverlapKeepsOtherDiagonal(t *testing.T) {
	const n = 6000
	q := dna(n, 79)
	target := append(append([]byte(nil), q...), q[:1500]...)
	a, b := genome(t, q), genome(t, target)
	big := containmentFixture(0, 0, false, []Op{{'=', n}})
	small := containmentFixture(0, n, false, []Op{{'=', 1500}})
	got, err := cleanRedundantMatches(context.Background(), a, b, []Match{small, big}, DefaultOptions())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("other-diagonal placement was removed: %+v", got)
	}
}
