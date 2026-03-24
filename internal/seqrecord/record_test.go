package seqrecord

import (
	"bytes"
	"strings"
	"testing"
)

func TestHeader(t *testing.T) {
	tests := []struct {
		name string
		rec  SeqRecord
		want string
	}{
		{name: "name and description", rec: SeqRecord{Name: "r1", Description: "desc"}, want: "r1 desc"},
		{name: "name only", rec: SeqRecord{Name: "r1"}, want: "r1"},
		{name: "description only", rec: SeqRecord{Description: "desc"}, want: "desc"},
		{name: "empty", rec: SeqRecord{}, want: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rec.Header(); got != tc.want {
				t.Fatalf("Header() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestFASTAFormatting(t *testing.T) {
	rec := SeqRecord{Name: "r1", Description: "desc", Seq: []byte("ACGT")}
	if got := rec.String(); got != ">r1 desc\nACGT\n" {
		t.Fatalf("String() = %q", got)
	}
	if got := rec.FASTAString(3); got != ">r1 desc\nACG\nT\n" {
		t.Fatalf("FASTAString() = %q", got)
	}

	var buf bytes.Buffer
	n, err := rec.WriteFASTATo(&buf, 3)
	if err != nil {
		t.Fatalf("WriteFASTATo() error = %v", err)
	}
	if got := buf.String(); got != ">r1 desc\nACG\nT\n" {
		t.Fatalf("WriteFASTATo() output = %q", got)
	}
	if n != int64(len(buf.String())) {
		t.Fatalf("WriteFASTATo() bytes = %d, want %d", n, len(buf.String()))
	}
}

func TestWriteFASTAToEmptySequence(t *testing.T) {
	rec := SeqRecord{Name: "empty"}
	var buf bytes.Buffer
	if _, err := rec.WriteFASTATo(&buf, 4); err != nil {
		t.Fatalf("WriteFASTATo() error = %v", err)
	}
	if got := buf.String(); got != ">empty\n\n" {
		t.Fatalf("WriteFASTATo() output = %q", got)
	}
}

func TestFASTQFormattingAndValidation(t *testing.T) {
	rec := SeqRecord{Name: "r1", Description: "desc", Seq: []byte("ACGT"), Qual: []byte("!!!!")}

	var buf bytes.Buffer
	n, err := rec.WriteTo(&buf)
	if err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	if got := buf.String(); got != "@r1 desc\nACGT\n+\n!!!!\n" {
		t.Fatalf("WriteTo() output = %q", got)
	}
	if n != int64(len(buf.String())) {
		t.Fatalf("WriteTo() bytes = %d, want %d", n, len(buf.String()))
	}

	bad := SeqRecord{Name: "bad", Seq: []byte("AC"), Qual: []byte("!")}
	if err := bad.ValidateFASTQ(); err == nil || !strings.Contains(err.Error(), "sequence length 2") {
		t.Fatalf("ValidateFASTQ() error = %v", err)
	}
	if _, err := bad.WriteTo(&bytes.Buffer{}); err == nil {
		t.Fatal("WriteTo() error = nil, want fastq validation error")
	}
}
