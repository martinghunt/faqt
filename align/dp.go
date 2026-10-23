package align

import (
	"context"
	"slices"
)

const minusInfinity = -1 << 28

// Five states: paired bases, short insertion/deletion, long insertion/deletion.
// Scores use rolling band rows. Traceback uses 15 packed bits per visited cell.
type dpRow struct {
	lo, hi    int
	score     [5][]int
	bestScore []int
	bestState []uint8
}

func reuseRow(r dpRow, lo, hi int) dpRow {
	r.lo, r.hi = lo, hi
	width := max(0, hi-lo+1)
	if cap(r.bestScore) < width {
		capacity := max(width, cap(r.bestScore)*2)
		r.bestScore = make([]int, width, capacity)
		r.bestState = make([]uint8, width, capacity)
	} else {
		r.bestScore = r.bestScore[:width]
		r.bestState = r.bestState[:width]
	}

	for s := range r.score {
		if cap(r.score[s]) < width {
			r.score[s] = make([]int, width, max(width, cap(r.score[s])*2))
		} else {
			r.score[s] = r.score[s][:width]
		}
		for i := range r.score[s] {
			r.score[s][i] = minusInfinity
		}
	}
	return r
}
func (r *dpRow) get(s, j int) int {
	if j < r.lo || j > r.hi {
		return minusInfinity
	}
	return r.score[s][j-r.lo]
}
func (r *dpRow) best(j int) (int, int) {
	if j < r.lo || j > r.hi {
		return minusInfinity, 0
	}
	return r.bestScore[j-r.lo], int(r.bestState[j-r.lo])
}
func (r *dpRow) finish(j int) (int, int) {
	idx := j - r.lo
	best, state := r.score[0][idx], 0
	for s := 1; s < 5; s++ {
		if v := r.score[s][idx]; v > best {
			best, state = v, s
		}
	}
	r.bestScore[idx] = best
	r.bestState[idx] = uint8(state)
	return best, state
}

type dpResult struct {
	ops         []Op
	score, q, t int
	edge        bool
}

func alignGap(ctx context.Context, q, t []byte, o Options) (dpResult, error) {
	if len(q) == 0 {
		return dpResult{ops: addOp(nil, 'D', len(t)), score: -gapCost(len(t), o), t: len(t)}, nil
	}
	if len(t) == 0 {
		return dpResult{ops: addOp(nil, 'I', len(q)), score: -gapCost(len(q), o), q: len(q)}, nil
	}
	band := abs(len(q)-len(t)) + o.BandWidth
	for {
		r, err := alignDP(ctx, q, t, band, false, o)
		if err != nil {
			return dpResult{}, err
		}
		// Traceback alone is insufficient: a better path can leave and
		// re-enter the band while the chosen path never touches its edge.
		if r.score < -o.ZDrop || band >= max(len(q), len(t)) || r.score >= outsideBandBound(len(q), len(t), band, o) {
			return r, nil
		}
		band = min(max(len(q), len(t)), band*2)
	}
}

// An excursion outside |i-j|<=band needs at least these insertion/deletion
// totals. Even perfect remaining matches cannot exceed this score. Positive
// gap opens make one run the cheapest way to consume each total.
func outsideBandBound(m, n, band int, o Options) int {
	d, delta := band+1, m-n
	upper := minusInfinity
	for _, gaps := range [][2]int{{d, max(0, d-delta)}, {max(0, d+delta), d}} {
		ins, del := gaps[0], gaps[1]
		paired := min(m-ins, n-del)
		if paired >= 0 {
			upper = max(upper, paired*o.Match-gapCost(ins, o)-gapCost(del, o))
		}
	}
	return upper
}

func gapCost(n int, o Options) int {
	if n == 0 {
		return 0
	}
	return min(o.GapOpen+n*o.GapExtend, o.LongGapOpen+n*o.LongGapExtend)
}

