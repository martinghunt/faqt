package bam

import (
	"bytes"
	"io"
	"testing"

	htsbam "github.com/biogo/hts/bam"
	htssam "github.com/biogo/hts/sam"
)

func TestReaderReverseComplementsReverseStrand(t *testing.T) {
	var buf bytes.Buffer

	ref, err := htssam.NewReference("ref", "", "", 100, nil, nil)
	if err != nil {
		t.Fatalf("NewReference() error = %v", err)
	}
	header, err := htssam.NewHeader(nil, []*htssam.Reference{ref})
	if err != nil {
		t.Fatalf("NewHeader() error = %v", err)
	}
	rec, err := htssam.NewRecord(
		"read1",
		ref,
		nil,
		0,
		-1,
		0,
		60,
		[]htssam.CigarOp{htssam.NewCigarOp(htssam.CigarMatch, 4)},
		[]byte("ATGC"),
		[]byte{32, 33, 34, 35},
		nil,
	)
	if err != nil {
		t.Fatalf("NewRecord() error = %v", err)
	}
	rec.Flags = htssam.Reverse

	w, err := htsbam.NewWriter(&buf, header, 0)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	if err := w.Write(rec); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	r, err := NewReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	out, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if out.Name != "read1" || string(out.Seq) != "GCAT" || string(out.Qual) != "DCBA" {
		t.Fatalf("record = %+v", out)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

// bamFixture builds a one-record BAM in memory.
func bamFixture(t *testing.T) []byte {
	t.Helper()
	header, err := htssam.NewHeader(nil, nil)
	if err != nil {
		t.Fatalf("NewHeader() error = %v", err)
	}
	var buf bytes.Buffer
	w, err := htsbam.NewWriter(&buf, header, 1)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	if err := w.Write(&htssam.Record{
		Name: "read1",
		Seq:  htssam.NewSeq([]byte("ACGT")),
		Qual: []byte{1, 2, 3, 4},
	}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

// TestNewReaderWithThreadsClampsZero guards the default: biogo reads zero as
// GOMAXPROCS, which would fan a BAM read out across every core.
func TestNewReaderWithThreadsClampsZero(t *testing.T) {
	for _, threads := range []int{-1, 0, 1} {
		buf := bamFixture(t)
		r, err := NewReaderWithThreads(bytes.NewReader(buf), threads)
		if err != nil {
			t.Fatalf("NewReaderWithThreads(%d) error = %v", threads, err)
		}
		rec, err := r.Read()
		if err != nil {
			t.Fatalf("Read(threads=%d) error = %v", threads, err)
		}
		if rec.Name == "" {
			t.Fatalf("record = %+v", rec)
		}
		if err := r.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}
