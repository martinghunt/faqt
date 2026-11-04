package align

import (
	"bytes"
	"context"
	"github.com/martinghunt/faqt/seq"
	"math"
	"slices"
)

// anchor is an exact run in forward A / oriented B coordinates. Compaction of
// collinear seeds makes chaining practical on long, highly similar assemblies.
type anchor struct{ q, t, n int }
type seedPos struct {
	pos    int
	strand uint8
}

func compareContigs(ctx context.Context, q, t contig, sketch SketchOptions, o Options) ([]Match, error) {
	var buckets [2][]anchor
	for si, s := range q.seeds {
		if si%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for hi, hit := range t.index[s.Hash] {
			if hi%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			strand := s.Strand ^ hit.strand
			pos := hit.pos
			target := t.bases
			if strand != 0 {
				pos = len(t.bases) - pos - sketch.K
				target = t.reverse
			}
			// Hashes are only seed lookups, never evidence of a base match.
			if !bytes.Equal(q.bases[s.Pos:s.Pos+sketch.K], target[pos:pos+sketch.K]) {
				continue
			}
			buckets[strand] = append(buckets[strand], anchor{s.Pos, pos, sketch.K})
			if o.MaxAnchors > 0 && len(buckets[strand]) > o.MaxAnchors {
				return nil, &LimitError{"anchors", int64(len(buckets[strand])), int64(o.MaxAnchors)}
			}
		}
	}
	var out []Match
	var index containmentIndex
	for strand, seeds := range buckets {
		target := t.bases
		if strand != 0 {
			target = t.reverse
		}
		anchors, err := compactAnchors(ctx, q.bases, target, seeds)
		if err != nil {
			return nil, err
		}
		chains, err := chainAnchors(ctx, anchors, o)
		if err != nil {
			return nil, err
		}
		// Significant exact runs are candidates in their own right. A weak join in
		// a longer chain must not hide a qualifying local placement.
		for i, a := range anchors {
			if a.n >= o.MinLength {
				chains = append(chains, []int{i})
			}
		}
		seen := make(map[string]bool)
		cache := newRefinementCache()
		for ci, path := range chains {
			if ci%64 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			// A standalone exact run already represented by a refined chain
			// is an equivalent subpath, not an additional placement.
			if len(path) == 1 {
				a := anchors[path[0]]
				candidate := Match{A: intervalForAnchor(a.q, a.n), B: intervalForAnchor(a.t, a.n), Reverse: strand != 0, Ops: []Op{{'=', a.n}}}
				if candidate.Reverse {
					candidate.B.Start, candidate.B.End = len(t.bases)-candidate.B.End, len(t.bases)-candidate.B.Start
				}
				if index.contains(candidate) {
					continue
				}
			}
			// Identical anchor paths arise as standalone runs and DP leaves.
			key := pathKey(path)
			if seen[key] {
				continue
			}
			seen[key] = true
			hits, err := refineChainCached(ctx, q.bases, target, anchors, path, o, cache)
			if err != nil {
				return nil, err
			}
			for _, m := range hits {
				m.Reverse = strand != 0
				if m.Reverse {
					m.B.Start, m.B.End = len(t.bases)-m.B.End, len(t.bases)-m.B.Start
				}
				out = append(out, m)
				index.add(m)
			}
		}
	}
	return deduplicateContext(ctx, out)
}

func compactAnchors(ctx context.Context, q, t []byte, seeds []anchor) ([]anchor, error) {
	slices.SortFunc(seeds, func(a, b anchor) int {
		if a.t-a.q != b.t-b.q {
			return (a.t - a.q) - (b.t - b.q)
		}
		return a.q - b.q
	})
	out := make([]anchor, 0, len(seeds))
	for i := 0; i < len(seeds); {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		a := seeds[i]
		i++
		// Extend an exact seed once; all seeds it contains are then skipped.
		for a.q > 0 && a.t > 0 && exact(q[a.q-1], t[a.t-1]) {
			a.q--
			a.t--
			a.n++
		}
		for a.q+a.n < len(q) && a.t+a.n < len(t) && exact(q[a.q+a.n], t[a.t+a.n]) {
			a.n++
			if a.n%65536 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
		}
		for i < len(seeds) && seeds[i].t-seeds[i].q == a.t-a.q && seeds[i].q+seeds[i].n <= a.q+a.n {
			i++
		}
		out = append(out, a)
	}
	slices.SortFunc(out, func(a, b anchor) int {
		if a.q != b.q {
			return a.q - b.q
		}
		if a.t != b.t {
			return a.t - b.t
		}
		return a.n - b.n
	})
	return slices.Compact(out), nil
}

