package align_test

import (
	"io"
	"testing"

	"github.com/martinghunt/faqt/align"
	"github.com/martinghunt/faqt/mapper"
	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

func TestSmithWatermanAlignerAlignsForwardCandidate(t *testing.T) {
	candidate := mapper.Candidate{
		RefName:        "ref1",
		QueryRange:     seq.Interval{Start: 0, End: 8},
		RefRange:       seq.Interval{Start: 2, End: 10},
		QuerySeq:       []byte("CCGGTTAA"),
		RefSeqForward:  []byte("CCGGTTAA"),
		RefSeqOriented: []byte("CCGGTTAA"),
		RelativeStrand: 0,
	}

	result, err := align.DefaultAligner().Align(candidate)
	if err != nil {
		t.Fatalf("Align() error = %v", err)
	}

	if result.Score <= 0 {
		t.Fatalf("Align() score = %d, want > 0", result.Score)
	}
	if result.CIGAR != "8M" {
		t.Fatalf("Align() CIGAR = %q, want 8M", result.CIGAR)
	}
	if result.QueryRange != (seq.Interval{Start: 0, End: 8}) {
		t.Fatalf("Align() query range = %#v", result.QueryRange)
	}
	if result.RefRangeForward != (seq.Interval{Start: 2, End: 10}) {
		t.Fatalf("Align() ref range = %#v", result.RefRangeForward)
	}
}

func TestSmithWatermanAlignerProjectsReverseCoordinates(t *testing.T) {
	candidate := mapper.Candidate{
		RefName:        "ref1",
		QueryRange:     seq.Interval{Start: 10, End: 14},
		RefRange:       seq.Interval{Start: 100, End: 104},
		QuerySeq:       []byte("ACGT"),
		RefSeqForward:  []byte("ACGT"),
		RefSeqOriented: []byte("ACGT"),
		RelativeStrand: 1,
	}

	result, err := align.DefaultAligner().Align(candidate)
	if err != nil {
		t.Fatalf("Align() error = %v", err)
	}

	if result.RefRangeForward != (seq.Interval{Start: 100, End: 104}) {
		t.Fatalf("Align() reverse ref range = %#v", result.RefRangeForward)
	}
}

func TestAlignCandidatesRanksResults(t *testing.T) {
	aligned := mapper.Candidate{
		RefName:        "ref1",
		QueryRange:     seq.Interval{Start: 0, End: 4},
		RefRange:       seq.Interval{Start: 0, End: 4},
		QuerySeq:       []byte("ACGT"),
		RefSeqForward:  []byte("ACGT"),
		RefSeqOriented: []byte("ACGT"),
		RelativeStrand: 0,
		Chain:          mapper.Chain{Score: 4},
	}
	weaker := mapper.Candidate{
		RefName:        "ref2",
		QueryRange:     seq.Interval{Start: 0, End: 4},
		RefRange:       seq.Interval{Start: 0, End: 4},
		QuerySeq:       []byte("ACGT"),
		RefSeqForward:  []byte("AGGT"),
		RefSeqOriented: []byte("AGGT"),
		RelativeStrand: 0,
		Chain:          mapper.Chain{Score: 1},
	}

	results, err := align.AlignCandidates(align.DefaultAligner(), []mapper.Candidate{weaker, aligned}, align.Options{
		Scoring:       align.DefaultAligner().Options.Scoring,
		MaxAlignments: 1,
	})
	if err != nil {
		t.Fatalf("AlignCandidates() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("AlignCandidates() count = %d, want 1", len(results))
	}
	if results[0].Candidate.RefName != "ref1" {
		t.Fatalf("AlignCandidates() top result ref = %q, want ref1", results[0].Candidate.RefName)
	}
}

func TestEndToEndMapAndAlignCandidate(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "ref1", Seq: []byte("TTTTACGTTGCAGGGG")},
		},
	}
	index, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 10, MaxMaxOcc: 20, OccDist: 500, QOccFrac: 0.01})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	candidates, err := mapper.Map(index, []byte("ACGTTGCA"), mapper.DefaultPipeline(), mapper.CandidateOptions{
		QueryPadding:  2,
		RefPadding:    2,
		MaxCandidates: 3,
	})
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("Map() returned no candidates")
	}

	results, err := align.AlignCandidates(align.DefaultAligner(), candidates, align.DefaultAligner().Options)
	if err != nil {
		t.Fatalf("AlignCandidates() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("AlignCandidates() returned no results")
	}
	if results[0].Score <= 0 {
		t.Fatalf("AlignCandidates() top score = %d, want > 0", results[0].Score)
	}
	if results[0].Candidate.RefName != "ref1" {
		t.Fatalf("AlignCandidates() top ref = %q, want ref1", results[0].Candidate.RefName)
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
