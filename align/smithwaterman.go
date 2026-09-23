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
	local := fillLocalDP(query, ref, opts.Scoring, opts.XDrop, opts.BandWidth)
	if local.score == 0 {
		return Result{
			Candidate: candidate,
		}, nil
	}

	queryEnd, refEnd := local.queryEnd, local.refEnd
	traceback, err := local.dp.trace(query, ref, queryEnd, refEnd, local.bestState, true, "")
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
		Score:           local.score,
		QueryRange:      queryRange,
		RefRangeForward: refForwardRange,
		CIGAR:           compressOps(traceback.ops),
		Matches:         traceback.matches,
		AlignedLength:   traceback.aligned,
		Identity:        identity,
	}, nil
}
