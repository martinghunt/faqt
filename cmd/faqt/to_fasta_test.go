package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestRunToFastaPathToPath(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "reads.dat")
	out := filepath.Join(dir, "out.fa")
	if err := os.WriteFile(in, []byte("@r1 desc\nACGT\n+\n!!!!\n@r2\nTT\n+\n##\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := seqio.ToFASTAPath(in, out, seqio.WithWrap(2), seqio.WithCompression(seqio.CompressAuto)); err != nil {
		t.Fatalf("ToFASTAPath() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := ">r1 desc\nAC\nGT\n>r2\nTT\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

func TestRemoveDashesTransform(t *testing.T) {
	transform := removeDashesTransform(true)
	rec, err := transform(&seqio.SeqRecord{Name: "r1", Seq: []byte("AC-G-T")})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	if string(rec.Seq) != "ACGT" {
		t.Fatalf("transformed seq = %q", string(rec.Seq))
	}
}

func TestRunToFastaStdinStdout(t *testing.T) {
	in, err := os.CreateTemp(t.TempDir(), "stdin-*")
	if err != nil {
		t.Fatalf("CreateTemp(stdin) error = %v", err)
	}
	if _, err := in.WriteString(">r1\nACGT\n"); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if _, err := in.Seek(0, 0); err != nil {
		t.Fatalf("Seek() error = %v", err)
	}
	out, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp(stdout) error = %v", err)
	}

	oldIn, oldOut := os.Stdin, os.Stdout
	os.Stdin, os.Stdout = in, out
	defer func() {
		os.Stdin, os.Stdout = oldIn, oldOut
	}()

	if err := seqio.ToFASTAPath("-", "-", seqio.WithCompression(seqio.CompressAuto)); err != nil {
		t.Fatalf("ToFASTAPath() error = %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != ">r1\nACGT\n" {
		t.Fatalf("stdout output = %q", string(data))
	}
}
