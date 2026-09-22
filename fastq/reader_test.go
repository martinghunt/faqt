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
	if first.Name != "r1" || first.Description != "desc" {
		t.Fatalf("first record header changed after reading second record: %+v", first)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

func TestReaderHandlesLongHeaderAndSeparator(t *testing.T) {
	longName := "read" + strings.Repeat("x", 64)
	input := "@" + longName + " desc\tone\nAC\n+" + strings.Repeat("q", 64) + "\n!!\n"
	r := NewReader(bufio.NewReaderSize(strings.NewReader(input), 16))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != longName || rec.Description != "desc one" {
		t.Fatalf("record header = (%q, %q), want (%q, %q)", rec.Name, rec.Description, longName, "desc one")
	}
	if string(rec.Seq) != "AC" || string(rec.Qual) != "!!" {
		t.Fatalf("record sequence = (%q, %q), want (%q, %q)", string(rec.Seq), string(rec.Qual), "AC", "!!")
	}
}

func TestReaderRejectsMismatchedLengths(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("@r1\nACGT\n+\nABC\n")))

	_, err := r.Read()
	if err == nil || !strings.Contains(err.Error(), "sequence length 4 and quality length 3") {
		t.Fatalf("Read() error = %v, want length mismatch", err)
	}
}

// TestReaderLongLines covers reads longer than the read buffer, where lines
// are stitched together from several ReadSlice calls.
func TestReaderLongLines(t *testing.T) {
	longSeq := strings.Repeat("ACGT", 200000) // 800 kB, e.g. a long read
	longQual := strings.Repeat("I", len(longSeq))
	longDesc := strings.Repeat("d", 300000)
	input := "@rec1 " + longDesc + "\n" + longSeq + "\n+\n" + longQual + "\n@rec2\nACGT\n+\n!!!!\n"

	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "rec1" || rec.Description != longDesc {
		t.Fatalf("first record name = %q, description length = %d", rec.Name, len(rec.Description))
	}
	if string(rec.Seq) != longSeq {
		t.Fatalf("first record sequence length = %d, want %d", len(rec.Seq), len(longSeq))
	}
	if string(rec.Qual) != longQual {
		t.Fatalf("first record quality length = %d, want %d", len(rec.Qual), len(longQual))
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "ACGT" || string(rec.Qual) != "!!!!" {
		t.Fatalf("second record = %+v", rec)
	}
	if _, err := r.Read(); err != io.EOF {
		t.Fatalf("Read() third error = %v, want io.EOF", err)
	}
}

// TestReaderSeqAndQualDoNotAlias checks that sequence and quality, which share
// one allocation, stay independent slices.
func TestReaderSeqAndQualDoNotAlias(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("@rec1\nACGT\n+\n!!!!\n")))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	rec.Seq = append(rec.Seq, 'X')
	if string(rec.Qual) != "!!!!" {
		t.Fatalf("appending to Seq changed Qual to %q", rec.Qual)
	}
	if len(rec.Seq) != 5 || string(rec.Seq[:4]) != "ACGT" {
		t.Fatalf("Seq = %q", rec.Seq)
	}
}

// TestReaderCRLF checks Windows line endings are trimmed from every line.
func TestReaderCRLF(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader("@rec1 desc\r\nACGT\r\n+\r\n!!!!\r\n")))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "rec1" || rec.Description != "desc" || string(rec.Seq) != "ACGT" || string(rec.Qual) != "!!!!" {
		t.Fatalf("record = %+v", rec)
	}
}
