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
