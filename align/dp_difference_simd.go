//go:build goexperiment.simd && (arm64 || amd64)

package align

import (
	"context"
	"simd/archsimd"
	"slices"
	"sync"
)

// The Suzuki-Kasahara difference recurrence keeps the six local differences
// bounded by scoring parameters, rather than by alignment length. See
// https://doi.org/10.1093/bioinformatics/bty191, section 2.2.2. The implementation
// retains the scalar kernel's five-state priorities and continuation-on-tie
// rule, and finalizes completed rows in order to preserve its Z-drop semantics.
func alignDPPlatform(ctx context.Context, q, t []byte, band int, extend bool, o Options) (dpResult, error) {
	return alignDPPlatformBoundary(ctx, q, t, band, extend, o, nil)
}
func alignDPPlatformBoundary(ctx context.Context, q, t []byte, band int, extend bool, o Options, boundary *dpBoundary) (dpResult, error) {
	if !differenceAvailable() {
		return alignDPScalarBoundary(ctx, q, t, band, extend, o, boundary)
	}
	if len(q) == 0 || len(t) == 0 || len(q) > 4096 || len(t) > 4096 || min(len(q), len(t)) < 16 || band < 16 {
		return differenceFallback(ctx, q, t, band, extend, o, boundary)
	}
	if o.Match > 31 || o.Mismatch > 31 || o.GapOpen > 31 || o.GapExtend > 31 || o.LongGapOpen > 31 || o.LongGapExtend > 31 {
		return differenceFallback(ctx, q, t, band, extend, o, boundary)
	}
	qe := min(o.GapOpen+o.GapExtend, o.LongGapOpen+o.LongGapExtend)
	// Conservative bounds keep every int8 intermediate within [-93,93].
	// Other valid public scoring options retain the wide/scalar fallback.
	if o.GapOpen+o.GapExtend > 31 || o.LongGapOpen+o.LongGapExtend > 31 || o.Match+qe > 31 || o.Mismatch > 2*qe {
		return differenceFallback(ctx, q, t, band, extend, o, boundary)
	}
	if o.MaxCells > 0 && int64(len(q)+1)*int64(min(len(t)+1, 2*band+1)) > o.MaxCells {
		return differenceFallback(ctx, q, t, band, extend, o, boundary)
	}
	return alignDPDifferenceBoundary(ctx, q, t, band, extend, o, boundary)
}

type differenceScratch struct {
	small           []int8
	wide, rows      []int32
	trace           []int8
	starts, offsets []int
	reversed        []byte
}

var differencePool = sync.Pool{New: func() any { return new(differenceScratch) }}

func differenceResize[T any](s []T, n int) []T {
	if cap(s) < n {
		return make([]T, n)
	}
	return s[:n]
}

