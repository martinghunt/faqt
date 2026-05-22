package align

import (
	"fmt"
	"strings"

	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/seq"
)

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
