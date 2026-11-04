package align

import (
	"context"
	"sort"

	"github.com/martinghunt/faqt/seq"
)

type unionSpan struct {
	seq.Interval
	path int
}

type unionSegment struct {
	lo, hi, diag int
	covered      bool
}

// trimUnionCovered reduces a path nested inside a higher-scoring, at least as
// identical path to the parts the other retained paths do not already report.
// Covered paired stretches of at least MinLength are cut out; each remaining
// piece is kept only if it still pairs MinLength new bases. A placement routed
// onto a neighbouring repeat copy at one end therefore becomes its own short
// copy-to-copy path, and disappears if that pairing is already reported. Paths
// are considered from the highest score, so a long path is never replaced by
// shorter pieces of itself, and every removed pairing stays reported.
// matches must be in descending score order.
func trimUnionCovered(ctx context.Context, matches []Match, o Options) ([]Match, error) {
	n := len(matches)
	paths := make([]pairedPath, n)
	lines := make(map[runKey][]unionSpan)
	addLines := func(id int) {
		m := paths[id].match
		group := containmentGroup{m.AContig, m.BContig, m.Reverse}
		for diag, spans := range paths[id].lines {
			key := runKey{group, diag}
			for _, s := range spans {
				lines[key] = append(lines[key], unionSpan{s, id})
			}
			if id >= n {
				l := lines[key]
				sort.SliceStable(l, func(i, j int) bool { return l[i].Start < l[j].Start })
			}
		}
	}
	var index redundancyIndex
	for i, m := range matches {
		paths[i] = pairedMatch(m)
		index.add(paths[i])
		addLines(i)
	}
	for _, spans := range lines {
		sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })
	}
	removed := make([]bool, n)
	var cover []seq.Interval
	for i := 0; i < n; i++ {
		if i%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		p := paths[i]
		m := p.match
		nested := index.find(p, func(id int) bool {
			h := matches[id]
			return id < i && !removed[id] && h.Score >= m.Score && h.Identity() >= m.Identity() &&
				h.B.Start <= m.B.Start && h.B.End >= m.B.End && (h.A != m.A || h.B != m.B)
		})
		if !nested {
			continue
		}
		group := containmentGroup{m.AContig, m.BContig, m.Reverse}
		runs, diags := pathRuns(m)
		var segs []unionSegment
		for k, r := range runs {
			cover = cover[:0]
			for _, u := range lines[runKey{group, diags[k]}] {
				if u.Start >= r.hi {
					break
				}
				if u.path != i && !removed[u.path] && u.End > r.lo {
					cover = append(cover, seq.Interval{Start: max(r.lo, u.Start), End: min(r.hi, u.End)})
				}
			}
			at := r.lo
			for _, c := range cover {
				if c.Start > at {
					segs = append(segs, unionSegment{at, c.Start, diags[k], false})
					at = c.Start
				}
				if c.End > at {
					segs = append(segs, unionSegment{at, c.End, diags[k], true})
					at = c.End
				}
			}
			if r.hi > at {
				segs = append(segs, unionSegment{at, r.hi, diags[k], false})
			}
		}
		// Group segments into pieces separated by long covered stretches.
		type piece struct{ first, last, novel int }
		var pieces []piece
		cur := piece{-1, -1, 0}
		for k, s := range segs {
			if s.covered {
				if s.hi-s.lo >= o.MinLength && cur.first >= 0 {
					pieces = append(pieces, cur)
					cur = piece{-1, -1, 0}
				}
				continue
			}
			if cur.first < 0 {
				cur.first = k
			}
			cur.last = k
			cur.novel += s.hi - s.lo
		}
		if cur.first >= 0 {
			pieces = append(pieces, cur)
		}
		if len(pieces) == 1 && pieces[0].novel >= o.MinLength && segs[pieces[0].first].lo == m.A.Start && segs[pieces[0].last].hi == m.A.End {
			continue
		}
		removed[i] = true
		for _, pc := range pieces {
			if pc.novel < o.MinLength {
				continue
			}
			a, b := segs[pc.first], segs[pc.last]
			head, _, ok := cutOps(m, b.hi, b.diag)
			if !ok {
				continue
			}
			trimmed := m
			trimmed.Ops = head
			_, tail, ok := cutOps(trimmed, a.lo, a.diag)
			if !ok {
				continue
			}
			x := buildPath(m, a.lo, a.lo+a.diag, tail, o)
			if x.Score < o.MinScore || x.Identity() < o.MinIdentity {
				continue
			}
			matches = append(matches, x)
			paths = append(paths, pairedMatch(x))
			removed = append(removed, false)
			addLines(len(paths) - 1)
		}
	}
	out := matches[:0]
	for i, m := range matches {
		if !removed[i] {
			out = append(out, m)
		}
	}
	return out, nil
}
