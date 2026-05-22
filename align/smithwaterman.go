package align

import (
	"fmt"

	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/seq"
)

func (a SmithWatermanAligner) Align(candidate mapper.Candidate) (Result, error) {
	return smithWatermanAlign(candidate, a.Options)
}

func smithWatermanAlign(candidate mapper.Candidate, opts Options) (Result, error) {
	if err := opts.validate(); err != nil {
		return Result{}, err
	}
	if len(candidate.QuerySeq) == 0 {
		return Result{}, fmt.Errorf("candidate query sequence is empty")
	}
	if len(candidate.RefSeqOriented) == 0 {
		return Result{}, fmt.Errorf("candidate reference sequence is empty")
	}

	query := candidate.QuerySeq
	ref := candidate.RefSeqOriented
	rows := len(query) + 1
	cols := len(ref) + 1
	dp := newAlignmentDP(rows, cols, 0, negInf)

	bestScore := 0
	bestI, bestJ := 0, 0
	bestState := byte(0)
	for i := 1; i < rows; i++ {
		rowBest := negInf
		for j := 1; j < cols; j++ {
			if !inBand(i, j, len(query), len(ref), opts.BandWidth) {
				continue
			}
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
			dp.mm[idx] = bestPrev + scorePair(query[i-1], ref[j-1], opts.Scoring)

			bestX := 0
			dp.traceX[idx] = 0
			openX := dp.mm[upIdx] + opts.Scoring.GapOpen
			if openX > bestX {
				bestX = openX
				dp.traceX[idx] = 'M'
			}
			extendX := dp.ix[upIdx] + opts.Scoring.GapExtend
			if extendX > bestX {
				bestX = extendX
				dp.traceX[idx] = 'X'
			}
			dp.ix[idx] = bestX

			bestY := 0
			dp.traceY[idx] = 0
			openY := dp.mm[leftIdx] + opts.Scoring.GapOpen
			if openY > bestY {
				bestY = openY
				dp.traceY[idx] = 'M'
			}
			extendY := dp.iy[leftIdx] + opts.Scoring.GapExtend
			if extendY > bestY {
				bestY = extendY
				dp.traceY[idx] = 'Y'
			}
			dp.iy[idx] = bestY

			score, state := dp.bestState(idx)
			if score > bestScore {
				bestScore = score
				bestI, bestJ = i, j
				bestState = state
			}
			if score > rowBest {
				rowBest = score
			}
		}
		if opts.XDrop > 0 && rowBest != negInf && bestScore-rowBest > opts.XDrop {
			break
		}
	}
	if bestScore == 0 {
		return Result{
			Candidate: candidate,
		}, nil
	}

	queryEnd, refEnd := bestI, bestJ
	traceback, err := dp.trace(query, ref, bestI, bestJ, bestState, true, "")
	if err != nil {
		return Result{}, err
	}
	queryStart, refStart := traceback.queryStart, traceback.refStart

	queryRange := seq.Interval{
		Start: candidate.QueryRange.Start + queryStart,
		End:   candidate.QueryRange.Start + queryEnd,
	}
	refForwardRange := orientedToForwardRange(candidate, refStart, refEnd)

	identity := 0.0
	if traceback.aligned > 0 {
		identity = float64(traceback.matches) / float64(traceback.aligned)
	}

	return Result{
		Candidate:       candidate,
		Score:           bestScore,
		QueryRange:      queryRange,
		RefRangeForward: refForwardRange,
		CIGAR:           compressOps(traceback.ops),
		Matches:         traceback.matches,
		AlignedLength:   traceback.aligned,
		Identity:        identity,
	}, nil
}
