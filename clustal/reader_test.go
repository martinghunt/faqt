package clustal

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderReadsBlocks(t *testing.T) {
	input := "CLUSTAL W (1.83) multiple sequence alignment\n\nseq1    ACGT--\nseq2    AC-TGG\n        ** ..\n\nseq1    AA\nseq2    TT\n"
	r, err := NewReader(bufio.NewReader(strings.NewReader(input)))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "seq1" || string(rec.Seq) != "ACGT--AA" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "seq2" || string(rec.Seq) != "AC-TGGTT" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}
