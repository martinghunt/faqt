//go:build goexperiment.simd && arm64

package align

import (
	"context"
	"simd/archsimd"
	"slices"
)

// Optional NEON kernel, enabled with GOEXPERIMENT=simd. Cells on an
// anti-diagonal have no dependencies on each other. Full row maxima are retained so that a subsequent
// row-order pass reproduces the reference Z-drop and endpoint tie-breaking.
// This computes extra cells beyond a Z-drop; the benchmark includes that cost.
func alignDPWideBoundary(ctx context.Context, q, t []byte, band int, extend bool, o Options, boundary *dpBoundary) (dpResult, error) {
	boundary.reset()
	m, n := len(q), len(t)
	if m == 0 || n == 0 || m > 4096 || n > 4096 || max(o.Match, o.Mismatch, o.GapOpen, o.LongGapOpen, o.GapExtend, o.LongGapExtend) > (1<<25)/(m+n) {
		return alignDPScalarBoundary(ctx, q, t, band, extend, o, boundary)
	}
	// Preserve scalar resource/cancellation behavior on restricted calls.
	if o.MaxCells > 0 && int64(m+1)*int64(min(n+1, 2*band+1)) > o.MaxCells {
		return alignDPScalarBoundary(ctx, q, t, band, extend, o, boundary)
	}
	type diag struct {
		s           [5][]int32
		best, state []int32
	}
	ds := [3]diag{}
	for k := range ds {
		for z := range ds[k].s {
			ds[k].s[z] = make([]int32, m+3)
		}
		ds[k].best = make([]int32, m+3)
		ds[k].state = make([]int32, m+3)
		for z := range ds[k].s {
			for j := range ds[k].s[z] {
				ds[k].s[z][j] = minusInfinity
			}
		}
		for j := range ds[k].best {
			ds[k].best[j] = minusInfinity
		}
	}
	qa, tr := make([]int32, m), make([]int32, n)
	for i, c := range q {
		qa[i] = int32(c)
	}
	for j, c := range t {
		tr[n-j-1] = int32(c)
	}
	starts, offset := make([]int, m+1), make([]int, m+1)
	cells := 0
	for i := 0; i <= m; i++ {
		starts[i] = max(0, i-band)
		offset[i] = cells
		cells += max(0, min(n, i+band)-starts[i]+1)
	}
	trace := make([]uint16, cells)
	rowBest, rowJ, rowState := make([]int, m+1), make([]int, m+1), make([]int, m+1)
	for i := range rowBest {
		rowBest[i] = minusInfinity
	}
	var rowEnd []int
	if boundary != nil {
		rowEnd = make([]int, m+1)
		for i := range rowEnd {
			rowEnd[i] = minusInfinity
		}
	}
	endScore, endState := minusInfinity, 0
	bcast := archsimd.BroadcastInt32x4
	zero, match, mis, amb := bcast(0), bcast(int32(o.Match)), bcast(int32(-o.Mismatch)), bcast('N')
	load := archsimd.LoadInt32x4
	for d := 0; d <= m+n; d++ {
		if d%64 == 0 {
			if err := ctx.Err(); err != nil {
				return dpResult{}, err
			}
		}
		cur, prev, prior := &ds[d%3], &ds[(d+2)%3], &ds[(d+1)%3]
		lo, hi := max(0, d-n, (d-band+1)/2), min(m, d, (d+band)/2)
		if lo > hi {
			break
		}
		for _, k := range []int{lo, hi + 2} {
			for z := range cur.s {
				cur.s[z][k] = minusInfinity
			}
			cur.best[k] = minusInfinity
			cur.state[k] = 0
		}
		record := func(i, j, idx int, packed int32) {
			v, s := int(cur.best[idx]), int(cur.state[idx])
			if boundary != nil && j == n {
				rowEnd[i] = v
			}
			trace[offset[i]+j-starts[i]] = uint16(packed)
			if v > rowBest[i] || (v == rowBest[i] && j < rowJ[i]) {
				rowBest[i], rowJ[i], rowState[i] = v, j, s
			}
			if i == m && j == n {
				endScore, endState = v, s
			}
		}
		for i := lo; i <= hi; {
			j, idx := d-i, i+1
			if i > 0 && i+3 <= hi && j >= 4 {
				qv, tv := load(qa[i-1:]), load(tr[n-d+i:])
				pair := match.IfElse(qv.Equal(tv), mis).IfElse(qv.NotEqual(amb), mis)
				v := load(prior.best[idx-1:]).Add(pair)
				packed := load(prior.state[idx-1:])
				best, state := v, zero
				v.Store(cur.s[0][idx:])
				{
					opened := load(prev.best[idx-1:]).Sub(bcast(int32(o.GapOpen + o.GapExtend)))
					cont := load(prev.s[1][idx-1:]).Sub(bcast(int32(o.GapExtend)))
					mask := cont.GreaterEqual(opened)
					v = cont.IfElse(mask, opened)
					from := bcast(1).IfElse(mask, load(prev.state[idx-1:]))
					packed = packed.Or(from.ShiftAllLeft(3))
					v.Store(cur.s[1][idx:])
					better := v.Greater(best)
					state = bcast(1).IfElse(better, state)
					best = v.Max(best)
				}
				{
					opened := load(prev.best[idx:]).Sub(bcast(int32(o.GapOpen + o.GapExtend)))
					cont := load(prev.s[2][idx:]).Sub(bcast(int32(o.GapExtend)))
					mask := cont.GreaterEqual(opened)
					v = cont.IfElse(mask, opened)
					from := bcast(2).IfElse(mask, load(prev.state[idx:]))
					packed = packed.Or(from.ShiftAllLeft(6))
					v.Store(cur.s[2][idx:])
					better := v.Greater(best)
					state = bcast(2).IfElse(better, state)
					best = v.Max(best)
				}
				{
					opened := load(prev.best[idx-1:]).Sub(bcast(int32(o.LongGapOpen + o.LongGapExtend)))
					cont := load(prev.s[3][idx-1:]).Sub(bcast(int32(o.LongGapExtend)))
					mask := cont.GreaterEqual(opened)
					v = cont.IfElse(mask, opened)
					from := bcast(3).IfElse(mask, load(prev.state[idx-1:]))
					packed = packed.Or(from.ShiftAllLeft(9))
					v.Store(cur.s[3][idx:])
					better := v.Greater(best)
					state = bcast(3).IfElse(better, state)
					best = v.Max(best)
				}
				{
					opened := load(prev.best[idx:]).Sub(bcast(int32(o.LongGapOpen + o.LongGapExtend)))
					cont := load(prev.s[4][idx:]).Sub(bcast(int32(o.LongGapExtend)))
					mask := cont.GreaterEqual(opened)
					v = cont.IfElse(mask, opened)
					from := bcast(4).IfElse(mask, load(prev.state[idx:]))
					packed = packed.Or(from.ShiftAllLeft(12))
					v.Store(cur.s[4][idx:])
					better := v.Greater(best)
					state = bcast(4).IfElse(better, state)
					best = v.Max(best)
				}
				best.Store(cur.best[idx:])
				state.Store(cur.state[idx:])
				var packs [4]int32
				packed.StoreArray(&packs)
				for k := 0; k < 4; k++ {
					record(i+k, j-k, idx+k, packs[k])
				}
				i += 4
			} else {
				for z := range cur.s {
					cur.s[z][idx] = minusInfinity
				}
				packed := int32(0)
				if i == 0 && j == 0 {
					cur.s[0][idx] = 0
				} else {
					if i > 0 && j > 0 {
						pair := -o.Mismatch
						if exact(q[i-1], t[j-1]) {
							pair = o.Match
						}
						cur.s[0][idx] = prior.best[idx-1] + int32(pair)
						packed = prior.state[idx-1]
					}
					for z := 1; z < 5; z++ {
						k := idx - 1
						valid := i > 0
						if z == 2 || z == 4 {
							k = idx
							valid = j > 0
						}
						op, ex := o.GapOpen, o.GapExtend
						if z >= 3 {
							op, ex = o.LongGapOpen, o.LongGapExtend
						}
						v, from := int32(minusInfinity-op-ex), int32(0)
						cont := int32(minusInfinity - ex)
						if valid {
							v, from = prev.best[k]-int32(op+ex), prev.state[k]
							cont = prev.s[z][k] - int32(ex)
						}
						if cont >= v {
							v, from = cont, int32(z)
						}
						cur.s[z][idx] = v
						packed |= from << uint(3*z)
					}
				}
				best, state := cur.s[0][idx], int32(0)
				for z := 1; z < 5; z++ {
					if cur.s[z][idx] > best {
						best, state = cur.s[z][idx], int32(z)
					}
				}
				cur.best[idx], cur.state[idx] = best, state
				record(i, j, idx, packed)
				i++
			}
		}
	}
	best, bi, bj, bs := 0, 0, 0, 0
	for i := 0; i <= m; i++ {
		if boundary != nil {
			boundary.t = max(boundary.t, rowEnd[i])
			if i == m {
				boundary.q = rowBest[i]
			}
		}
		if rowBest[i] > best {
			best, bi, bj, bs = rowBest[i], i, rowJ[i], rowState[i]
		}
		drift := abs((i - bi) - (rowJ[i] - bj))
		if !extend {
			drift = min(drift, abs(m-n))
		}
		if best-rowBest[i] > o.ZDrop+drift*min(o.GapExtend, o.LongGapExtend) {
			if boundary != nil {
				boundary.dropped = true
			}
			if extend {
				break
			}
			return dpResult{score: -o.ZDrop - 1}, nil
		}
	}
	i, j, state, score := m, n, endState, endScore
	if extend {
		i, j, state, score = bi, bj, bs, best
	}
	ei, ej := i, j
	edge := false
	var ops []Op
	for i > 0 || j > 0 {
		if j < starts[i] || j > min(n, i+band) {
			return dpResult{}, &LimitError{"DP band", int64(band + 1), int64(band)}
		}
		if (j == i-band && j > 0) || (j == i+band && j < n) {
			edge = true
		}
		from := int(trace[offset[i]+j-starts[i]] >> uint(3*state) & 7)
		switch state {
		case 0:
			code := byte('X')
			if exact(q[i-1], t[j-1]) {
				code = '='
			}
			ops = addOp(ops, code, 1)
			i--
			j--
		case 1, 3:
			ops = addOp(ops, 'I', 1)
			i--
		case 2, 4:
			ops = addOp(ops, 'D', 1)
			j--
		}
		state = from
	}
	slices.Reverse(ops)
	return dpResult{ops, score, ei, ej, edge}, nil
}