func alignDPDifference(ctx context.Context, q, t []byte, band int, extend bool, o Options) (dpResult, error) {
	return alignDPDifferenceBoundary(ctx, q, t, band, extend, o, nil)
}
func alignDPDifferenceBoundary(ctx context.Context, q, t []byte, band int, extend bool, o Options, boundary *dpBoundary) (dpResult, error) {
	boundary.reset()
	m, n := len(q), len(t)
	w := differencePool.Get().(*differenceScratch)
	defer func() {
		if cap(w.trace) <= 1<<20 && cap(w.small) <= 1<<18 {
			differencePool.Put(w)
		}
	}()
	stride := m + 17
	w.small = differenceResize(w.small, 18*stride)
	w.wide = differenceResize(w.wide, 3*stride)
	type diagonal struct {
		// U, V, Xshort, Yshort, Xlong, Ylong.
		s [6][]int8
		h []int32
	}
	var ds [3]diagonal
	for k := range ds {
		for z := range ds[k].s {
			ds[k].s[z] = w.small[(k*6+z)*stride : (k*6+z+1)*stride]
		}
		ds[k].h = w.wide[k*stride : (k+1)*stride]
	}
	rowCount := 2
	if boundary != nil {
		rowCount = 3
	}
	w.rows = differenceResize(w.rows, rowCount*(m+1))
	rowBest, rowJ := w.rows[:m+1], w.rows[m+1:2*(m+1)]
	var rowEnd []int32
	if boundary != nil {
		rowEnd = w.rows[2*(m+1):]
		for i := range rowEnd {
			rowEnd[i] = minusInfinity
		}
	}
	for i := range rowBest {
		rowBest[i] = minusInfinity
		rowJ[i] = 0
	}
	w.reversed = differenceResize(w.reversed, n)
	for j, c := range t {
		w.reversed[n-j-1] = c
	}
	w.starts = differenceResize(w.starts, m+n+1)
	w.offsets = differenceResize(w.offsets, m+n+1)
	cells := 0
	for d := 0; d <= m+n; d++ {
		lo, hi := max(0, d-n, (d-band+1)/2), min(m, d, (d+band)/2)
		w.starts[d] = lo
		w.offsets[d] = cells
		cells += max(0, hi-lo+1)
	}
	w.trace = differenceResize(w.trace, cells)
	load8, load32 := archsimd.LoadInt8x16, archsimd.LoadInt32x4
	b8, b32 := archsimd.BroadcastInt8x16, archsimd.BroadcastInt32x4
	zero, match, mis, amb := b8(0), b8(int8(o.Match)), b8(int8(-o.Mismatch)), b8('N')
	shortOpen, shortExt, longOpen, longExt := b8(int8(-o.GapOpen)), b8(int8(o.GapExtend)), b8(int8(-o.LongGapOpen)), b8(int8(o.LongGapExtend))
	one, two, three, four := b8(1), b8(2), b8(3), b8(4)
	lanes := archsimd.LoadInt32x4Array(&[4]int32{0, 1, 2, 3})
	best, bi, bj, bs, nextRow := 0, 0, 0, 0, 0
	dropped := false
	// A row is final once its last band cell has been visited. Consume rows
	// in scalar order; scores from later, partially visited rows never select
	// an endpoint or influence the Z-drop decision.
	finishRows := func(lastD int) bool {
		for nextRow <= m && nextRow+min(n, nextRow+band) <= lastD {
			i := nextRow
			nextRow++
			v, j := int(rowBest[i]), int(rowJ[i])
			if boundary != nil {
				boundary.t = max(boundary.t, int(rowEnd[i]))
				if i == m {
					boundary.q = v
				}
			}

			if v > best {
				best, bi, bj = v, i, j
				bs = int(w.trace[w.offsets[i+j]+i-w.starts[i+j]] & 7)
			}
			drift := abs((i - bi) - (j - bj))
			if !extend {
				drift = min(drift, abs(m-n))
			}
			if best-v > o.ZDrop+drift*min(o.GapExtend, o.LongGapExtend) {
				return true
			}
		}
		return false
	}
	endScore, endState := minusInfinity, 0
	for d := 0; d <= m+n; d++ {
		if d%64 == 0 {
			if err := ctx.Err(); err != nil {
				return dpResult{}, err
			}
		}
		lo, hi := w.starts[d], min(m, d, (d+band)/2)
		if lo > hi {
			break
		}
		cur, prev, prior := &ds[d%3], &ds[(d+2)%3], &ds[(d+1)%3]
		for i := lo; i <= hi; {
			j, idx, k := d-i, i+1, 2*i-d
			ti := w.offsets[d] + i - lo
			if i > 0 && i+15 <= hi && j >= 16 && k > -band && 2*(i+15)-d < band {
				qv := archsimd.LoadUint8x16(q[i-1:]).BitsToInt8()
				tv := archsimd.LoadUint8x16(w.reversed[n-d+i:]).BitsToInt8()
				z := match.IfElse(qv.Equal(tv), mis).IfElse(qv.NotEqual(amb), mis)
				vu, ul := load8(prev.s[1][idx-1:]), load8(prev.s[0][idx:])
				a, b := load8(prev.s[2][idx-1:]).Add(vu), load8(prev.s[3][idx:]).Add(ul)
				c, e := load8(prev.s[4][idx-1:]).Add(vu), load8(prev.s[5][idx:]).Add(ul)
				state := zero
				state = one.IfElse(a.Greater(z), state)
				z = z.Max(a)
				state = two.IfElse(b.Greater(z), state)
				z = z.Max(b)
				state = three.IfElse(c.Greater(z), state)
				z = z.Max(c)
				state = four.IfElse(e.Greater(z), state)
				z = z.Max(e)
				z.Sub(vu).Store(cur.s[0][idx:])
				z.Sub(ul).Store(cur.s[1][idx:])
				ax, bx, cx, ex := a.Sub(z), b.Sub(z), c.Sub(z), e.Sub(z)
				ax.Max(shortOpen).Sub(shortExt).Store(cur.s[2][idx:])
				bx.Max(shortOpen).Sub(shortExt).Store(cur.s[3][idx:])
				cx.Max(longOpen).Sub(longExt).Store(cur.s[4][idx:])
				ex.Max(longOpen).Sub(longExt).Store(cur.s[5][idx:])
				// Low three bits select H's state. The other four bits say
				// whether a gap step FROM this cell continues the same gap.
				packed := state.Or(b8(8).Masked(ax.GreaterEqual(shortOpen))).Or(b8(16).Masked(bx.GreaterEqual(shortOpen))).Or(b8(32).Masked(cx.GreaterEqual(longOpen))).Or(b8(64).Masked(ex.GreaterEqual(longOpen)))
				packed.Store(w.trace[ti:])
				{
					delta := differenceWiden0(z)
					h := load32(prior.h[idx-1+0:]).Add(delta)
					h.Store(cur.h[idx+0:])
					old, jold := load32(rowBest[i+0:]), load32(rowJ[i+0:])
					js := b32(int32(j - 0)).Sub(lanes)
					update := h.Greater(old).Or(h.Equal(old).And(jold.Greater(js)))
					h.Max(old).Store(rowBest[i+0:])
					js.IfElse(update, jold).Store(rowJ[i+0:])
				}
				{
					delta := differenceWiden4(z)
					h := load32(prior.h[idx-1+4:]).Add(delta)
					h.Store(cur.h[idx+4:])
					old, jold := load32(rowBest[i+4:]), load32(rowJ[i+4:])
					js := b32(int32(j - 4)).Sub(lanes)
					update := h.Greater(old).Or(h.Equal(old).And(jold.Greater(js)))
					h.Max(old).Store(rowBest[i+4:])
					js.IfElse(update, jold).Store(rowJ[i+4:])
				}
				{
					delta := differenceWiden8(z)
					h := load32(prior.h[idx-1+8:]).Add(delta)
					h.Store(cur.h[idx+8:])
					old, jold := load32(rowBest[i+8:]), load32(rowJ[i+8:])
					js := b32(int32(j - 8)).Sub(lanes)
					update := h.Greater(old).Or(h.Equal(old).And(jold.Greater(js)))
					h.Max(old).Store(rowBest[i+8:])
					js.IfElse(update, jold).Store(rowJ[i+8:])
				}
				{
					delta := differenceWiden12(z)
					h := load32(prior.h[idx-1+12:]).Add(delta)
					h.Store(cur.h[idx+12:])
					old, jold := load32(rowBest[i+12:]), load32(rowJ[i+12:])
					js := b32(int32(j - 12)).Sub(lanes)
					update := h.Greater(old).Or(h.Equal(old).And(jold.Greater(js)))
					h.Max(old).Store(rowBest[i+12:])
					js.IfElse(update, jold).Store(rowJ[i+12:])
				}
				if boundary != nil && j == n {
					rowEnd[i] = cur.h[idx]
				}
				i += 16
			} else {
				h, state := 0, 0
				var values [6]int
				flags := 0
				if i == 0 || j == 0 {
					length := max(i, j)
					h = -gapCost(length, o)
					values[2], values[3] = -o.GapOpen-o.GapExtend, -o.GapOpen-o.GapExtend
					values[4], values[5] = -o.LongGapOpen-o.LongGapExtend, -o.LongGapOpen-o.LongGapExtend
					if length > 0 {
						short, long := -o.GapOpen-length*o.GapExtend, -o.LongGapOpen-length*o.LongGapExtend
						first, second := 1, 3
						if i == 0 {
							first, second = 2, 4
						}
						state = first
						if long > short {
							state = second
						}
						if i > 0 {
							values[0] = h - int(prev.h[idx-1])
							values[2] = max(-o.GapOpen, short-h) - o.GapExtend
							values[4] = max(-o.LongGapOpen, long-h) - o.LongGapExtend
						} else {
							values[1] = h - int(prev.h[idx])
							values[3] = max(-o.GapOpen, short-h) - o.GapExtend
							values[5] = max(-o.LongGapOpen, long-h) - o.LongGapExtend
						}
						if short-h >= -o.GapOpen {
							flags |= 1 << uint(first+2)
						}
						if long-h >= -o.LongGapOpen {
							flags |= 1 << uint(second+2)
						}
					}
				} else {
					pair := -o.Mismatch
					if exact(q[i-1], t[j-1]) {
						pair = o.Match
					}
					a, b, c, e := minusInfinity, minusInfinity, minusInfinity, minusInfinity
					up, left := k > -band, k < band
					if up {
						a = int(prev.s[2][idx-1]) + int(prev.s[1][idx-1])
						c = int(prev.s[4][idx-1]) + int(prev.s[1][idx-1])
					}
					if left {
						b = int(prev.s[3][idx]) + int(prev.s[0][idx])
						e = int(prev.s[5][idx]) + int(prev.s[0][idx])
					}
					z := pair
					for s, v := range [4]int{a, b, c, e} {
						if v > z {
							z, state = v, s+1
						}
					}
					h = int(prior.h[idx-1]) + z
					if up {
						values[0] = z - int(prev.s[1][idx-1])
					}
					if left {
						values[1] = z - int(prev.s[0][idx])
					}
					values[2], values[3] = max(-o.GapOpen, a-z)-o.GapExtend, max(-o.GapOpen, b-z)-o.GapExtend
					values[4], values[5] = max(-o.LongGapOpen, c-z)-o.LongGapExtend, max(-o.LongGapOpen, e-z)-o.LongGapExtend
					for s, v := range [4]int{a, b, c, e} {
						opening := o.GapOpen
						if s >= 2 {
							opening = o.LongGapOpen
						}
						if v-z >= -opening {
							flags |= 1 << uint(s+3)
						}
					}
				}
				for z, v := range values {
					cur.s[z][idx] = int8(v)
				}
				cur.h[idx] = int32(h)
				if boundary != nil && j == n {
					rowEnd[i] = int32(h)
				}
				w.trace[ti] = int8(state | flags)
				if h > int(rowBest[i]) || (h == int(rowBest[i]) && j < int(rowJ[i])) {
					rowBest[i], rowJ[i] = int32(h), int32(j)
				}
				if i == m && j == n {
					endScore, endState = h, state
				}
				i++
			}
		}
		if finishRows(d) {
			dropped = true
			break
		}
	}
	if !dropped {
		dropped = finishRows(m + n)
	}
	if boundary != nil {
		boundary.dropped = dropped
	}
	if dropped && !extend {
		return dpResult{score: -o.ZDrop - 1}, nil
	}
	i, j, state, score := m, n, endState, endScore
	if extend {
		i, j, state, score = bi, bj, bs, best
	}
	ei, ej := i, j
	edge := false
	var ops []Op
	for i > 0 || j > 0 {
		if j < max(0, i-band) || j > min(n, i+band) {
			return dpResult{}, &LimitError{"DP band", int64(band + 1), int64(band)}
		}
		if (j == i-band && j > 0) || (j == i+band && j < n) {
			edge = true
		}
		if state == 0 {
			code := byte('X')
			if exact(q[i-1], t[j-1]) {
				code = '='
			}
			ops = addOp(ops, code, 1)
			i--
			j--
			state = int(w.trace[w.offsets[i+j]+i-w.starts[i+j]] & 7)
		} else {
			oldState := state
			if state == 1 || state == 3 {
				ops = addOp(ops, 'I', 1)
				i--
			} else {
				ops = addOp(ops, 'D', 1)
				j--
			}
			source := int(w.trace[w.offsets[i+j]+i-w.starts[i+j]])
			state = source & 7
			if source&(1<<uint(oldState+2)) != 0 {
				state = oldState
			}
		}
	}
	slices.Reverse(ops)
	return dpResult{ops, score, ei, ej, edge}, nil
}
