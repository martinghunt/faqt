package fastq

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderMultiRecord(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("@r1 desc\nACGT\n+\nABCD\n@r2\nNNNN\n+\n!!!!\n")))

	first, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if first.Name != "r1" || first.Description != "desc" || string(first.Seq) != "ACGT" || string(first.Qual) != "ABCD" {
		t.Fatalf("first record = %+v", first)
	}

	second, err := r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if second.Name != "r2" || string(second.Seq) != "NNNN" || string(second.Qual) != "!!!!" {
		t.Fatalf("second record = %+v", second)
	}
	if string(first.Seq) != "ACGT" || string(first.Qual) != "ABCD" {
		t.Fatalf("first record changed after reading second record: %+v", first)
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
