package fastq

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

func TestWriteRecord(t *testing.T) {
	var buf bytes.Buffer
	rec := seqrecord.SeqRecord{Name: "r1", Description: "desc", Seq: []byte("ACGT"), Qual: []byte("!!!!")}
	if err := WriteRecord(&buf, rec); err != nil {
		t.Fatalf("WriteRecord() error = %v", err)
	}
	if got := buf.String(); got != "@r1 desc\nACGT\n+\n!!!!\n" {
		t.Fatalf("WriteRecord() = %q", got)
	}
}

func TestMustFormat(t *testing.T) {
	rec := seqrecord.SeqRecord{Name: "r1", Seq: []byte("AC"), Qual: []byte("!!")}
	if got := MustFormat(rec); got != "@r1\nAC\n+\n!!\n" {
		t.Fatalf("MustFormat() = %q", got)
	}
}

func TestWriteRecordValidationError(t *testing.T) {
	err := WriteRecord(&bytes.Buffer{}, seqrecord.SeqRecord{Name: "bad", Seq: []byte("AC"), Qual: []byte("!")})
	if err == nil || !strings.Contains(err.Error(), "sequence length 2 and quality length 1") {
		t.Fatalf("WriteRecord() error = %v", err)
	}
}
