package align

import (
	"fmt"
	"slices"
	"strings"

	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/seq"
)

type Scoring struct {
	Match     int
	Mismatch  int
	GapOpen   int
	GapExtend int
}

type Options struct {
	Scoring       Scoring
	MaxAlignments int
	XDrop         int
	BandWidth     int
}

type Result struct {
	Candidate       mapper.Candidate
	Score           int
	QueryRange      seq.Interval
	RefRangeForward seq.Interval
	CIGAR           string
	Matches         int
	AlignedLength   int
	Identity        float64
}

type Aligner interface {
	Align(mapper.Candidate) (Result, error)
}

type SmithWatermanAligner struct {
	Options Options
}

type AnchorGuidedAligner struct {
	Options Options
}

func DefaultAligner() AnchorGuidedAligner {
	return AnchorGuidedAligner{
		Options: Options{
			Scoring: Scoring{
				Match:     2,
				Mismatch:  -4,
				GapOpen:   -4,
				GapExtend: -2,
			},
			MaxAlignments: 5,
			XDrop:         20,
			BandWidth:     64,
		},
	}
}

func AlignCandidates(aligner Aligner, candidates []mapper.Candidate, opts Options) ([]Result, error) {
	if aligner == nil {
		return nil, fmt.Errorf("aligner is required")
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(candidates))
	for _, candidate := range candidates {
		result, err := aligner.Align(candidate)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	slices.SortFunc(results, compareResults)
	if opts.MaxAlignments > 0 && len(results) > opts.MaxAlignments {
		results = results[:opts.MaxAlignments]
	}
	return results, nil
}

func (a SmithWatermanAligner) Align(candidate mapper.Candidate) (Result, error) {
	return smithWatermanAlign(candidate, a.Options)
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
	mm := make([]int, rows*cols)
	ix := make([]int, rows*cols)
	iy := make([]int, rows*cols)
	traceM := make([]byte, rows*cols)
	traceX := make([]byte, rows*cols)
	traceY := make([]byte, rows*cols)

	for idx := range mm {
		mm[idx], ix[idx], iy[idx] = 0, negInf, negInf
	}

	bestScore := 0
	bestI, bestJ := 0, 0
	bestState := byte(0)
	for i := 1; i < rows; i++ {
		rowBest := negInf
		for j := 1; j < cols; j++ {
			if !inBand(i, j, len(query), len(ref), opts.BandWidth) {
				continue
			}
			idx := i*cols + j
			diagIdx := (i-1)*cols + (j - 1)
			upIdx := (i-1)*cols + j
			leftIdx := i*cols + (j - 1)

			bestPrev := 0
			traceM[idx] = 0
			if mm[diagIdx] > bestPrev {
				bestPrev = mm[diagIdx]
				traceM[idx] = 'M'
			}
			if ix[diagIdx] > bestPrev {
				bestPrev = ix[diagIdx]
				traceM[idx] = 'X'
			}
			if iy[diagIdx] > bestPrev {
				bestPrev = iy[diagIdx]
				traceM[idx] = 'Y'
			}
			mm[idx] = bestPrev + scorePair(query[i-1], ref[j-1], opts.Scoring)

			bestX := 0
			traceX[idx] = 0
			openX := mm[upIdx] + opts.Scoring.GapOpen
			if openX > bestX {
				bestX = openX
				traceX[idx] = 'M'
			}
			extendX := ix[upIdx] + opts.Scoring.GapExtend
			if extendX > bestX {
				bestX = extendX
				traceX[idx] = 'X'
			}
			ix[idx] = bestX

			bestY := 0
			traceY[idx] = 0
			openY := mm[leftIdx] + opts.Scoring.GapOpen
			if openY > bestY {
				bestY = openY
				traceY[idx] = 'M'
			}
			extendY := iy[leftIdx] + opts.Scoring.GapExtend
			if extendY > bestY {
				bestY = extendY
				traceY[idx] = 'Y'
			}
			iy[idx] = bestY

			score := mm[idx]
			state := byte('M')
			if ix[idx] > score {
				score = ix[idx]
				state = 'X'
			}
			if iy[idx] > score {
				score = iy[idx]
				state = 'Y'
			}
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

	i, j := bestI, bestJ
	queryEnd, refEnd := i, j
	ops := make([]byte, 0, queryEnd+refEnd)
	matches := 0
	aligned := 0
	state := bestState
	for i > 0 && j > 0 {
		idx := i*cols + j
		current := mm[idx]
		if state == 'X' {
			current = ix[idx]
		} else if state == 'Y' {
			current = iy[idx]
		}
		if state == 0 || current == 0 {
			break
		}
		switch state {
		case 'M':
			ops = append(ops, 'M')
			if query[i-1] == ref[j-1] {
				matches++
			}
			aligned++
			state = traceM[idx]
			i--
			j--
		case 'X':
			ops = append(ops, 'D')
			aligned++
			state = traceX[idx]
			i--
		case 'Y':
			ops = append(ops, 'I')
			aligned++
			state = traceY[idx]
			j--
		default:
			return Result{}, fmt.Errorf("unexpected traceback state %q", state)
		}
	}
	queryStart, refStart := i, j
	slices.Reverse(ops)

	queryRange := seq.Interval{
		Start: candidate.QueryRange.Start + queryStart,
		End:   candidate.QueryRange.Start + queryEnd,
	}
	refForwardRange := orientedToForwardRange(candidate, refStart, refEnd)

	identity := 0.0
	if aligned > 0 {
		identity = float64(matches) / float64(aligned)
	}

	return Result{
		Candidate:       candidate,
		Score:           bestScore,
		QueryRange:      queryRange,
		RefRangeForward: refForwardRange,
		CIGAR:           compressOps(ops),
		Matches:         matches,
		AlignedLength:   aligned,
		Identity:        identity,
	}, nil
}

func (o Options) validate() error {
	if err := o.Scoring.validate(); err != nil {
		return err
	}
	if o.MaxAlignments < 0 {
		return fmt.Errorf("max alignments must be >= 0")
	}
	if o.XDrop < 0 {
		return fmt.Errorf("x-drop must be >= 0")
	}
	if o.BandWidth < 0 {
		return fmt.Errorf("band width must be >= 0")
	}
	return nil
}

func (s Scoring) validate() error {
	if s.Match <= 0 {
		return fmt.Errorf("match score must be > 0")
	}
	if s.Mismatch >= 0 {
		return fmt.Errorf("mismatch score must be < 0")
	}
	if s.GapOpen >= 0 {
		return fmt.Errorf("gap open score must be < 0")
	}
	if s.GapExtend >= 0 {
		return fmt.Errorf("gap extend score must be < 0")
	}
	return nil
}

func scorePair(a, b byte, scoring Scoring) int {
	if a == b {
		return scoring.Match
	}
	return scoring.Mismatch
}

func compressOps(ops []byte) string {
	if len(ops) == 0 {
		return ""
	}
	var b strings.Builder
	run := 1
	for i := 1; i <= len(ops); i++ {
		if i < len(ops) && ops[i] == ops[i-1] {
			run++
			continue
		}
		fmt.Fprintf(&b, "%d%c", run, ops[i-1])
		run = 1
	}
	return b.String()
}

func orientedToForwardRange(candidate mapper.Candidate, start, end int) seq.Interval {
	if candidate.RelativeStrand == 0 {
		return seq.Interval{
			Start: candidate.RefRange.Start + start,
			End:   candidate.RefRange.Start + end,
		}
	}
	n := len(candidate.RefSeqForward)
	return seq.Interval{
		Start: candidate.RefRange.Start + (n - end),
		End:   candidate.RefRange.Start + (n - start),
	}
}

func compareResults(a, b Result) int {
	switch {
	case a.Score != b.Score:
		if a.Score > b.Score {
			return -1
		}
		return 1
	case a.Identity != b.Identity:
		if a.Identity > b.Identity {
			return -1
		}
		return 1
	case a.Candidate.Chain.Score != b.Candidate.Chain.Score:
		if a.Candidate.Chain.Score > b.Candidate.Chain.Score {
			return -1
		}
		return 1
	default:
		return 0
	}
}

type anchorBlock struct {
	queryStart int
	queryEnd   int
	refStart   int
	refEnd     int
}

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

func globalAlign(query, ref []byte, scoring Scoring, xdrop, bandWidth int) (globalAlignment, error) {
	if len(query) == 0 && len(ref) == 0 {
		return globalAlignment{}, nil
	}

	rows := len(query) + 1
	cols := len(ref) + 1
	mm := make([]int, rows*cols)
	ix := make([]int, rows*cols)
	iy := make([]int, rows*cols)
	traceM := make([]byte, rows*cols)
	traceX := make([]byte, rows*cols)
	traceY := make([]byte, rows*cols)

	for idx := range mm {
		mm[idx], ix[idx], iy[idx] = negInf, negInf, negInf
	}
	mm[0] = 0
	for i := 1; i < rows; i++ {
		idx := i * cols
		if inBand(i, 0, len(query), len(ref), bandWidth) {
			ix[idx] = scoring.GapOpen + (i-1)*scoring.GapExtend
			traceX[idx] = 'X'
		}
	}
	for j := 1; j < cols; j++ {
		if inBand(0, j, len(query), len(ref), bandWidth) {
			iy[j] = scoring.GapOpen + (j-1)*scoring.GapExtend
			traceY[j] = 'Y'
		}
	}

	bestScore := 0
	for i := 1; i < rows; i++ {
		rowBest := negInf
		for j := 1; j < cols; j++ {
			if !inBand(i, j, len(query), len(ref), bandWidth) {
				continue
			}
			idx := i*cols + j
			diagIdx := (i-1)*cols + (j - 1)
			upIdx := (i-1)*cols + j
			leftIdx := i*cols + (j - 1)

			bestPrev := mm[diagIdx]
			traceM[idx] = 'M'
			if ix[diagIdx] > bestPrev {
				bestPrev = ix[diagIdx]
				traceM[idx] = 'X'
			}
			if iy[diagIdx] > bestPrev {
				bestPrev = iy[diagIdx]
				traceM[idx] = 'Y'
			}
			mm[idx] = bestPrev + scorePair(query[i-1], ref[j-1], scoring)

			openX := mm[upIdx] + scoring.GapOpen
			extendX := ix[upIdx] + scoring.GapExtend
			if openX >= extendX {
				ix[idx] = openX
				traceX[idx] = 'M'
			} else {
				ix[idx] = extendX
				traceX[idx] = 'X'
			}

			openY := mm[leftIdx] + scoring.GapOpen
			extendY := iy[leftIdx] + scoring.GapExtend
			if openY >= extendY {
				iy[idx] = openY
				traceY[idx] = 'M'
			} else {
				iy[idx] = extendY
				traceY[idx] = 'Y'
			}

			cellBest := mm[idx]
			if ix[idx] > cellBest {
				cellBest = ix[idx]
			}
			if iy[idx] > cellBest {
				cellBest = iy[idx]
			}
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
	ops := make([]byte, 0, i+j)
	matches := 0
	aligned := 0
	state := byte('M')
	best := mm[i*cols+j]
	if ix[i*cols+j] > best {
		best = ix[i*cols+j]
		state = 'X'
	}
	if iy[i*cols+j] > best {
		best = iy[i*cols+j]
		state = 'Y'
	}
	if best == negInf {
		return globalAlignment{}, nil
	}
	for i > 0 || j > 0 {
		idx := i*cols + j
		switch state {
		case 'M':
			ops = append(ops, 'M')
			if i > 0 && j > 0 && query[i-1] == ref[j-1] {
				matches++
			}
			aligned++
			state = traceM[idx]
			i--
			j--
		case 'X':
			ops = append(ops, 'D')
			aligned++
			state = traceX[idx]
			i--
		case 'Y':
			ops = append(ops, 'I')
			aligned++
			state = traceY[idx]
			j--
		default:
			return globalAlignment{}, fmt.Errorf("unexpected global traceback state %q", state)
		}
	}
	slices.Reverse(ops)

	return globalAlignment{
		ops:     ops,
		score:   best,
		matches: matches,
		aligned: aligned,
	}, nil
}

func suffixAlign(query, ref []byte, scoring Scoring, xdrop int) (endAlignment, error) {
	if len(query) == 0 && len(ref) == 0 {
		return endAlignment{}, nil
	}
	rows := len(query) + 1
	cols := len(ref) + 1
	mm := make([]int, rows*cols)
	ix := make([]int, rows*cols)
	iy := make([]int, rows*cols)
	traceM := make([]byte, rows*cols)
	traceX := make([]byte, rows*cols)
	traceY := make([]byte, rows*cols)

	for idx := range mm {
		mm[idx], ix[idx], iy[idx] = negInf, negInf, negInf
	}
	for i := 0; i < rows; i++ {
		mm[i*cols] = 0
	}
	for j := 0; j < cols; j++ {
		mm[j] = 0
	}

	bestScore := 0
	bestI, bestJ := rows-1, cols-1
	bestState := byte(0)

	for i := 1; i < rows; i++ {
		rowBest := negInf
		for j := 1; j < cols; j++ {
			idx := i*cols + j
			diagIdx := (i-1)*cols + (j - 1)
			upIdx := (i-1)*cols + j
			leftIdx := i*cols + (j - 1)

			bestPrev := 0
			traceM[idx] = 0
			if mm[diagIdx] > bestPrev {
				bestPrev = mm[diagIdx]
				traceM[idx] = 'M'
			}
			if ix[diagIdx] > bestPrev {
				bestPrev = ix[diagIdx]
				traceM[idx] = 'X'
			}
			if iy[diagIdx] > bestPrev {
				bestPrev = iy[diagIdx]
				traceM[idx] = 'Y'
			}
			mm[idx] = bestPrev + scorePair(query[i-1], ref[j-1], scoring)

			bestX := 0
			traceX[idx] = 0
			openX := mm[upIdx] + scoring.GapOpen
			if openX > bestX {
				bestX = openX
				traceX[idx] = 'M'
			}
			extendX := ix[upIdx] + scoring.GapExtend
			if extendX > bestX {
				bestX = extendX
				traceX[idx] = 'X'
			}
			ix[idx] = bestX

			bestY := 0
			traceY[idx] = 0
			openY := mm[leftIdx] + scoring.GapOpen
			if openY > bestY {
				bestY = openY
				traceY[idx] = 'M'
			}
			extendY := iy[leftIdx] + scoring.GapExtend
			if extendY > bestY {
				bestY = extendY
				traceY[idx] = 'Y'
			}
			iy[idx] = bestY

			cellBest := mm[idx]
			cellState := byte('M')
			if ix[idx] > cellBest {
				cellBest = ix[idx]
				cellState = 'X'
			}
			if iy[idx] > cellBest {
				cellBest = iy[idx]
				cellState = 'Y'
			}
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

	i, j := bestI, bestJ
	ops := make([]byte, 0, i+j)
	matches := 0
	aligned := 0
	state := bestState
	for i > 0 && j > 0 {
		idx := i*cols + j
		current := mm[idx]
		if state == 'X' {
			current = ix[idx]
		} else if state == 'Y' {
			current = iy[idx]
		}
		if current == 0 || state == 0 {
			break
		}
		switch state {
		case 'M':
			ops = append(ops, 'M')
			if query[i-1] == ref[j-1] {
				matches++
			}
			aligned++
			state = traceM[idx]
			i--
			j--
		case 'X':
			ops = append(ops, 'D')
			aligned++
			state = traceX[idx]
			i--
		case 'Y':
			ops = append(ops, 'I')
			aligned++
			state = traceY[idx]
			j--
		default:
			return endAlignment{}, fmt.Errorf("unexpected suffix traceback state %q", state)
		}
	}
	slices.Reverse(ops)
	return endAlignment{
		ops:       ops,
		score:     bestScore,
		matches:   matches,
		aligned:   aligned,
		queryClip: i,
		refClip:   j + (len(ref) - bestJ),
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
