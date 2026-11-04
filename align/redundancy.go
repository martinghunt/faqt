package align

import (
	"context"
	"slices"
	"sort"

	"github.com/martinghunt/faqt/seq"
)

type pairedPath struct {
	match  Match
	lines  map[int][]seq.Interval
	paired int
}

func pairedMatch(m Match) pairedPath {
	p := pairedPath{match: m, lines: make(map[int][]seq.Interval)}
	q, t := m.A.Start, m.B.Start
	if m.Reverse {
		t = -m.B.End
	}
	for _, op := range m.Ops {
		switch op.Code {
		case '=', 'X':
			diag := t - q
			spans := p.lines[diag]
			if n := len(spans); n > 0 && spans[n-1].End == q {
				spans[n-1].End += op.Length
			} else {
				spans = append(spans, seq.Interval{Start: q, End: q + op.Length})
			}
			p.lines[diag] = spans
			p.paired += op.Length
			q += op.Length
			t += op.Length
		case 'I':
			q += op.Length
		case 'D':
			t += op.Length
		}
	}
	return p
}

// Both intervals must be contained by one equal-or-higher-scoring path. At
// least 95% of paired coordinates must agree, and an independently reportable
// new paired stretch prevents suppression even in a very long alignment,
// unless both ends of the smaller path lie on the larger one: novel stretches
// are then internal detours (e.g. alternative tandem-copy pairings) of the
// same placement.
func nearContained(big, small pairedPath, minNovel int) bool {
	a, b := big.match, small.match
	if a.Score < b.Score || a.AContig != b.AContig || a.BContig != b.BContig || a.Reverse != b.Reverse ||
		a.A.Start > b.A.Start || a.A.End < b.A.End || a.B.Start > b.B.Start || a.B.End < b.B.End || small.paired == 0 {
		return false
	}
	shared, novel := 0, false
	for diag, spans := range small.lines {
		host := big.lines[diag]
		j := 0
		for _, s := range spans {
			at := s.Start
			for j < len(host) && host[j].End <= at {
				j++
			}
			for k := j; k < len(host) && host[k].Start < s.End; k++ {
				h := host[k]
				if min(s.End, h.Start)-at >= minNovel {
					novel = true
				}
				lo, hi := max(at, h.Start), min(s.End, h.End)
				if hi > lo {
					shared += hi - lo
					at = hi
				}
			}
			if s.End-at >= minNovel {
				novel = true
			}
		}
	}
	if novel {
		q, t := pathStart(b)
		qe, te := pathEnd(b)
		if !onLine(big.lines[t-q], q) || !onLine(big.lines[te-qe], qe-1) {
			return false
		}
	}
	return int64(shared)*100 >= int64(small.paired)*95
}

func onLine(spans []seq.Interval, q int) bool {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].End > q })
	return i < len(spans) && spans[i].Start <= q
}

type redundancyIndex struct {
	// Reuse the interval treap implementation with one bounding interval per
	// path. Each candidate still requires the two-axis and paired-path checks.
	tree  containmentIndex
	roots map[containmentGroup]int
	paths []pairedPath
}

func (x *redundancyIndex) add(p pairedPath) {
	if x.roots == nil {
		x.roots = make(map[containmentGroup]int)
		x.tree.nodes = append(x.tree.nodes, containmentNode{maxHi: -int(^uint(0)>>1) - 1})
	}
	m := p.match
	id, node := len(x.paths), len(x.tree.nodes)
	x.paths = append(x.paths, p)
	z := uint64(node) + 0x9e3779b97f4a7c15
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	z ^= z >> 31
	x.tree.nodes = append(x.tree.nodes, containmentNode{lo: m.A.Start, hi: m.A.End, maxHi: m.A.End, match: id, priority: z})
	key := containmentGroup{m.AContig, m.BContig, m.Reverse}
	x.roots[key] = x.tree.insert(x.roots[key], node)
}

func (x *redundancyIndex) contains(p pairedPath, minNovel int) bool {
	return x.find(p, func(id int) bool { return nearContained(x.paths[id], p, minNovel) })
}

// find reports whether accept holds for some indexed path whose A interval
// contains p's A interval.
func (x *redundancyIndex) find(p pairedPath, accept func(id int) bool) bool {
	m := p.match
	return x.query(x.roots[containmentGroup{m.AContig, m.BContig, m.Reverse}], p, accept)
}

func (x *redundancyIndex) query(root int, p pairedPath, accept func(id int) bool) bool {
	if root == 0 || x.tree.nodes[root].maxHi < p.match.A.End {
		return false
	}
	n := x.tree.nodes[root]
	if n.lo > p.match.A.Start {
		return x.query(n.left, p, accept)
	}
	if n.hi >= p.match.A.End && accept(n.match) {
		return true
	}
	return x.query(n.left, p, accept) || x.query(n.right, p, accept)
}

func cleanRedundantMatches(ctx context.Context, a, b *Genome, matches []Match, o Options) ([]Match, error) {
	var err error
	matches, err = normalizeMatches(ctx, a, b, matches, o)
	if err != nil {
		return nil, err
	}
	matches, err = deduplicateContext(ctx, matches)
	if err != nil {
		return nil, err
	}
	matches, err = spliceEndOverlaps(ctx, matches, o)
	if err != nil {
		return nil, err
	}
	matches, err = deduplicateContext(ctx, matches)
	if err != nil {
		return nil, err
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		if matches[i].Columns != matches[j].Columns {
			return matches[i].Columns > matches[j].Columns
		}
		return compareMatch(matches[i], matches[j]) < 0
	})
	self := a.digest == b.digest
	out := make([]Match, 0, len(matches))
	var index redundancyIndex
	for i, m := range matches {
		if i%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		p := pairedMatch(m)
		if index.contains(p, o.MinLength) {
			continue
		}
		out = append(out, m)
		index.add(p)
		if self {
			partner := m.Swap()
			if compareMatch(m, partner) != 0 {
				out = append(out, partner)
				index.add(pairedMatch(partner))
			}
		}
	}
	if !self {
		out, err = trimUnionCovered(ctx, out, o)
		if err != nil {
			return nil, err
		}
	}
	SortMatches(out)
	return slices.Clip(out), nil
}
