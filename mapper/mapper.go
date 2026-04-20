package mapper

import (
	"fmt"
	"slices"

	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

type Cluster struct {
	RefID          int
	RelativeStrand uint8
	Band           int
	Anchors        []minimizer.Anchor
	QueryStart     int
	QueryEnd       int
	RefStart       int
	RefEnd         int
}

type Chain struct {
	RefID          int
	RelativeStrand uint8
	Anchors        []minimizer.Anchor
	QueryStart     int
	QueryEnd       int
	RefStart       int
	RefEnd         int
	Score          int
	AnchorCount    int
	QuerySpan      int
	RefSpan        int
}

type ClusterOptions struct {
	DiagonalBand int
	MinAnchors   int
}

type ChainOptions struct {
	MaxGap           int
	MaxDiagonalDrift int
	MinAnchors       int
	MinScore         int
	GapPenalty       int
	OccurrenceWeight int
	SplitGapDiff     int
	SplitGapRatio    float64
}

type Candidate struct {
	Chain          Chain
	SeedLength     int
	RefName        string
	QueryRange     seq.Interval
	RefRange       seq.Interval
	QuerySeq       []byte
	RefSeqForward  []byte
	RefSeqOriented []byte
	RelativeStrand uint8
}

type CandidateOptions struct {
	QueryPadding  int
	RefPadding    int
	MaxCandidates int
}

type Clusterer interface {
	Cluster([]minimizer.Anchor) ([]Cluster, error)
}

type Chainer interface {
	Chain([]Cluster) ([]Chain, error)
}

type DiagonalClusterer struct {
	Options ClusterOptions
}

type GreedyChainer struct {
	Options ChainOptions
}

type Pipeline struct {
	Clusterer Clusterer
	Chainer   Chainer
}

func DefaultPipeline() Pipeline {
	return Pipeline{
		Clusterer: DiagonalClusterer{
			Options: ClusterOptions{
				DiagonalBand: 50,
				MinAnchors:   2,
			},
		},
		Chainer: GreedyChainer{
			Options: ChainOptions{
				MaxGap:           1000,
				MaxDiagonalDrift: 100,
				MinAnchors:       2,
				MinScore:         2,
				GapPenalty:       1,
				OccurrenceWeight: 0,
				SplitGapDiff:     200,
				SplitGapRatio:    3.0,
			},
		},
	}
}

func (p Pipeline) Process(anchors []minimizer.Anchor) ([]Chain, error) {
	if p.Clusterer == nil {
		return nil, fmt.Errorf("clusterer is required")
	}
	if p.Chainer == nil {
		return nil, fmt.Errorf("chainer is required")
	}
	clusters, err := p.Clusterer.Cluster(anchors)
	if err != nil {
		return nil, err
	}
	return p.Chainer.Chain(clusters)
}

func RankChains(chains []Chain) []Chain {
	ranked := append([]Chain(nil), chains...)
	slices.SortFunc(ranked, compareChains)
	return ranked
}

func ExtractCandidates(index *minimizer.Index, query []byte, chains []Chain, opts CandidateOptions) ([]Candidate, error) {
	if index == nil {
		return nil, fmt.Errorf("index is required")
	}
	if err := opts.validate(); err != nil {
		return nil, err
	}

	ranked := RankChains(chains)
	if opts.MaxCandidates > 0 && len(ranked) > opts.MaxCandidates {
		ranked = ranked[:opts.MaxCandidates]
	}

	candidates := make([]Candidate, 0, len(ranked))
	for _, chain := range ranked {
		if chain.RefID < 0 || chain.RefID >= len(index.Refs) {
			return nil, fmt.Errorf("chain reference id %d out of range", chain.RefID)
		}
		ref := index.Refs[chain.RefID]

		queryRange := paddedRange(chain.QueryStart, chain.QueryEnd+index.K, len(query), opts.QueryPadding)
		refRange := paddedRange(chain.RefStart, chain.RefEnd+index.K, len(ref.Seq), opts.RefPadding)

		querySeq, err := seq.Subseq(query, queryRange.Start, queryRange.End)
		if err != nil {
			return nil, err
		}
		refSeqForward, err := seq.Subseq(ref.Seq, refRange.Start, refRange.End)
		if err != nil {
			return nil, err
		}
		refSeqOriented := append([]byte(nil), refSeqForward...)
		if chain.RelativeStrand == 1 {
			refSeqOriented = seq.ReverseComplement(refSeqOriented)
		}

		candidates = append(candidates, Candidate{
			Chain:          chain,
			SeedLength:     index.K,
			RefName:        ref.Name,
			QueryRange:     queryRange,
			RefRange:       refRange,
			QuerySeq:       querySeq,
			RefSeqForward:  refSeqForward,
			RefSeqOriented: refSeqOriented,
			RelativeStrand: chain.RelativeStrand,
		})
	}
	return candidates, nil
}

func Map(index *minimizer.Index, query []byte, pipeline Pipeline, candidateOpts CandidateOptions) ([]Candidate, error) {
	if index == nil {
		return nil, fmt.Errorf("index is required")
	}
	anchors := index.Query(query)
	chains, err := pipeline.Process(anchors)
	if err != nil {
		return nil, err
	}
	return ExtractCandidates(index, query, chains, candidateOpts)
}

func (c DiagonalClusterer) Cluster(anchors []minimizer.Anchor) ([]Cluster, error) {
	if err := c.Options.validate(); err != nil {
		return nil, err
	}
	if len(anchors) == 0 {
		return nil, nil
	}

	sorted := append([]minimizer.Anchor(nil), anchors...)
	slices.SortFunc(sorted, compareAnchors)

	clusters := make([]Cluster, 0, len(sorted))
	start := 0
	for i := 1; i <= len(sorted); i++ {
		if i == len(sorted) || !sameClusterKey(sorted[start], sorted[i], c.Options.DiagonalBand) {
			if i-start >= c.Options.MinAnchors {
				clusters = append(clusters, makeCluster(sorted[start:i], c.Options.DiagonalBand))
			}
			start = i
		}
	}
	return clusters, nil
}

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

func (o ClusterOptions) validate() error {
	if o.DiagonalBand <= 0 {
		return fmt.Errorf("diagonal band must be > 0")
	}
	if o.MinAnchors <= 0 {
		return fmt.Errorf("minimum anchors must be > 0")
	}
	return nil
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

func compareAnchors(a, b minimizer.Anchor) int {
	if a.RefID != b.RefID {
		return cmpInt(a.RefID, b.RefID)
	}
	if relStrand(a) != relStrand(b) {
		return cmpInt(int(relStrand(a)), int(relStrand(b)))
	}
	da := diagonalValue(a)
	db := diagonalValue(b)
	if da != db {
		return cmpInt(da, db)
	}
	if a.QueryPos != b.QueryPos {
		return cmpInt(a.QueryPos, b.QueryPos)
	}
	if relStrand(a) == 0 {
		return cmpInt(a.RefPos, b.RefPos)
	}
	return cmpInt(b.RefPos, a.RefPos)
}

func sameClusterKey(a, b minimizer.Anchor, diagonalBand int) bool {
	return a.RefID == b.RefID &&
		relStrand(a) == relStrand(b) &&
		diagonalBucket(a, diagonalBand) == diagonalBucket(b, diagonalBand)
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

func makeCluster(anchors []minimizer.Anchor, diagonalBand int) Cluster {
	out := Cluster{
		RefID:          anchors[0].RefID,
		RelativeStrand: relStrand(anchors[0]),
		Band:           diagonalBucket(anchors[0], diagonalBand),
		Anchors:        append([]minimizer.Anchor(nil), anchors...),
		QueryStart:     anchors[0].QueryPos,
		QueryEnd:       anchors[0].QueryPos,
		RefStart:       anchors[0].RefPos,
		RefEnd:         anchors[0].RefPos,
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
	return out
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

func relStrand(anchor minimizer.Anchor) uint8 {
	return anchor.QueryStrand ^ anchor.RefStrand
}

func diagonalValue(anchor minimizer.Anchor) int {
	if relStrand(anchor) == 0 {
		return anchor.QueryPos - anchor.RefPos
	}
	return anchor.QueryPos + anchor.RefPos
}

func diagonalBucket(anchor minimizer.Anchor, diagonalBand int) int {
	return diagonalValue(anchor) / diagonalBand
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
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
