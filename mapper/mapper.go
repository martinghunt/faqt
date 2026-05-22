package mapper

import (
	"fmt"
	"slices"

	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seq"
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

		candidate, err := CandidateFromChain(ref, query, index.K, chain, opts)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
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
