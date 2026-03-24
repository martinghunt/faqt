package fasta

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderMultiRecord(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader(">seq1 desc one\nACG\nT\n>seq2\nNN-\n")))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "seq1" || rec.Description != "desc one" || string(rec.Seq) != "ACGT" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "seq2" || string(rec.Seq) != "NN-" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}
