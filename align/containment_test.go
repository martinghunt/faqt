package align

import (
	"context"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"

	"github.com/martinghunt/faqt/seq"
)

func containmentFixture(q, t int, reverse bool, ops []Op) Match {
	m := Match{A: seq.Interval{Start: q}, Reverse: reverse, Ops: ops}
	startT := t
	for _, op := range ops {
		m.Columns += op.Length
		switch op.Code {
		case '=':
			m.Matches += op.Length
			m.Score += 2 * op.Length
		case 'X':
			m.Score -= 4 * op.Length
		default:
			m.Score -= gapCost(op.Length, DefaultOptions())
		}
		if op.Code != 'D' {
			q += op.Length
		}
		if op.Code != 'I' {
			t += op.Length
		}
	}
	m.A.End = q
	if reverse {
		m.B = seq.Interval{Start: -t, End: -startT}
	} else {
		m.B = seq.Interval{Start: startT, End: t}
	}
	return m
}
func containmentSubpath(m Match, r *rand.Rand) Match {
	begin := r.IntN(len(m.Ops))
	end := begin + 1 + r.IntN(len(m.Ops)-begin)
	q, t := m.A.Start, m.B.Start
	if m.Reverse {
		t = -m.B.End
	}
	for _, op := range m.Ops[:begin] {
		if op.Code != 'D' {
			q += op.Length
		}
		if op.Code != 'I' {
			t += op.Length
		}
	}
	ops := slices.Clone(m.Ops[begin:end])
	skip := r.IntN(ops[0].Length)
	if ops[0].Code != 'D' {
		q += skip
	}
	if ops[0].Code != 'I' {
		t += skip
	}
	ops[0].Length -= skip
	ops[len(ops)-1].Length = 1 + r.IntN(ops[len(ops)-1].Length)
	out := containmentFixture(q, t, m.Reverse, ops)
	out.AContig, out.BContig = m.AContig, m.BContig
	return out
}
func containmentBrute(matches []Match) []Match {
	matches = slices.Clone(matches)
	slices.SortFunc(matches, func(a, b Match) int {
		if a.Columns != b.Columns {
			return b.Columns - a.Columns
		}
		if a.Score != b.Score {
			return b.Score - a.Score
		}
		return compareMatch(a, b)
	})
	out := make([]Match, 0, len(matches))
	for _, m := range matches {
		found := false
		for _, big := range out {
			if pathContains(big, m) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, m)
		}
	}
	SortMatches(out)
	return out
}
func TestContainmentIndexAgainstScan(t *testing.T) {
	r := rand.New(rand.NewPCG(98, 811))
	var all []Match
	var index containmentIndex
	for i := 0; i < 3000; i++ {
		var m Match
		if i > 0 && i%2 == 0 {
			m = containmentSubpath(all[r.IntN(len(all))], r)
		} else {
			var ops []Op
			for j := 0; j < 1+r.IntN(20); j++ {
				ops = addOp(ops, "=XID"[r.IntN(4)], 1+r.IntN(30))
			}
			rev := r.IntN(2) == 0
			tpos := 1000 + r.IntN(1000)
			if rev {
				tpos = -3000 - r.IntN(1000)
			}
			m = containmentFixture(1000+r.IntN(1000), tpos, rev, ops)
			m.AContig, m.BContig = r.IntN(3), r.IntN(3)
		}
		want := false
		for _, big := range all {
			if pathContains(big, m) {
				want = true
				break
			}
		}
		if got := index.contains(m); got != want {
			t.Fatalf("query %d got %v want %v: %+v", i, got, want, m)
		}
		index.add(m)
		all = append(all, m)
	}
	want := containmentBrute(all)
	got, err := deduplicateContext(context.Background(), slices.Clone(all))
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("indexed dedup changed retained paths: got %d want %d error %v", len(got), len(want), err)
	}
}
func TestContainmentIndexAtOperationBoundaries(t *testing.T) {
	for _, rev := range []bool{false, true} {
		start := 100
		if rev {
			start = -1000
		}
		big := containmentFixture(100, start, rev, []Op{{'=', 20}, {'X', 4}, {'I', 10}, {'D', 11}, {'=', 30}})
		var index containmentIndex
		index.add(big)
		q, tp := big.A.Start, start
		for _, op := range big.Ops {
			for offset := 0; offset < op.Length; offset++ {
				sq, st := q, tp
				if op.Code != 'D' {
					sq += offset
				}
				if op.Code != 'I' {
					st += offset
				}
				small := containmentFixture(sq, st, rev, []Op{{op.Code, op.Length - offset}})
				if index.contains(small) != pathContains(big, small) {
					t.Fatalf("boundary rev=%v offset=%d op=%c", rev, offset, op.Code)
				}
			}
			if op.Code != 'D' {
				q += op.Length
			}
			if op.Code != 'I' {
				tp += op.Length
			}
		}
	}
}
