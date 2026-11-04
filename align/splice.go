package align

import (
	"context"
	"slices"
	"sort"
)

// pathRun is one maximal paired run (= and X operations) on a fixed diagonal,
// in the strand-oriented frame used by pairedMatch: q follows A forwards and t
// follows B forwards, or negated B coordinates on the reverse strand.
type pathRun struct {
	lo, hi, match int
}
type runKey struct {
	group containmentGroup
	diag  int
}

func pathStart(m Match) (int, int) {
	if m.Reverse {
		return m.A.Start, -m.B.End
	}
	return m.A.Start, m.B.Start
}

func pathEnd(m Match) (int, int) {
	if m.Reverse {
		return m.A.End, -m.B.Start
	}
	return m.A.End, m.B.End
}

func pathRuns(m Match) (runs []pathRun, diags []int) {
	q, t := pathStart(m)
	for i := 0; i < len(m.Ops); i++ {
		switch m.Ops[i].Code {
		case '=', 'X':
			lo, diag := q, t-q
			for ; i < len(m.Ops) && (m.Ops[i].Code == '=' || m.Ops[i].Code == 'X'); i++ {
				q += m.Ops[i].Length
				t += m.Ops[i].Length
			}
			i--
			runs = append(runs, pathRun{lo, q, -1})
			diags = append(diags, diag)
		case 'I':
			q += m.Ops[i].Length
		case 'D':
			t += m.Ops[i].Length
		}
	}
	return runs, diags
}

// cutOps splits a path at A coordinate q, which must lie on a paired run of
// diagonal diag or at one of its ends. Both halves meet at (q, q+diag).
func cutOps(m Match, at, diag int) (before, after []Op, ok bool) {
	q, t := pathStart(m)
	for i, op := range m.Ops {
		if q == at && t == at+diag {
			return slices.Clone(m.Ops[:i]), slices.Clone(m.Ops[i:]), true
		}
		if (op.Code == '=' || op.Code == 'X') && q < at && at < q+op.Length && t-q == diag {
			n := at - q
			before = append(slices.Clone(m.Ops[:i]), Op{op.Code, n})
			after = append([]Op{{op.Code, op.Length - n}}, m.Ops[i+1:]...)
			return before, after, true
		}
		if op.Code != 'D' {
			q += op.Length
		}
		if op.Code != 'I' {
			t += op.Length
		}
	}
	if q == at && t == at+diag {
		return slices.Clone(m.Ops), nil, true
	}
	return nil, nil, false
}

// joinPath builds a path from first's operations up to the shared point and
// second's operations after it, rescoring the result under o.
func joinPath(first, second Match, at, diag int, o Options) (Match, bool) {
	head, _, ok := cutOps(first, at, diag)
	if !ok {
		return Match{}, false
	}
	_, tail, ok := cutOps(second, at, diag)
	if !ok {
		return Match{}, false
	}
	q, t := pathStart(first)
	return buildPath(first, q, t, append(head, tail...), o), true
}

// buildPath makes a scored path with template's contigs and strand, starting
// at (q, t) in the strand-oriented frame.
func buildPath(template Match, q, t int, ops []Op, o Options) Match {
	m := Match{AContig: template.AContig, BContig: template.BContig, Reverse: template.Reverse}
	for _, op := range ops {
		m.Ops = addOp(m.Ops, op.Code, op.Length)
	}
	m.A.Start = q
	start := t
	for _, op := range m.Ops {
		m.Columns += op.Length
		switch op.Code {
		case '=':
			m.Matches += op.Length
			m.Score += op.Length * o.Match
		case 'X':
			m.Score -= op.Length * o.Mismatch
		default:
			m.Score -= gapCost(op.Length, o)
		}
		if op.Code != 'D' {
			q += op.Length
		}
		if op.Code != 'I' {
			t += op.Length
		}
	}
	m.A.End = q
	if m.Reverse {
		m.B.Start, m.B.End = -t, -start
	} else {
		m.B.Start, m.B.End = start, t
	}
	return m
}

