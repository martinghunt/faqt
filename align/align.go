package align

import (
	"fmt"
	"slices"

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
