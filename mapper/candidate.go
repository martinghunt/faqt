package mapper

import (
	"fmt"

	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

func (o CandidateOptions) validate() error {
	if o.QueryPadding < 0 {
		return fmt.Errorf("query padding must be >= 0")
	}
	if o.RefPadding < 0 {
		return fmt.Errorf("reference padding must be >= 0")
	}
	if o.MaxCandidates < 0 {
		return fmt.Errorf("max candidates must be >= 0")
	}
	return nil
}

func paddedRange(start, end, length, padding int) seq.Interval {
	start -= padding
	if start < 0 {
		start = 0
	}
	end += padding
	if end > length {
		end = length
	}
	return seq.Interval{Start: start, End: end}
}

func CandidateFromChain(ref seqio.SeqRecord, query []byte, k int, chain Chain, opts CandidateOptions) (Candidate, error) {
	if err := opts.validate(); err != nil {
		return Candidate{}, err
	}
	queryRange := paddedRange(chain.QueryStart, chain.QueryEnd+k, len(query), opts.QueryPadding)
	refRange := paddedRange(chain.RefStart, chain.RefEnd+k, len(ref.Seq), opts.RefPadding)

	querySeq, err := seq.Subseq(query, queryRange.Start, queryRange.End)
	if err != nil {
		return Candidate{}, err
	}
	refSeqForward, err := seq.Subseq(ref.Seq, refRange.Start, refRange.End)
	if err != nil {
		return Candidate{}, err
	}
	refSeqOriented := append([]byte(nil), refSeqForward...)
	if chain.RelativeStrand == 1 {
		refSeqOriented = seq.ReverseComplement(refSeqOriented)
	}

	return Candidate{
		Chain:          chain,
		SeedLength:     k,
		RefName:        ref.Name,
		QueryRange:     queryRange,
		RefRange:       refRange,
		QuerySeq:       querySeq,
		RefSeqForward:  refSeqForward,
		RefSeqOriented: refSeqOriented,
		RelativeStrand: chain.RelativeStrand,
	}, nil
}
