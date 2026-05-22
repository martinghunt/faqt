package genbank

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderMultiRecord(t *testing.T) {
	input := "LOCUS       REC1\nDEFINITION  first record\nORIGIN\n        1 acgt nn\n//\nLOCUS       REC2\nDEFINITION  second record\nORIGIN\n        1 ttaa\n//\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "REC1" || rec.Description != "first record" || string(rec.Seq) != "acgtnn" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "REC2" || rec.Description != "second record" || string(rec.Seq) != "ttaa" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

func TestReaderReadsDefinitionContinuation(t *testing.T) {
	input := "LOCUS       REC1\nDEFINITION  first line\n            second line\nACCESSION   X12345\nORIGIN\n        1 ACGT NN\n//\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Description != "first line second line" {
		t.Fatalf("description = %q, want continuation text", rec.Description)
	}
	if string(rec.Seq) != "acgtnn" {
		t.Fatalf("sequence = %q, want acgtnn", rec.Seq)
	}
}
