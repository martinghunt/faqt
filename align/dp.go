package align

import (
	"fmt"
	"slices"
)

type globalAlignment struct {
	ops     []byte
	score   int
	matches int
	aligned int
}

type endAlignment struct {
	ops       []byte
	score     int
	matches   int
	aligned   int
	queryClip int
	refClip   int
}

const negInf = -1 << 29

type alignmentDP struct {
	cols   int
	mm     []int
	ix     []int
	iy     []int
	traceM []byte
	traceX []byte
	traceY []byte
}

type alignmentTraceback struct {
	ops        []byte
	matches    int
	aligned    int
	queryStart int
	refStart   int
}

func newAlignmentDP(rows, cols, matchInit, gapInit int) alignmentDP {
	size := rows * cols
	dp := alignmentDP{
		cols:   cols,
		mm:     make([]int, size),
		ix:     make([]int, size),
		iy:     make([]int, size),
		traceM: make([]byte, size),
		traceX: make([]byte, size),
		traceY: make([]byte, size),
	}
	for idx := range dp.mm {
		dp.mm[idx], dp.ix[idx], dp.iy[idx] = matchInit, gapInit, gapInit
	}
	return dp
}

func (dp alignmentDP) index(i, j int) int {
	return i*dp.cols + j
}

func (dp alignmentDP) bestState(idx int) (int, byte) {
	score := dp.mm[idx]
	state := byte('M')
	if dp.ix[idx] > score {
		score = dp.ix[idx]
		state = 'X'
	}
	if dp.iy[idx] > score {
		score = dp.iy[idx]
		state = 'Y'
	}
	return score, state
}

func (dp alignmentDP) stateScore(idx int, state byte) (int, bool) {
	switch state {
	case 'M':
		return dp.mm[idx], true
	case 'X':
		return dp.ix[idx], true
	case 'Y':
		return dp.iy[idx], true
	default:
		return 0, false
	}
}

func (dp alignmentDP) trace(query, ref []byte, i, j int, state byte, local bool, context string) (alignmentTraceback, error) {
	ops := make([]byte, 0, i+j)
	matches := 0
	aligned := 0
	for traceContinues(local, i, j) {
		idx := dp.index(i, j)
		if local {
			if state == 0 {
				break
			}
			if score, ok := dp.stateScore(idx, state); ok && score == 0 {
				break
			}
		}
		switch state {
		case 'M':
			ops = append(ops, 'M')
			if i > 0 && j > 0 && query[i-1] == ref[j-1] {
				matches++
			}
			aligned++
			state = dp.traceM[idx]
			i--
			j--
		case 'X':
			ops = append(ops, 'D')
			aligned++
			state = dp.traceX[idx]
			i--
		case 'Y':
			ops = append(ops, 'I')
			aligned++
			state = dp.traceY[idx]
			j--
		default:
			if context == "" {
				return alignmentTraceback{}, fmt.Errorf("unexpected traceback state %q", state)
			}
			return alignmentTraceback{}, fmt.Errorf("unexpected %s traceback state %q", context, state)
		}
	}
	slices.Reverse(ops)
	return alignmentTraceback{
		ops:        ops,
		matches:    matches,
		aligned:    aligned,
		queryStart: i,
		refStart:   j,
	}, nil
}

func traceContinues(local bool, i, j int) bool {
	if local {
		return i > 0 && j > 0
	}
	return i > 0 || j > 0
}

func globalAlign(query, ref []byte, scoring Scoring, xdrop, bandWidth int) (globalAlignment, error) {
	if len(query) == 0 && len(ref) == 0 {
		return globalAlignment{}, nil
	}

	rows := len(query) + 1
	cols := len(ref) + 1
	dp := newAlignmentDP(rows, cols, negInf, negInf)
	dp.mm[0] = 0
	for i := 1; i < rows; i++ {
		idx := dp.index(i, 0)
		if inBand(i, 0, len(query), len(ref), bandWidth) {
			dp.ix[idx] = scoring.GapOpen + (i-1)*scoring.GapExtend
			dp.traceX[idx] = 'X'
		}
	}
	for j := 1; j < cols; j++ {
		if inBand(0, j, len(query), len(ref), bandWidth) {
			dp.iy[j] = scoring.GapOpen + (j-1)*scoring.GapExtend
			dp.traceY[j] = 'Y'
		}
	}

	bestScore := 0
	for i := 1; i < rows; i++ {
		rowBest := negInf
		for j := 1; j < cols; j++ {
			if !inBand(i, j, len(query), len(ref), bandWidth) {
				continue
			}
			idx := dp.index(i, j)
			diagIdx := dp.index(i-1, j-1)
			upIdx := dp.index(i-1, j)
			leftIdx := dp.index(i, j-1)

			bestPrev := dp.mm[diagIdx]
			dp.traceM[idx] = 'M'
			if dp.ix[diagIdx] > bestPrev {
				bestPrev = dp.ix[diagIdx]
				dp.traceM[idx] = 'X'
			}
			if dp.iy[diagIdx] > bestPrev {
				bestPrev = dp.iy[diagIdx]
				dp.traceM[idx] = 'Y'
			}
			dp.mm[idx] = bestPrev + scorePair(query[i-1], ref[j-1], scoring)

			openX := dp.mm[upIdx] + scoring.GapOpen
			extendX := dp.ix[upIdx] + scoring.GapExtend
			if openX >= extendX {
				dp.ix[idx] = openX
				dp.traceX[idx] = 'M'
			} else {
				dp.ix[idx] = extendX
				dp.traceX[idx] = 'X'
			}

			openY := dp.mm[leftIdx] + scoring.GapOpen
			extendY := dp.iy[leftIdx] + scoring.GapExtend
			if openY >= extendY {
				dp.iy[idx] = openY
				dp.traceY[idx] = 'M'
			} else {
				dp.iy[idx] = extendY
				dp.traceY[idx] = 'Y'
			}

			cellBest, _ := dp.bestState(idx)
			if cellBest > rowBest {
				rowBest = cellBest
			}
			if cellBest > bestScore {
				bestScore = cellBest
			}
		}
		if xdrop > 0 && rowBest != negInf && bestScore-rowBest > xdrop {
			break
		}
	}

	i, j := len(query), len(ref)
	best, state := dp.bestState(dp.index(i, j))
	if best == negInf {
		return globalAlignment{}, nil
	}
	traceback, err := dp.trace(query, ref, i, j, state, false, "global")
	if err != nil {
		return globalAlignment{}, err
	}

	return globalAlignment{
		ops:     traceback.ops,
		score:   best,
		matches: traceback.matches,
		aligned: traceback.aligned,
	}, nil
}

