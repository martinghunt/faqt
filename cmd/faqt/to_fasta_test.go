package main

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func TestToFastaCommandReadsAllAGCSamplesWithPrefixedNames(t *testing.T) {
	in := writeCommandAGC(t)
	out := filepath.Join(t.TempDir(), "all.fa")
	cmd := newToFastaCmd()
	cmd.SetArgs([]string{in, "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, ">ref.chr1\n") || !strings.Contains(text, ">b.g h i 21\n") || !strings.Contains(text, ">c.3\n") {
		t.Fatalf("AGC FASTA output lacks prefixed records: %q", text)
	}
	if strings.Index(text, ">ref.chr1\n") > strings.Index(text, ">a.chr1a\n") || strings.Index(text, ">a.chr1a\n") > strings.Index(text, ">b.chr1\n") {
		t.Fatalf("AGC samples are not adjacent in archive order: %q", text)
	}
}

func TestToFastaCommandSelectsOneAGCSampleWithoutPrefix(t *testing.T) {
	in := writeCommandAGC(t)
	out := filepath.Join(t.TempDir(), "sample.fa")
	cmd := newToFastaCmd()
	cmd.SetArgs([]string{in, "--sample", "a", "-o", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	want := ">chr1a\nCTGAGCTGACTGA\n>chr3a\nAGTTTAGCT\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", data, want)
	}
}

func TestToFastaCommandRejectsSampleForNonAGCInput(t *testing.T) {
	in := filepath.Join(t.TempDir(), "input.fa")
	if err := os.WriteFile(in, []byte(">contig\nACGT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newToFastaCmd()
	cmd.SetArgs([]string{in, "--sample", "sample1", "-o", filepath.Join(t.TempDir(), "out.fa")})
	if err := cmd.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want non-AGC input error")
	}
}

func writeCommandAGC(t *testing.T) string {
	t.Helper()
	encoded, err := os.ReadFile(filepath.Join("..", "..", "agc", "testdata", "toy_ex.agc.b64"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "misleading.fasta")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
