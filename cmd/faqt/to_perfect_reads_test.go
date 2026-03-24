package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToPerfectReadsCommand(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "ref.fa")
	fwd := filepath.Join(dir, "reads_1.fq")
	rev := filepath.Join(dir, "reads_2.fq")
	if err := os.WriteFile(in, []byte(">chr1\nACGTACGTACGTACGTACGTACGTACGTACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := newToPerfectReadsCmd()
	cmd.SetArgs([]string{
		in,
		"--forward-out", fwd,
		"--reverse-out", rev,
		"--mean-insert", "12",
		"--insert-std", "0",
		"--coverage", "1",
		"--read-length", "4",
		"--seed", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	fwdData, err := os.ReadFile(fwd)
	if err != nil {
		t.Fatalf("ReadFile(fwd) error = %v", err)
	}
	revData, err := os.ReadFile(rev)
	if err != nil {
		t.Fatalf("ReadFile(rev) error = %v", err)
	}
	if !strings.Contains(string(fwdData), "/1") || !strings.Contains(string(revData), "/2") {
		t.Fatalf("unexpected outputs")
	}
	if !strings.Contains(string(fwdData), "DCBA") || !strings.Contains(string(revData), "DCBA") {
		t.Fatalf("expected non-constant quality profile")
	}
}

func TestToPerfectReadsCommandSingleEnd(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "ref.fa")
	out := filepath.Join(dir, "reads.fq")
	if err := os.WriteFile(in, []byte(">chr1\nACGTACGTACGTACGTACGTACGTACGTACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := newToPerfectReadsCmd()
	cmd.SetArgs([]string{
		in,
		"--out", out,
		"--coverage", "1",
		"--read-length", "4",
		"--seed", "1",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile(out) error = %v", err)
	}
	if !strings.Contains(string(data), "@chr1:1:") {
		t.Fatalf("unexpected output")
	}
	if !strings.Contains(string(data), "DCBA") {
		t.Fatalf("expected non-constant quality profile")
	}
}

func TestToPerfectReadsCommandRejectsMixedOutputs(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "ref.fa")
	if err := os.WriteFile(in, []byte(">chr1\nACGTACGTACGTACGTACGTACGTACGTACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := newToPerfectReadsCmd()
	cmd.SetArgs([]string{
		in,
		"--out", filepath.Join(dir, "reads.fq"),
		"--forward-out", filepath.Join(dir, "reads_1.fq"),
		"--reverse-out", filepath.Join(dir, "reads_2.fq"),
		"--mean-insert", "12",
		"--insert-std", "0",
		"--coverage", "1",
		"--read-length", "4",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "use either --out") {
		t.Fatalf("Execute() error = %v, want mixed output validation error", err)
	}
}
