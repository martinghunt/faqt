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

// TestReaderLongLines covers sequence and header lines longer than the read
// buffer, which are stitched together from several ReadSlice calls.
func TestReaderLongLines(t *testing.T) {
	longSeq := strings.Repeat("ACGT", 200000) // 800 kB, far past any buffer
	longDesc := strings.Repeat("d", 300000)
	input := ">rec1 " + longDesc + "\n" + longSeq + "\n>rec2\nACGT\n"

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

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "ACGT" {
		t.Fatalf("second record = %+v", rec)
	}
	if _, err := r.Read(); err != io.EOF {
		t.Fatalf("Read() third error = %v, want io.EOF", err)
	}
}

// TestReaderLongWrappedLines mixes a long line with wrapped lines in one
// record, so the stitched scratch buffer and the accumulator must not collide.
func TestReaderLongWrappedLines(t *testing.T) {
	long := strings.Repeat("A", 500000)
	input := ">rec1\nACGT\n" + long + "\nGGGG\n>rec2\nTTTT\n"

	r := NewReader(bufio.NewReader(strings.NewReader(input)))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	want := "ACGT" + long + "GGGG"
	if string(rec.Seq) != want {
		t.Fatalf("sequence length = %d, want %d", len(rec.Seq), len(want))
	}
	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "TTTT" {
		t.Fatalf("second record = %+v", rec)
	}
}

// TestReaderRecordsStayValid checks the ownership contract: a record must not
// change when later records are read, even though the Reader reuses buffers.
func TestReaderRecordsStayValid(t *testing.T) {
	input := ">rec1\nACGT\n>rec2\nNNNNNNNN\n>rec3\nTT\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	first, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	second, err := r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if _, err := r.Read(); err != nil {
		t.Fatalf("Read() third error = %v", err)
	}
	if first.Name != "rec1" || string(first.Seq) != "ACGT" {
		t.Fatalf("first record changed: %+v", first)
	}
	if second.Name != "rec2" || string(second.Seq) != "NNNNNNNN" {
		t.Fatalf("second record changed: %+v", second)
	}
}

// TestReaderCRLF checks Windows line endings are trimmed from headers and
// sequence lines.
func TestReaderCRLF(t *testing.T) {
	input := ">rec1 desc\r\nACGT\r\nAAAA\r\n>rec2\r\nTT\r\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "rec1" || rec.Description != "desc" || string(rec.Seq) != "ACGTAAAA" {
		t.Fatalf("record = %+v", rec)
	}
	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "TT" {
		t.Fatalf("second record = %+v", rec)
	}
}