// The transition reward follows minimap2 v2.30 lchain.c's assembly chaining
// model: new anchor coverage minus linear + logarithmic diagonal penalties.
// No fixed diagonal bins, max-skip shortcut, or top-N chain filter is used.
//
// Chains are extracted greedily from the highest-scoring leaf. Backtracking
// stops at an anchor already claimed by an earlier chain, so alternative
// branches do not repeat a shared backbone. The residual branch keeps that
// boundary anchor as context for refinement, and it qualifies if its own
// anchors plus the boundary anchor's exact-match length reach MinChainScore.
// This retains short alternative placements next to a shared prefix.
func chainAnchors(ctx context.Context, a []anchor, o Options) ([][]int, error) {
	scores := make([]int, len(a))
	parent := make([]int, len(a))
	children := make([]int, len(a))
	prefixEnd := make([]int, len(a))
	for i, v := range a {
		prefixEnd[i] = v.q + v.n
		if i > 0 {
			prefixEnd[i] = max(prefixEnd[i], prefixEnd[i-1])
		}
	}
	for i, next := range a {
		if i%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		scores[i] = next.n
		parent[i] = -1
		for j := i - 1; j >= 0; j-- {
			if (i-j)%4096 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
			}
			if next.q-prefixEnd[j] > o.MaxGap {
				break
			}
			prev := a[j]
			if next.q-(prev.q+prev.n) > o.MaxGap {
				// Earlier starts may belong to longer exact runs: cannot break by start.
				continue
			}
			dq, dt := next.q-prev.q, next.t-prev.t
			if dq <= 0 || dt <= 0 || dq < prev.n || dt < prev.n {
				continue
			}
			qgap, tgap := dq-prev.n, dt-prev.n
			if max(qgap, tgap) > o.MaxGap {
				continue
			}
			dd := abs(qgap - tgap)
			penalty := int(0.15*float64(dd) + 0.5*math.Log2(float64(dd+1)) + 0.0001*float64(min(qgap, tgap)))
			score := scores[j] + next.n - penalty
			if score > scores[i] {
				scores[i] = score
				parent[i] = j
			}
		}
		if parent[i] >= 0 {
			children[parent[i]]++
		}
	}
	var leaves []int
	for i := range a {
		if children[i] == 0 && scores[i] >= o.MinChainScore {
			leaves = append(leaves, i)
		}
	}
	slices.SortFunc(leaves, func(i, j int) int {
		if scores[i] != scores[j] {
			return scores[j] - scores[i]
		}
		return i - j
	})
	used := make([]bool, len(a))
	var paths [][]int
	for _, i := range leaves {
		if used[i] {
			continue
		}
		var path []int
		for j := i; j >= 0 && !used[j]; j = parent[j] {
			path = append(path, j)
		}
		slices.Reverse(path)
		score := a[path[0]].n
		for k := 1; k < len(path); k++ {
			score += scores[path[k]] - scores[parent[path[k]]]
		}
		boundary := parent[path[0]]
		if boundary >= 0 {
			score += a[boundary].n
		}
		if score < o.MinChainScore {
			continue
		}
		for _, j := range path {
			used[j] = true
		}
		if boundary >= 0 {
			path = append([]int{boundary}, path...)
		}
		paths = append(paths, path)
	}
	return paths, nil
}
func pathKey(path []int) string { // binary key, independent of machine word width
	b := make([]byte, 8*len(path))
	for i, n := range path {
		v := uint64(n)
		for j := 0; j < 8; j++ {
			b[8*i+j] = byte(v)
			v >>= 8
		}
	}
	return string(b)
}

func intervalForAnchor(start, n int) seq.Interval { return seq.Interval{Start: start, End: start + n} }
