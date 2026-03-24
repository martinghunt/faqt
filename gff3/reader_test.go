package gff3

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderReadsFASTASection(t *testing.T) {
	input := "##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n##FASTA\n>chr1 desc\nACGT\n>chr2\nNNNN\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "chr1" || rec.Description != "desc" || string(rec.Seq) != "ACGT" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "chr2" || string(rec.Seq) != "NNNN" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

func TestReaderErrorsWithoutFASTA(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n")))

	_, err := r.Read()
	if err == nil || !strings.Contains(err.Error(), "does not contain ##FASTA") {
		t.Fatalf("Read() error = %v, want missing FASTA error", err)
	}
}
