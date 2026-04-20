package mapping_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinghunt/faqt/mapping"
	"github.com/martinghunt/faqt/minimizer"
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

func TestBuildFromReaderAndMapRecord(t *testing.T) {
	refs := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "ref1", Seq: []byte("TTTTACGTTGCAGGGG")},
		},
	}

	m, err := mapping.BuildFromReader(refs, minimizer.Options{
		K:         3,
		W:         2,
		MidOcc:    10,
		MaxMaxOcc: 20,
		OccDist:   500,
		QOccFrac:  0.01,
	})
	if err != nil {
		t.Fatalf("BuildFromReader() error = %v", err)
	}

	result, err := m.MapRecord(&seqio.SeqRecord{Name: "q1", Seq: []byte("ACGTTGCA")})
	if err != nil {
		t.Fatalf("MapRecord() error = %v", err)
	}

	if result.QueryName != "q1" {
		t.Fatalf("MapRecord() query name = %q, want q1", result.QueryName)
	}
	if len(result.Hits) == 0 {
		t.Fatal("MapRecord() returned no hits")
	}
	if result.Hits[0].RefName != "ref1" {
		t.Fatalf("MapRecord() top ref = %q, want ref1", result.Hits[0].RefName)
	}
	if result.Hits[0].Alignment.Score <= 0 {
		t.Fatalf("MapRecord() alignment score = %d, want > 0", result.Hits[0].Alignment.Score)
	}
}

func TestMapWithoutAlignerReturnsCandidatesOnly(t *testing.T) {
	refs := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "ref1", Seq: []byte("TTTTACGTTGCAGGGG")},
		},
	}

	m, err := mapping.BuildFromReader(refs, minimizer.Options{
		K:         3,
		W:         2,
		MidOcc:    10,
		MaxMaxOcc: 20,
		OccDist:   500,
		QOccFrac:  0.01,
	})
	if err != nil {
		t.Fatalf("BuildFromReader() error = %v", err)
	}
	m.Aligner = nil

	result, err := m.Map("q1", []byte("ACGTTGCA"))
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Hits) == 0 {
		t.Fatal("Map() returned no hits")
	}
	if result.Hits[0].Candidate.RefName != "ref1" {
		t.Fatalf("Map() candidate ref = %q, want ref1", result.Hits[0].Candidate.RefName)
	}
	if result.Hits[0].Alignment.Score != 0 {
		t.Fatalf("Map() alignment score with nil aligner = %d, want 0", result.Hits[0].Alignment.Score)
	}
}

func TestMapAll(t *testing.T) {
	refs := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "ref1", Seq: []byte("TTTTACGTTGCAGGGG")},
		},
	}
	m, err := mapping.BuildFromReader(refs, minimizer.Options{
		K:         3,
		W:         2,
		MidOcc:    10,
		MaxMaxOcc: 20,
		OccDist:   500,
		QOccFrac:  0.01,
	})
	if err != nil {
		t.Fatalf("BuildFromReader() error = %v", err)
	}

	queries := &sliceReader{
		records: []seqio.SeqRecord{
			{Name: "q1", Seq: []byte("ACGTTGCA")},
			{Name: "q2", Seq: []byte("TTTT")},
		},
	}
	results, err := m.MapAll(queries)
	if err != nil {
		t.Fatalf("MapAll() error = %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("MapAll() returned %d results, want 2", len(results))
	}
	if results[0].QueryName != "q1" || results[1].QueryName != "q2" {
		t.Fatalf("MapAll() query names = %q, %q", results[0].QueryName, results[1].QueryName)
	}
}

func TestBuildFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ref.fa")
	if err := os.WriteFile(path, []byte(">ref1\nTTTTACGTTGCAGGGG\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	m, err := mapping.BuildFromPath(path, minimizer.Options{
		K:         3,
		W:         2,
		MidOcc:    10,
		MaxMaxOcc: 20,
		OccDist:   500,
		QOccFrac:  0.01,
	})
	if err != nil {
		t.Fatalf("BuildFromPath() error = %v", err)
	}

	result, err := m.Map("q1", []byte("ACGTTGCA"))
	if err != nil {
		t.Fatalf("Map() error = %v", err)
	}
	if len(result.Hits) == 0 {
		t.Fatal("Map() returned no hits")
	}
}