func pairedBefore(m Match, at int) int {
	q, n := m.A.Start, 0
	for _, op := range m.Ops {
		if q >= at {
			break
		}
		if op.Code == '=' || op.Code == 'X' {
			n += min(op.Length, at-q)
		}
		if op.Code != 'D' {
			q += op.Length
		}
	}
	return n
}

func pairedAfter(m Match, at int) int {
	q, n := m.A.Start, 0
	for _, op := range m.Ops {
		if op.Code == '=' || op.Code == 'X' {
			n += max(0, q+op.Length-max(q, at))
		}
		if op.Code != 'D' {
			q += op.Length
		}
	}
	return n
}

// spliceEndOverlaps joins a lower-scoring path onto the end of a higher-scoring
// path when both follow the same diagonal there and the lower path continues
// beyond it. This repairs one placement split into a long path plus a short
// overlapping path carrying its true end. A join is accepted only when it
// improves the higher path's score, discards fewer than MinLength of its paired
// bases, and leaves the absorbed path near-contained in the result.
func spliceEndOverlaps(ctx context.Context, matches []Match, o Options) ([]Match, error) {
	order := make([]int, len(matches))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := matches[order[i]], matches[order[j]]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Columns != b.Columns {
			return a.Columns > b.Columns
		}
		return compareMatch(a, b) < 0
	})
	rank := make([]int, len(matches))
	index := make(map[runKey][]pathRun)
	for r, i := range order {
		rank[i] = r
		m := matches[i]
		group := containmentGroup{m.AContig, m.BContig, m.Reverse}
		runs, diags := pathRuns(m)
		for k, run := range runs {
			run.match = i
			key := runKey{group, diags[k]}
			index[key] = append(index[key], run)
		}
	}
	consumed := make([]bool, len(matches))
	for r, i := range order {
		if r%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		if consumed[i] {
			continue
		}
		p := matches[i]
		group := containmentGroup{p.AContig, p.BContig, p.Reverse}
		for changed := true; changed; {
			changed = false
			runs, diags := pathRuns(p)
			try := func(k int, left bool) bool {
				run, diag := runs[k], diags[k]
				for _, s := range index[runKey{group, diag}] {
					if consumed[s.match] || rank[s.match] <= r || s.hi <= run.lo || s.lo >= run.hi {
						continue
					}
					small := matches[s.match]
					var m Match
					var ok bool
					if left {
						sq, st := pathStart(small)
						pq, pt := pathStart(p)
						at := max(s.lo, run.lo)
						if sq >= pq || st >= pt || pairedBefore(p, at) >= o.MinLength {
							continue
						}
						m, ok = joinPath(small, p, at, diag, o)
					} else {
						sq, st := pathEnd(small)
						pq, pt := pathEnd(p)
						at := min(s.hi, run.hi)
						if sq <= pq || st <= pt || pairedAfter(p, at) >= o.MinLength {
							continue
						}
						m, ok = joinPath(p, small, at, diag, o)
					}
					if !ok || m.Score <= p.Score || m.Score < small.Score {
						continue
					}
					if !nearContained(pairedMatch(m), pairedMatch(small), o.MinLength) {
						continue
					}
					p = m
					consumed[s.match] = true
					return true
				}
				return false
			}
			// Paired bases of p before (or after) the junction are discarded,
			// so only consider junctions within its first and last MinLength.
			for k := 0; k < len(runs) && pairedBefore(p, runs[k].lo) < o.MinLength && !changed; k++ {
				changed = try(k, true)
			}
			for k := len(runs) - 1; k >= 0 && pairedAfter(p, runs[k].hi) < o.MinLength && !changed; k-- {
				changed = try(k, false)
			}
		}
		matches[i] = p
	}
	out := matches[:0]
	for i, m := range matches {
		if !consumed[i] {
			out = append(out, m)
		}
	}
	return out, nil
}
