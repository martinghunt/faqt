package minimizer_test

import (
	"io"
	"reflect"
	"slices"
	"testing"

	"github.com/martinghunt/faqt/minimizer"
	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

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

func TestSketchCanonicalMatchesReverseComplement(t *testing.T) {
	seq1 := []byte("ACGTTGCA")
	seq2 := seq.ReverseComplement(seq1)

	got1 := minimizer.Sketch(seq1, 3, 2)
	got2 := minimizer.Sketch(seq2, 3, 2)

	hashes1 := hashes(got1)
	hashes2 := hashes(got2)
	slices.Sort(hashes1)
	slices.Sort(hashes2)
	if !reflect.DeepEqual(unique(hashes1), unique(hashes2)) {
		t.Fatalf("Sketch() hashes differ for reverse complements: %v vs %v", hashes1, hashes2)
	}
}

func TestSketchSkipsAmbiguousBases(t *testing.T) {
	got := minimizer.Sketch([]byte("AAANAAAA"), 3, 2)
	if len(got) != 1 {
		t.Fatalf("Sketch() after ambiguous base = %#v, want one post-N minimizer", got)
	}
	if got[0].Pos != 5 {
		t.Fatalf("Sketch() minimizer position after ambiguous base = %d, want 5", got[0].Pos)
	}
}

func TestSketchSkipsPalindromicKMers(t *testing.T) {
	got := minimizer.Sketch([]byte("ATATAT"), 2, 2)
	if len(got) != 0 {
		t.Fatalf("Sketch() with palindromic k-mers = %#v, want none", got)
	}
}

func TestBuildIndexesRecordsAndCopiesSequences(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("ACGTTGCA")},
			{Name: "r2", Seq: []byte("TTTACGTA")},
		},
	}

	idx, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(idx.Refs) != 2 {
		t.Fatalf("Build() indexed %d refs, want 2", len(idx.Refs))
	}
	if idx.TotalMinimizers == 0 {
		t.Fatal("Build() produced no minimizers")
	}

	firstHash := minimizer.Sketch([]byte("ACGTTGCA"), 3, 2)[0].Hash
	hits := idx.Lookup(firstHash)
	if len(hits) == 0 {
		t.Fatalf("Lookup(%d) returned no hits", firstHash)
	}
	if hits[0].RefID != 0 {
		t.Fatalf("Lookup(%d) first hit RefID = %d, want 0", firstHash, hits[0].RefID)
	}

	reader.records[0].Seq[0] = 'T'
	if string(idx.Refs[0].Seq) != "ACGTTGCA" {
		t.Fatalf("Build() kept alias to input sequence: %q", idx.Refs[0].Seq)
	}
}

func TestBuildRejectsInvalidOptions(t *testing.T) {
	reader := &sliceReader{}
	if _, err := minimizer.Build(reader, minimizer.Options{K: 0, W: 2}); err == nil {
		t.Fatal("Build() expected error for invalid k")
	}
	if _, err := minimizer.Build(reader, minimizer.Options{K: 4, W: 256}); err == nil {
		t.Fatal("Build() expected error for invalid w")
	}
	if _, err := minimizer.Build(reader, minimizer.Options{K: 4, W: 2, MidOcc: -1}); err == nil {
		t.Fatal("Build() expected error for invalid MidOcc")
	}
	if _, err := minimizer.Build(reader, minimizer.Options{K: 4, W: 2, MidOcc: 10, MaxMaxOcc: 9}); err == nil {
		t.Fatal("Build() expected error for MaxMaxOcc < MidOcc")
	}
	if _, err := minimizer.Build(reader, minimizer.Options{K: 4, W: 2, OccDist: -1}); err == nil {
		t.Fatal("Build() expected error for invalid OccDist")
	}
	if _, err := minimizer.Build(reader, minimizer.Options{K: 4, W: 2, QOccFrac: 1.1}); err == nil {
		t.Fatal("Build() expected error for invalid QOccFrac")
	}
}

func TestQueryReturnsAnchorsForRetainedMinimizers(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("ACGTTGCA")},
			{Name: "r2", Seq: []byte("TTTACGTA")},
		},
	}

	idx, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 10, MaxMaxOcc: 20, OccDist: 500, QOccFrac: 0.01})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	anchors := idx.Query([]byte("ACGTTGCA"))
	if len(anchors) == 0 {
		t.Fatal("Query() returned no anchors")
	}

	foundSelfHit := false
	for _, anchor := range anchors {
		if anchor.RefID == 0 {
			foundSelfHit = true
			break
		}
	}
	if !foundSelfHit {
		t.Fatalf("Query() anchors did not include reference self hit: %#v", anchors)
	}
}

func TestBuildKeepsHighOccurrenceMinimizersInIndex(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("AAAAAAAAAA")},
		},
	}

	idx, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 1, MaxMaxOcc: 4, OccDist: 2})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if len(idx.Table) == 0 {
		t.Fatal("Build() dropped high-occurrence minimizers from the index")
	}
	if idx.TotalMinimizers == 0 {
		t.Fatal("Build() recorded no minimizers")
	}
}

func TestQueryFiltersHighOccurrenceMinimizersWithoutRescue(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("AAAAAAAAAA")},
		},
	}

	idx, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 1, MaxMaxOcc: 1, OccDist: 0})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if anchors := idx.Query([]byte("AAAAAAAAAA")); len(anchors) != 0 {
		t.Fatalf("Query() returned anchors for high-occurrence minimizers without rescue: %#v", anchors)
	}
}

func TestQueryRescuesHighOccurrenceMinimizers(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("AAAAAAAAAA")},
		},
	}

	idx, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 1, MaxMaxOcc: 10, OccDist: 2})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	anchors := idx.Query([]byte("AAAAAAAAAA"))
	if len(anchors) == 0 {
		t.Fatal("Query() did not rescue any high-occurrence minimizers")
	}
	for _, anchor := range anchors {
		if anchor.Occurrence <= 1 {
			t.Fatalf("Query() rescue returned non-repetitive anchor: %#v", anchor)
		}
	}
}

func TestQueryFiltersOverrepresentedQueryMinimizers(t *testing.T) {
	reader := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "r1", Seq: []byte("AAAAAAAAAAAAAAAA")},
			{Name: "r2", Seq: []byte("ACGTTGCAACGTTGCA")},
		},
	}

	idx, err := minimizer.Build(reader, minimizer.Options{K: 3, W: 2, MidOcc: 1, MaxMaxOcc: 10, OccDist: 0, QOccFrac: 0.2})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if anchors := idx.Query([]byte("AAAAAAAAAAAAAAAA")); len(anchors) != 0 {
		t.Fatalf("Query() kept overrepresented query minimizers: %#v", anchors)
	}
}

func hashes(mins []minimizer.Minimizer) []uint64 {
	out := make([]uint64, len(mins))
	for i := range mins {
		out[i] = mins[i].Hash
	}
	return out
}

func unique(in []uint64) []uint64 {
	if len(in) == 0 {
		return nil
	}
	out := []uint64{in[0]}
	for i := 1; i < len(in); i++ {
		if in[i] != in[i-1] {
			out = append(out, in[i])
		}
	}
	return out
}