func suffixAlign(query, ref []byte, scoring Scoring, xdrop int) (endAlignment, error) {
	if len(query) == 0 && len(ref) == 0 {
		return endAlignment{}, nil
	}
	rows := len(query) + 1
	cols := len(ref) + 1
	dp := newAlignmentDP(rows, cols, negInf, negInf)
	for i := 0; i < rows; i++ {
		dp.mm[dp.index(i, 0)] = 0
	}
	for j := 0; j < cols; j++ {
		dp.mm[j] = 0
	}

	bestScore := 0
	bestI, bestJ := rows-1, cols-1
	bestState := byte(0)

	for i := 1; i < rows; i++ {
		rowBest := negInf
		for j := 1; j < cols; j++ {
			idx := dp.index(i, j)
			diagIdx := dp.index(i-1, j-1)
			upIdx := dp.index(i-1, j)
			leftIdx := dp.index(i, j-1)

			bestPrev := 0
			dp.traceM[idx] = 0
			if dp.mm[diagIdx] > bestPrev {
				bestPrev = dp.mm[diagIdx]
				dp.traceM[idx] = 'M'
			}
			if dp.ix[diagIdx] > bestPrev {
				bestPrev = dp.ix[diagIdx]
				dp.traceM[idx] = 'X'
			}
			if dp.iy[diagIdx] > bestPrev {
				bestPrev = dp.iy[diagIdx]
				dp.traceM[idx] = 'Y'
			}
			dp.mm[idx] = bestPrev + scorePair(query[i-1], ref[j-1], scoring)

			bestX := 0
			dp.traceX[idx] = 0
			openX := dp.mm[upIdx] + scoring.GapOpen
			if openX > bestX {
				bestX = openX
				dp.traceX[idx] = 'M'
			}
			extendX := dp.ix[upIdx] + scoring.GapExtend
			if extendX > bestX {
				bestX = extendX
				dp.traceX[idx] = 'X'
			}
			dp.ix[idx] = bestX

			bestY := 0
			dp.traceY[idx] = 0
			openY := dp.mm[leftIdx] + scoring.GapOpen
			if openY > bestY {
				bestY = openY
				dp.traceY[idx] = 'M'
			}
			extendY := dp.iy[leftIdx] + scoring.GapExtend
			if extendY > bestY {
				bestY = extendY
				dp.traceY[idx] = 'Y'
			}
			dp.iy[idx] = bestY

			cellBest, cellState := dp.bestState(idx)
			if cellBest > rowBest {
				rowBest = cellBest
			}
			if cellBest > bestScore {
				bestScore = cellBest
				bestI, bestJ = i, j
				bestState = cellState
			}
		}
		if xdrop > 0 && bestScore-rowBest > xdrop {
			break
		}
	}

	traceback, err := dp.trace(query, ref, bestI, bestJ, bestState, true, "suffix")
	if err != nil {
		return endAlignment{}, err
	}
	return endAlignment{
		ops:       traceback.ops,
		score:     bestScore,
		matches:   traceback.matches,
		aligned:   traceback.aligned,
		queryClip: traceback.queryStart,
		refClip:   traceback.refStart + (len(ref) - bestJ),
	}, nil
}

func prefixAlign(query, ref []byte, scoring Scoring, xdrop int) (endAlignment, error) {
	rq := append([]byte(nil), query...)
	rr := append([]byte(nil), ref...)
	slices.Reverse(rq)
	slices.Reverse(rr)
	res, err := suffixAlign(rq, rr, scoring, xdrop)
	if err != nil {
		return endAlignment{}, err
	}
	slices.Reverse(res.ops)
	return res, nil
}

func inBand(i, j, qlen, rlen, bandWidth int) bool {
	if bandWidth <= 0 || qlen == 0 || rlen == 0 {
		return true
	}
	center := int(float64(i) * float64(rlen) / float64(qlen))
	if center-bandWidth > j || center+bandWidth < j {
		return false
	}
	return true
}
