package align

import (
	"fmt"
	"slices"
	"strings"

	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/seq"
)

type Scoring struct {
	Match    int
	Mismatch int
	Gap      int
}

type Options struct {
	Scoring       Scoring
	MaxAlignments int
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

func DefaultAligner() SmithWatermanAligner {
	return SmithWatermanAligner{
		Options: Options{
			Scoring: Scoring{
				Match:    2,
				Mismatch: -4,
				Gap:      -4,
			},
			MaxAlignments: 5,
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
	if err := a.Options.validate(); err != nil {
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
	scores := make([]int, rows*cols)
	trace := make([]byte, rows*cols)

	bestScore := 0
	bestI, bestJ := 0, 0
	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			diag := scores[(i-1)*cols+(j-1)] + scorePair(query[i-1], ref[j-1], a.Options.Scoring)
			up := scores[(i-1)*cols+j] + a.Options.Scoring.Gap
			left := scores[i*cols+(j-1)] + a.Options.Scoring.Gap

			score := 0
			dir := byte(0)
			if diag > score {
				score = diag
				dir = 'M'
			}
			if up > score {
				score = up
				dir = 'D'
			}
			if left > score {
				score = left
				dir = 'I'
			}

			scores[i*cols+j] = score
			trace[i*cols+j] = dir
			if score > bestScore {
				bestScore = score
				bestI, bestJ = i, j
			}
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
	for i > 0 && j > 0 {
		dir := trace[i*cols+j]
		if dir == 0 || scores[i*cols+j] == 0 {
			break
		}
		switch dir {
		case 'M':
			ops = append(ops, 'M')
			if query[i-1] == ref[j-1] {
				matches++
			}
			aligned++
			i--
			j--
		case 'D':
			ops = append(ops, 'D')
			aligned++
			i--
		case 'I':
			ops = append(ops, 'I')
			aligned++
			j--
		default:
			return Result{}, fmt.Errorf("unexpected traceback operation %q", dir)
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
	return nil
}

func (s Scoring) validate() error {
	if s.Match <= 0 {
		return fmt.Errorf("match score must be > 0")
	}
	if s.Mismatch >= 0 {
		return fmt.Errorf("mismatch score must be < 0")
	}
	if s.Gap >= 0 {
		return fmt.Errorf("gap score must be < 0")
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
