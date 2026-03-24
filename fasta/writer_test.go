package fasta

import (
	"bytes"
	"testing"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

func TestWriteRecord(t *testing.T) {
	var buf bytes.Buffer
	rec := seqrecord.SeqRecord{Name: "r1", Description: "desc", Seq: []byte("ACGT")}
	if err := WriteRecord(&buf, rec, 3); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if got := buf.String(); got != ">r1 desc\nACG\nT\n" {
		t.Fatalf("WriteRecord() = %q", got)
	}
}

func TestMustFormat(t *testing.T) {
	rec := seqrecord.SeqRecord{Name: "r1", Seq: []byte("ACGT")}
	if got := MustFormat(rec, 2); got != ">r1\nAC\nGT\n" {
		t.Fatalf("MustFormat() = %q", got)
	}
}
