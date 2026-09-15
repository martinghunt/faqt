package phylip

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderReadsSequentialPhylip(t *testing.T) {
	r, err := NewReader(bufio.NewReader(strings.NewReader("2 6\nseq1 ACGTAA\nseq2 NN--TT\n")))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "seq1" || string(rec.Seq) != "ACGTAA" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "seq2" || string(rec.Seq) != "NN--TT" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

func TestReaderReadsWrappedSequentialPhylip(t *testing.T) {
	input := "2 8\nseq1 ACGT\nAA--\nseq2 TT\nAA\nCCGG\n"
	r, err := NewReader(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "seq1" || string(rec.Seq) != "ACGTAA--" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "seq2" || string(rec.Seq) != "TTAACCGG" {
		t.Fatalf("second record = %+v", rec)
	}
}

func TestReaderErrorsOnInterleavedPhylipInsteadOfCorruptingSequence(t *testing.T) {
	// Two taxa, each with a 20-character sequence split across two
	// interleaved blocks. A sequential-format reader must not silently
	// splice "seq2"'s name and sequence into seq1's record.
	input := "2 20\n" +
		"seq1 ACGTACGTAC\n" +
		"seq2 TTGGTTGGTT\n" +
		"\n" +
		"GTACGTACGT\n" +
		"GGTTGGTTGG\n"
	r, err := NewReader(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	_, err = r.Read()
	if err == nil || !strings.Contains(err.Error(), "interleaved PHYLIP is not supported") {
		t.Fatalf("Read() error = %v, want interleaved-PHYLIP error", err)
	}
}

func TestReaderErrorsOnTruncatedWrappedPhylip(t *testing.T) {
	r, err := NewReader(bufio.NewReader(strings.NewReader("1 8\nseq1 ACGT\nAA\n")))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	_, err = r.Read()
	if err == nil || !strings.Contains(err.Error(), "unexpected end") {
		t.Fatalf("Read() error = %v, want truncated sequence error", err)
	}
}
