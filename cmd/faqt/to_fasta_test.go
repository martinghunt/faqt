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
	rec, err := transform(&seqio.SeqRecord{Name: "r1", Seq: []byte("AC-G-T"), Qual: []byte("123456")})
	if err != nil {
		t.Fatalf("transform() error = %v", err)
	}
	if string(rec.Seq) != "ACGT" {
		t.Fatalf("transformed seq = %q", string(rec.Seq))
	}
	if string(rec.Qual) != "1246" {
		t.Fatalf("transformed qual = %q", string(rec.Qual))
	}
}

func TestRunToFastaStdinStdout(t *testing.T) {
	got, err := runWithCapturedStdinStdout(t, ">r1\nACGT\n", func() error {
		return seqio.ToFASTAPath("-", "-", seqio.WithCompression(seqio.CompressAuto))
	})
	if err != nil {
		t.Fatalf("ToFASTAPath() error = %v", err)
	}
	if got != ">r1\nACGT\n" {
		t.Fatalf("stdout output = %q", got)
	}
}
