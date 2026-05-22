package mapper

import (
	"fmt"

	"github.com/martinghunt/faqt/minimizer"
)

func (c GreedyChainer) Chain(clusters []Cluster) ([]Chain, error) {
	if err := c.Options.validate(); err != nil {
		return nil, err
	}
	if len(clusters) == 0 {
		return nil, nil
	}

	chains := make([]Chain, 0, len(clusters))
	for _, cluster := range clusters {
		if len(cluster.Anchors) == 0 {
			continue
		}
		start := 0
		for i := 1; i <= len(cluster.Anchors); i++ {
			if i == len(cluster.Anchors) ||
				!chainCompatible(cluster.Anchors[i-1], cluster.Anchors[i], c.Options.MaxGap, c.Options.MaxDiagonalDrift) ||
				shouldSplitTransition(cluster.Anchors[i-1], cluster.Anchors[i], c.Options) {
				if i-start >= c.Options.MinAnchors {
					chain := makeChain(cluster.RefID, cluster.RelativeStrand, cluster.Anchors[start:i])
					chain.Score = scoreChain(chain.Anchors, c.Options)
					if chain.Score >= c.Options.MinScore {
						chains = append(chains, chain)
					}
				}
				start = i
			}
		}
	}
	return RankChains(chains), nil
}

func (o ChainOptions) validate() error {
	if o.MaxGap < 0 {
		return fmt.Errorf("max gap must be >= 0")
	}
	if o.MaxDiagonalDrift < 0 {
		return fmt.Errorf("max diagonal drift must be >= 0")
	}
	if o.MinAnchors <= 0 {
		return fmt.Errorf("minimum anchors must be > 0")
	}
	if o.MinScore <= 0 {
		return fmt.Errorf("minimum score must be > 0")
	}
	if o.GapPenalty < 0 {
		return fmt.Errorf("gap penalty must be >= 0")
	}
	if o.OccurrenceWeight < 0 {
		return fmt.Errorf("occurrence weight must be >= 0")
	}
	if o.SplitGapDiff < 0 {
		return fmt.Errorf("split gap difference must be >= 0")
	}
	if o.SplitGapRatio < 0 {
		return fmt.Errorf("split gap ratio must be >= 0")
	}
	return nil
}

func chainCompatible(prev, next minimizer.Anchor, maxGap, maxDiagonalDrift int) bool {
	if prev.RefID != next.RefID || relStrand(prev) != relStrand(next) {
		return false
	}

	queryGap := next.QueryPos - prev.QueryPos
	if queryGap < 0 || queryGap > maxGap {
		return false
	}

	if relStrand(prev) == 0 {
		refGap := next.RefPos - prev.RefPos
		if refGap < 0 || refGap > maxGap {
			return false
		}
	} else {
		refGap := prev.RefPos - next.RefPos
		if refGap < 0 || refGap > maxGap {
			return false
		}
	}

	return abs(diagonalValue(next)-diagonalValue(prev)) <= maxDiagonalDrift
}

func shouldSplitTransition(prev, next minimizer.Anchor, opts ChainOptions) bool {
	queryGap := next.QueryPos - prev.QueryPos
	if queryGap < 0 {
		return true
	}

	var refGap int
	if relStrand(prev) == 0 {
		refGap = next.RefPos - prev.RefPos
	} else {
		refGap = prev.RefPos - next.RefPos
	}
	if refGap < 0 {
		return true
	}

	gapDiff := abs(queryGap - refGap)
	if opts.SplitGapDiff > 0 && gapDiff > opts.SplitGapDiff {
		return true
	}

	if opts.SplitGapRatio > 0 {
		shorter := minNonZero(queryGap, refGap)
		longer := maxInt(queryGap, refGap)
		if shorter == 0 {
			return longer > 0
		}
		if float64(longer)/float64(shorter) > opts.SplitGapRatio {
			return true
		}
	}

	return false
}

func makeChain(refID int, relativeStrand uint8, anchors []minimizer.Anchor) Chain {
	out := Chain{
		RefID:          refID,
		RelativeStrand: relativeStrand,
		Anchors:        append([]minimizer.Anchor(nil), anchors...),
		QueryStart:     anchors[0].QueryPos,
		QueryEnd:       anchors[0].QueryPos,
		RefStart:       anchors[0].RefPos,
		RefEnd:         anchors[0].RefPos,
		AnchorCount:    len(anchors),
	}
	for _, anchor := range anchors[1:] {
		if anchor.QueryPos < out.QueryStart {
			out.QueryStart = anchor.QueryPos
		}
		if anchor.QueryPos > out.QueryEnd {
			out.QueryEnd = anchor.QueryPos
		}
		if anchor.RefPos < out.RefStart {
			out.RefStart = anchor.RefPos
		}
		if anchor.RefPos > out.RefEnd {
			out.RefEnd = anchor.RefPos
		}
	}
	out.QuerySpan = out.QueryEnd - out.QueryStart + 1
	out.RefSpan = out.RefEnd - out.RefStart + 1
	return out
}

func scoreChain(anchors []minimizer.Anchor, opts ChainOptions) int {
	if len(anchors) == 0 {
		return 0
	}

	score := len(anchors)
	for i := 1; i < len(anchors); i++ {
		queryGap := anchors[i].QueryPos - anchors[i-1].QueryPos
		refGap := abs(anchors[i].RefPos - anchors[i-1].RefPos)
		gapCost := abs(queryGap - refGap)
		score -= gapCost * opts.GapPenalty
	}
	for _, anchor := range anchors {
		if anchor.Occurrence > 1 {
			score -= (anchor.Occurrence - 1) * opts.OccurrenceWeight
		}
	}
	if score < 1 {
		return 1
	}
	return score
}

func compareChains(a, b Chain) int {
	if a.Score != b.Score {
		return cmpInt(b.Score, a.Score)
	}
	if a.AnchorCount != b.AnchorCount {
		return cmpInt(b.AnchorCount, a.AnchorCount)
	}
	if a.QuerySpan != b.QuerySpan {
		return cmpInt(b.QuerySpan, a.QuerySpan)
	}
	if a.RefSpan != b.RefSpan {
		return cmpInt(b.RefSpan, a.RefSpan)
	}
	if a.RefID != b.RefID {
		return cmpInt(a.RefID, b.RefID)
	}
	return cmpInt(a.QueryStart, b.QueryStart)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minNonZero(a, b int) int {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}