func alignDP(ctx context.Context, q, t []byte, band int, extend bool, o Options) (dpResult, error) {
	m, n := len(q), len(t)
	if extend && (m == 0 || n == 0) {
		return dpResult{}, nil
	}
	offset := make([]int, m+1)
	starts := make([]int, m+1)
	var cells int64
	// Allocate traceback as rows are visited. Z-drop may stop a large gap
	// early, without reserving memory for a matrix we will never compute.
	var trace []uint16
	if extend {
		capacity := min(int64(m+1)*int64(min(n+1, 2*band+1)), 65536)
		if o.MaxCells > 0 {
			capacity = min(capacity, o.MaxCells)
		}
		trace = make([]uint16, 0, int(capacity))
	}
	var prev, scratch dpRow
	best, bestI, bestJ, bestState := 0, 0, 0, 0
	endScore, endState := minusInfinity, 0
	for i := 0; i <= m; i++ {
		if i%64 == 0 {
			if err := ctx.Err(); err != nil {
				return dpResult{}, err
			}
		}
		lo, hi := max(0, i-band), min(n, i+band)
		starts[i] = lo
		offset[i] = int(cells)
		width := max(0, hi-lo+1)
		cells += int64(width)
		if o.MaxCells > 0 && cells > o.MaxCells {
			return dpResult{}, &LimitError{"DP cells", cells, o.MaxCells}
		}
		trace = append(trace, make([]uint16, width)...)
		row := reuseRow(scratch, lo, hi)
		rowBest := minusInfinity
		rowBestJ := 0
		for j := lo; j <= hi; j++ {
			idx := j - lo
			var packed uint16
			if i == 0 && j == 0 {
				row.score[0][idx] = 0
			} else {
				if i > 0 && j > 0 {
					v, s := prev.best(j - 1)
					pair := -o.Mismatch
					if exact(q[i-1], t[j-1]) {
						pair = o.Match
					}
					row.score[0][idx] = v + pair
					packed |= uint16(s)
				}
				for s := 1; s < 5; s++ {
					source, pos := &prev, j
					if s == 2 || s == 4 {
						source, pos = &row, j-1
					}
					open, extension := o.GapOpen, o.GapExtend
					if s >= 3 {
						open, extension = o.LongGapOpen, o.LongGapExtend
					}
					// Open from the best other state. This also permits adjacent opposite
					// gaps when that scores better than an extremely costly substitution.
					v, from := source.best(pos)
					v -= open + extension
					cont := source.get(s, pos) - extension
					if cont >= v {
						v, from = cont, s
					}
					row.score[s][idx] = v
					packed |= uint16(from) << uint(3*s)
				}
			}
			trace[offset[i]+idx] = packed
			v, s := row.finish(j)
			if v > rowBest {
				rowBest = v
				rowBestJ = j
			}
			if v > best {
				best, bestI, bestJ, bestState = v, i, j, s
			}
		}
		if i == m {
			endScore, endState = row.best(n)
		}
		scratch, prev = prev, row
		// Z-drop discounts the unavoidable extension cost of an indel.
		// Global gaps that lose support split the chain, rather than forcing
		// a huge alignment through unrelated sequence.
		drift := abs((i - bestI) - (rowBestJ - bestJ))
		if !extend {
			drift = min(drift, abs(m-n))
		}
		if best-rowBest > o.ZDrop+drift*min(o.GapExtend, o.LongGapExtend) {
			if extend {
				break
			}
			return dpResult{score: -o.ZDrop - 1}, nil
		}
	}
	i, j, state := m, n, endState
	score := endScore
	if extend {
		i, j, state, score = bestI, bestJ, bestState, best
	}
	endI, endJ := i, j
	var reverseOps []Op
	edge := false
	for i > 0 || j > 0 {
		lo := starts[i]
		if j < lo || j > min(n, i+band) {
			return dpResult{}, &LimitError{"DP band", int64(band + 1), int64(band)}
		}
		if (j == i-band && j > 0) || (j == i+band && j < n) {
			edge = true
		}
		packed := trace[offset[i]+j-lo]
		from := int(packed >> uint(3*state) & 7)
		switch state {
		case 0:
			code := byte('X')
			if exact(q[i-1], t[j-1]) {
				code = '='
			}
			reverseOps = addOp(reverseOps, code, 1)
			i--
			j--
		case 1, 3:
			reverseOps = addOp(reverseOps, 'I', 1)
			i--
		case 2, 4:
			reverseOps = addOp(reverseOps, 'D', 1)
			j--
		}
		state = from
	}
	slices.Reverse(reverseOps)
	return dpResult{reverseOps, score, endI, endJ, edge}, nil
}
