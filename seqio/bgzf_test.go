package seqio_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	htsbam "github.com/biogo/hts/bam"
	"github.com/biogo/hts/bgzf"
	"github.com/biogo/hts/sam"
	"github.com/martinghunt/faqt/seqio"
)

// TestBGZFTextIsNotTreatedAsBAM is the regression test for bgzipped input
// being routed to the BAM reader. Before the fix every bgzipped text format
// failed with "sam: magic number mismatch".
func TestBGZFTextIsNotTreatedAsBAM(t *testing.T) {
	inputs := map[string]string{
		"fasta": ">rec1 desc\nACGT\n",
		"fastq": "@rec1 desc\nACGT\n+\n!!!!\n",
	}
	for name, plain := range inputs {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			w := bgzf.NewWriter(&buf, 1)
			if _, err := w.Write([]byte(plain)); err != nil {
				t.Fatalf("bgzf Write() error = %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("bgzf Close() error = %v", err)
			}

			path := filepath.Join(t.TempDir(), "input.gz")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			reader, err := seqio.OpenPath(path)
			if err != nil {
				t.Fatalf("OpenPath() error = %v", err)
			}
			rec, err := reader.Read()
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if rec.Name != "rec1" || rec.Description != "desc" || string(rec.Seq) != "ACGT" {
				t.Fatalf("record = %+v", rec)
			}
			if closer, ok := reader.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
		})
	}
}

// TestBGZFBAMStillReadsAsBAM guards the other side of the same decision: BAM
// is also BGZF, and must still reach the BAM reader.
func TestBGZFBAMStillReadsAsBAM(t *testing.T) {
	header, err := sam.NewHeader(nil, nil)
	if err != nil {
		t.Fatalf("NewHeader() error = %v", err)
	}
	var buf bytes.Buffer
	bw, err := htsbam.NewWriter(&buf, header, 1)
	if err != nil {
		t.Fatalf("bam NewWriter() error = %v", err)
	}
	rec := &sam.Record{
		Name: "rec1",
		Seq:  sam.NewSeq([]byte("ACGT")),
		Qual: []byte{1, 2, 3, 4},
	}
	if err := bw.Write(rec); err != nil {
		t.Fatalf("bam Write() error = %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("bam Close() error = %v", err)
	}

	path := filepath.Join(t.TempDir(), "input.bam")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	reader, err := seqio.OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	got, err := reader.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if got.Name != "rec1" || string(got.Seq) != "ACGT" {
		t.Fatalf("record = %+v", got)
	}
	if closer, ok := reader.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
}
