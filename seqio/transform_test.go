package seqio_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

func TestProcessAppliesTransform(t *testing.T) {
	reader, err := seqio.OpenReader(bytes.NewBufferString(">r1\nACGT\n>r2\nTTAA\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}

	var out bytes.Buffer
	writer := seqio.NewFASTAWriter(&out, seqio.WithWrap(2))
	err = seqio.Process(reader, writer, func(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
		copyRec := *rec
		copyRec.Seq = seq.ReverseComplement(rec.Seq)
		return &copyRec, nil
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	want := ">r1\nAC\nGT\n>r2\nTT\nAA\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestTransformPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "reads.fa")
	out := filepath.Join(dir, "rc.fa")
	if err := os.WriteFile(in, []byte(">r1\nACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := seqio.TransformPath(in, out, seqio.FASTA, func(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
		copyRec := *rec
		copyRec.Seq = seq.ReverseComplement(rec.Seq)
		return &copyRec, nil
	}, seqio.WithWrap(0))
	if err != nil {
		t.Fatalf("TransformPath() error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != ">r1\nACGT\n" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestToFASTAPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "reads.fq")
	out := filepath.Join(dir, "reads.fa")
	if err := os.WriteFile(in, []byte("@r1\nACGT\n+\n!!!!\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := seqio.ToFASTAPath(in, out, seqio.WithWrap(2)); err != nil {
		t.Fatalf("ToFASTAPath() error = %v", err)
	}

	r, err := seqio.OpenPath(out)
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	rec, err := r.Read()
	if err != nil && err != io.EOF {
		t.Fatalf("Read() error = %v", err)
	}
	if rec == nil || rec.Name != "r1" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestToFASTAPathWithTransform(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "reads.fa")
	out := filepath.Join(dir, "reads.filtered.fa")
	if err := os.WriteFile(in, []byte(">keep\nACGT\n>drop\nTTAA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	err := seqio.ToFASTAPathWithTransform(in, out, func(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
		if rec.Name == "drop" {
			return nil, nil
		}
		return rec, nil
	})
	if err != nil {
		t.Fatalf("ToFASTAPathWithTransform() error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != ">keep\nACGT\n" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestProcessErrorPaths(t *testing.T) {
	reader, err := seqio.OpenReader(bytes.NewBufferString(">r1\nACGT\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}

	if err := seqio.Process(reader, errorWriter{}, nil); err == nil || !strings.Contains(err.Error(), "write failed") {
		t.Fatalf("Process() error = %v", err)
	}

	reader, err = seqio.OpenReader(bytes.NewBufferString(">r1\nACGT\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	if err := seqio.Process(reader, seqio.NewFASTAWriter(io.Discard), func(*seqio.SeqRecord) (*seqio.SeqRecord, error) {
		return nil, errors.New("transform failed")
	}); err == nil || !strings.Contains(err.Error(), "transform failed") {
		t.Fatalf("Process() error = %v", err)
	}
}

type errorWriter struct{}

func (errorWriter) Write(*seqio.SeqRecord) error { return errors.New("write failed") }
func (errorWriter) Close() error                 { return nil }
