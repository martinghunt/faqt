package mapper_test

import (
	"io"
	"testing"

	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seqio"
)

func TestDiagonalClustererGroupsByReferenceStrandAndBand(t *testing.T) {
	anchors := []minimizer.Anchor{
		{RefID: 0, QueryPos: 10, RefPos: 12, QueryStrand: 0, RefStrand: 0},
		{RefID: 0, QueryPos: 20, RefPos: 22, QueryStrand: 0, RefStrand: 0},
		{RefID: 0, QueryPos: 30, RefPos: 60, QueryStrand: 0, RefStrand: 0},
		{RefID: 1, QueryPos: 10, RefPos: 12, QueryStrand: 0, RefStrand: 0},
		{RefID: 0, QueryPos: 15, RefPos: 40, QueryStrand: 0, RefStrand: 1},
		{RefID: 0, QueryPos: 25, RefPos: 30, QueryStrand: 0, RefStrand: 1},
	}

	clusterer := mapper.DiagonalClusterer{
		Options: mapper.ClusterOptions{
			DiagonalBand: 10,
			MinAnchors:   2,
		},
	}
	clusters, err := clusterer.Cluster(anchors)
	if err != nil {
		t.Fatalf("Cluster() error = %v", err)
	}

	if len(clusters) != 2 {
		t.Fatalf("Cluster() returned %d clusters, want 2", len(clusters))
	}
	if clusters[0].RefID != 0 || len(clusters[0].Anchors) != 2 {
		t.Fatalf("Cluster() first cluster = %#v", clusters[0])
	}
	if clusters[1].RelativeStrand != 1 || len(clusters[1].Anchors) != 2 {
		t.Fatalf("Cluster() second cluster = %#v", clusters[1])
	}
}

func TestGreedyChainerSplitsOnLargeGap(t *testing.T) {
	clusters := []mapper.Cluster{
		{
			RefID:          0,
			RelativeStrand: 0,
			Anchors: []minimizer.Anchor{
				{RefID: 0, QueryPos: 10, RefPos: 10, QueryStrand: 0, RefStrand: 0},
				{RefID: 0, QueryPos: 20, RefPos: 20, QueryStrand: 0, RefStrand: 0},
				{RefID: 0, QueryPos: 200, RefPos: 200, QueryStrand: 0, RefStrand: 0},
				{RefID: 0, QueryPos: 210, RefPos: 210, QueryStrand: 0, RefStrand: 0},
			},
		},
	}

	chainer := mapper.GreedyChainer{
		Options: mapper.ChainOptions{
			MaxGap:           50,
			MaxDiagonalDrift: 10,
			MinAnchors:       2,
			MinScore:         2,
			GapPenalty:       1,
			OccurrenceWeight: 1,
		},
	}
	chains, err := chainer.Chain(clusters)
	if err != nil {
		t.Fatalf("Chain() error = %v", err)
	}

	if len(chains) != 2 {
		t.Fatalf("Chain() returned %d chains, want 2", len(chains))
	}
	if chains[0].Score != 2 || chains[1].Score != 2 {
		t.Fatalf("Chain() scores = %d, %d; want 2,2", chains[0].Score, chains[1].Score)
	}
}

func TestGreedyChainerHandlesReverseOrientation(t *testing.T) {
	clusters := []mapper.Cluster{
		{
			RefID:          0,
			RelativeStrand: 1,
			Anchors: []minimizer.Anchor{
				{RefID: 0, QueryPos: 10, RefPos: 90, QueryStrand: 0, RefStrand: 1},
				{RefID: 0, QueryPos: 20, RefPos: 80, QueryStrand: 0, RefStrand: 1},
				{RefID: 0, QueryPos: 30, RefPos: 70, QueryStrand: 0, RefStrand: 1},
			},
		},
	}

	chainer := mapper.GreedyChainer{
		Options: mapper.ChainOptions{
			MaxGap:           20,
			MaxDiagonalDrift: 20,
			MinAnchors:       2,
			MinScore:         2,
			GapPenalty:       1,
			OccurrenceWeight: 1,
		},
	}
	chains, err := chainer.Chain(clusters)
	if err != nil {
		t.Fatalf("Chain() error = %v", err)
	}

	if len(chains) != 1 {
		t.Fatalf("Chain() returned %d chains, want 1", len(chains))
	}
	if chains[0].RelativeStrand != 1 || chains[0].Score != 3 {
		t.Fatalf("Chain() reverse chain = %#v", chains[0])
	}
}

func TestPipelineUsesInjectedStrategies(t *testing.T) {
	pipeline := mapper.DefaultPipeline()
	anchors := []minimizer.Anchor{
		{RefID: 0, QueryPos: 10, RefPos: 10, QueryStrand: 0, RefStrand: 0},
		{RefID: 0, QueryPos: 20, RefPos: 20, QueryStrand: 0, RefStrand: 0},
		{RefID: 0, QueryPos: 30, RefPos: 30, QueryStrand: 0, RefStrand: 0},
	}

	chains, err := pipeline.Process(anchors)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if len(chains) != 1 {
		t.Fatalf("Process() returned %d chains, want 1", len(chains))
	}
	if chains[0].Score != 3 {
		t.Fatalf("Process() chain score = %d, want 3", chains[0].Score)
	}
}

