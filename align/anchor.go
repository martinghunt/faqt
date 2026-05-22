package align

import (
	"slices"

	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/seq"
)

type anchorBlock struct {
	queryStart int
	queryEnd   int
	refStart   int
	refEnd     int
}

func (a AnchorGuidedAligner) Align(candidate mapper.Candidate) (Result, error) {
	if err := a.Options.validate(); err != nil {
		return Result{}, err
	}
	blocks := anchorBlocks(candidate)
	if len(blocks) == 0 {
		return smithWatermanAlign(candidate, a.Options)
	}

	var (
		ops          []byte
		score        int
		matches      int
		aligned      int
		queryStart   = blocks[0].queryStart
		queryEnd     = blocks[len(blocks)-1].queryEnd
		refStart     = blocks[0].refStart
		refEnd       = blocks[len(blocks)-1].refEnd
		prevQueryEnd = blocks[0].queryStart
		prevRefEnd   = blocks[0].refStart
	)

	left, err := suffixAlign(
		candidate.QuerySeq[:blocks[0].queryStart],
		candidate.RefSeqOriented[:blocks[0].refStart],
		a.Options.Scoring,
		a.Options.XDrop,
	)
	if err != nil {
		return Result{}, err
	}
	if left.queryClip > 0 {
		for i := 0; i < left.queryClip; i++ {
			ops = append(ops, 'S')
		}
	}
	ops = append(ops, left.ops...)
	score += left.score
	matches += left.matches
	aligned += left.aligned
	queryStart = left.queryClip
	refStart = left.refClip

	for _, block := range blocks {
		if block.queryStart > prevQueryEnd || block.refStart > prevRefEnd {
			gapResult, err := globalAlign(
				candidate.QuerySeq[prevQueryEnd:block.queryStart],
				candidate.RefSeqOriented[prevRefEnd:block.refStart],
				a.Options.Scoring,
				a.Options.XDrop,
				a.Options.BandWidth,
			)
			if err != nil {
				return Result{}, err
			}
			ops = append(ops, gapResult.ops...)
			score += gapResult.score
			matches += gapResult.matches
			aligned += gapResult.aligned
		}

		matchLen := block.queryEnd - block.queryStart
		for i := 0; i < matchLen; i++ {
			ops = append(ops, 'M')
		}
		score += matchLen * a.Options.Scoring.Match
		matches += matchLen
		aligned += matchLen
		prevQueryEnd = block.queryEnd
		prevRefEnd = block.refEnd
	}

	right, err := prefixAlign(
		candidate.QuerySeq[prevQueryEnd:],
		candidate.RefSeqOriented[prevRefEnd:],
		a.Options.Scoring,
		a.Options.XDrop,
	)
	if err != nil {
		return Result{}, err
	}
	ops = append(ops, right.ops...)
	score += right.score
	matches += right.matches
	aligned += right.aligned
	if right.queryClip > 0 {
		for i := 0; i < right.queryClip; i++ {
			ops = append(ops, 'S')
		}
	}
	queryEnd = len(candidate.QuerySeq) - right.queryClip
	refEnd = len(candidate.RefSeqOriented) - right.refClip

	if len(ops) == 0 {
		return smithWatermanAlign(candidate, a.Options)
	}

	queryRange := seq.Interval{
		Start: candidate.QueryRange.Start + queryStart,
		End:   candidate.QueryRange.Start + queryEnd,
	}
	refForwardRange := orientedToForwardRange(candidate, refStart, refEnd)
	identity := float64(matches) / float64(aligned)

	return Result{
		Candidate:       candidate,
		Score:           score,
		QueryRange:      queryRange,
		RefRangeForward: refForwardRange,
		CIGAR:           compressOps(ops),
		Matches:         matches,
		AlignedLength:   aligned,
		Identity:        identity,
	}, nil
}

func anchorBlocks(candidate mapper.Candidate) []anchorBlock {
	k := candidate.SeedLength
	if k <= 0 || len(candidate.Chain.Anchors) == 0 {
		return nil
	}

	blocks := make([]anchorBlock, 0, len(candidate.Chain.Anchors))
	refLen := len(candidate.RefSeqForward)
	for _, anchor := range candidate.Chain.Anchors {
		qs := anchor.QueryPos - candidate.QueryRange.Start
		if qs < 0 || qs+k > len(candidate.QuerySeq) {
			continue
		}

		rf := anchor.RefPos - candidate.RefRange.Start
		if rf < 0 || rf+k > refLen {
			continue
		}

		rs := rf
		if candidate.RelativeStrand == 1 {
			rs = refLen - (rf + k)
		}
		if rs < 0 || rs+k > len(candidate.RefSeqOriented) {
			continue
		}

		blocks = append(blocks, anchorBlock{
			queryStart: qs,
			queryEnd:   qs + k,
			refStart:   rs,
			refEnd:     rs + k,
		})
	}
	if len(blocks) == 0 {
		return nil
	}

	slices.SortFunc(blocks, func(a, b anchorBlock) int {
		switch {
		case a.queryStart != b.queryStart:
			if a.queryStart < b.queryStart {
				return -1
			}
			return 1
		case a.refStart != b.refStart:
			if a.refStart < b.refStart {
				return -1
			}
			return 1
		default:
			return 0
		}
	})

	merged := []anchorBlock{blocks[0]}
	for _, block := range blocks[1:] {
		last := &merged[len(merged)-1]
		if block.queryStart < last.queryStart || block.refStart < last.refStart {
			continue
		}
		if block.queryStart <= last.queryEnd && block.refStart <= last.refEnd {
			if block.queryEnd > last.queryEnd {
				last.queryEnd = block.queryEnd
			}
			if block.refEnd > last.refEnd {
				last.refEnd = block.refEnd
			}
			continue
		}
		if block.queryStart < last.queryEnd || block.refStart < last.refEnd {
			continue
		}
		merged = append(merged, block)
	}
	return merged
}
