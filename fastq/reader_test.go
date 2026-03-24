package fastq

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderMultiRecord(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("@r1 desc\nACGT\n+\nABCD\n@r2\nNNNN\n+\n!!!!\n")))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "r1" || rec.Description != "desc" || string(rec.Seq) != "ACGT" || string(rec.Qual) != "ABCD" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "r2" || string(rec.Seq) != "NNNN" || string(rec.Qual) != "!!!!" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

func TestReaderRejectsMismatchedLengths(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("@r1\nACGT\n+\nABC\n")))

	_, err := r.Read()
	if err == nil || !strings.Contains(err.Error(), "sequence length 4 and quality length 3") {
		t.Fatalf("Read() error = %v, want length mismatch", err)
	}
}