func TestRankChainsOrdersByScoreThenSpan(t *testing.T) {
	chains := []mapper.Chain{
		{RefID: 0, Score: 3, AnchorCount: 3, QuerySpan: 20, RefSpan: 20},
		{RefID: 1, Score: 5, AnchorCount: 2, QuerySpan: 10, RefSpan: 10},
		{RefID: 2, Score: 5, AnchorCount: 2, QuerySpan: 30, RefSpan: 30},
	}

	ranked := mapper.RankChains(chains)
	if ranked[0].RefID != 2 || ranked[1].RefID != 1 || ranked[2].RefID != 0 {
		t.Fatalf("RankChains() order = %#v", ranked)
	}
}

func TestCandidateFromChainExtractsForwardWindows(t *testing.T) {
	ref := seqio.SeqRecord{Name: "ref1", Seq: []byte("AACCGGTTAACC")}
	query := []byte("CCGGTTAA")
	chain := mapper.Chain{
		RefID:          0,
		RelativeStrand: 0,
		QueryStart:     1,
		QueryEnd:       4,
		RefStart:       2,
		RefEnd:         5,
	}

	candidate, err := mapper.CandidateFromChain(ref, query, 3, chain, mapper.CandidateOptions{
		QueryPadding: 1,
		RefPadding:   2,
	})
	if err != nil {
		t.Fatalf("CandidateFromChain() error = %v", err)
	}

	if string(candidate.QuerySeq) != "CCGGTTAA" {
		t.Fatalf("CandidateFromChain() query seq = %q", candidate.QuerySeq)
	}
	if string(candidate.RefSeqForward) != "AACCGGTTAA" {
		t.Fatalf("CandidateFromChain() forward ref seq = %q", candidate.RefSeqForward)
	}
	if string(candidate.RefSeqOriented) != "AACCGGTTAA" {
		t.Fatalf("CandidateFromChain() oriented ref seq = %q", candidate.RefSeqOriented)
	}
}

func TestCandidateFromChainReverseComplementsReference(t *testing.T) {
	ref := seqio.SeqRecord{Name: "ref1", Seq: []byte("AACCGGTTAACC")}
	query := []byte("GGTT")
	chain := mapper.Chain{
		RefID:          0,
		RelativeStrand: 1,
		QueryStart:     0,
		QueryEnd:       1,
		RefStart:       4,
		RefEnd:         5,
	}

	candidate, err := mapper.CandidateFromChain(ref, query, 2, chain, mapper.CandidateOptions{})
	if err != nil {
		t.Fatalf("CandidateFromChain() error = %v", err)
	}

	if string(candidate.RefSeqForward) != "GGT" {
		t.Fatalf("CandidateFromChain() forward ref seq = %q", candidate.RefSeqForward)
	}
	if string(candidate.RefSeqOriented) != "ACC" {
		t.Fatalf("CandidateFromChain() oriented ref seq = %q", candidate.RefSeqOriented)
	}
}

func TestExtractCandidatesLimitsAndRanks(t *testing.T) {
	index := &minimizer.Index{
		K: 3,
		Refs: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("AACCGGTTAACC")},
			{Name: "r2", Seq: []byte("TTTTACGTACGT")},
		},
	}
	chains := []mapper.Chain{
		{RefID: 0, RelativeStrand: 0, QueryStart: 0, QueryEnd: 2, RefStart: 2, RefEnd: 4, Score: 3, AnchorCount: 2, QuerySpan: 3, RefSpan: 3},
		{RefID: 1, RelativeStrand: 0, QueryStart: 0, QueryEnd: 3, RefStart: 4, RefEnd: 7, Score: 6, AnchorCount: 3, QuerySpan: 4, RefSpan: 4},
	}

	candidates, err := mapper.ExtractCandidates(index, []byte("ACGTAC"), chains, mapper.CandidateOptions{MaxCandidates: 1})
	if err != nil {
		t.Fatalf("ExtractCandidates() error = %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("ExtractCandidates() count = %d, want 1", len(candidates))
	}
	if candidates[0].RefName != "r2" {
		t.Fatalf("ExtractCandidates() top ref = %q, want r2", candidates[0].RefName)
	}
}

func TestMapProducesCandidates(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "ref1", Seq: []byte("ACGTTGCAACGTTGCA")},
		},
	}
	index, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 10, MaxMaxOcc: 20, OccDist: 500, QOccFrac: 0.01})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	candidates, err := mapper.Map(index, []byte("ACGTTGCA"), mapper.DefaultPipeline(), mapper.CandidateOptions{MaxCandidates: 3})
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}

	if len(candidates) == 0 {
		t.Fatal("Map() returned no candidates")
	}
	if candidates[0].RefName != "ref1" {
		t.Fatalf("Map() top candidate ref = %q, want ref1", candidates[0].RefName)
	}
}

type sliceReader struct {
	records []seqio.SeqRecord
	index   int
}

func (r *sliceReader) Read() (*seqio.SeqRecord, error) {
	if r.index >= len(r.records) {
		return nil, io.EOF
	}
	rec := r.records[r.index]
	r.index++
	return &rec, nil
}
