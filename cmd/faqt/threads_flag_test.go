package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/biogo/hts/bgzf"
)

// TestThreadsFlagDefaultsToOne checks every command that reads or writes
// sequence data offers --threads and defaults it to one core.
func TestThreadsFlagDefaultsToOne(t *testing.T) {
	for _, name := range []string{"to-fasta", "stats", "interleave", "to-perfect-reads"} {
		t.Run(name, func(t *testing.T) {
			root := newRootCmd()
			cmd, _, err := root.Find([]string{name})
			if err != nil {
				t.Fatalf("Find(%q) error = %v", name, err)
			}
			flag := cmd.Flags().Lookup("threads")
			if flag == nil {
				t.Fatalf("%s has no --threads flag", name)
			}
			if flag.DefValue != "1" {
				t.Fatalf("%s --threads default = %q, want \"1\"", name, flag.DefValue)
			}
		})
	}
}

// TestToFastaReadsBgzippedInput is the command-level regression test for
// bgzipped input, which failed with "sam: magic number mismatch".
func TestToFastaReadsBgzippedInput(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "reads.fastq.gz")
	out := filepath.Join(dir, "out.fa")

	var buf bytes.Buffer
	w := bgzf.NewWriter(&buf, 1)
	if _, err := w.Write([]byte("@r1 desc\nACGT\n+\n!!!!\n@r2\nTT\n+\n##\n")); err != nil {
		t.Fatalf("bgzf Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bgzf Close() error = %v", err)
	}
	if err := os.WriteFile(in, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	root := newRootCmd()
	root.SetArgs([]string{"to-fasta", "-i", in, "-o", out})
	if err := root.Execute(); err != nil {
		t.Fatalf("to-fasta on bgzipped input error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	want := ">r1 desc\nACGT\n>r2\nTT\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

// TestToFastaWithThreads checks the flag is accepted and does not change
// output.
func TestToFastaWithThreads(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "reads.fastq")
	if err := os.WriteFile(in, []byte("@r1\nACGT\n+\n!!!!\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	outputs := make([]string, 0, 2)
	for _, threads := range []string{"1", "4"} {
		out := filepath.Join(dir, "out"+threads+".fa")
		root := newRootCmd()
		root.SetArgs([]string{"to-fasta", "-i", in, "-o", out, "--threads", threads})
		if err := root.Execute(); err != nil {
			t.Fatalf("to-fasta --threads %s error = %v", threads, err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		outputs = append(outputs, string(data))
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("--threads changed output: %q vs %q", outputs[0], outputs[1])
	}
	if outputs[0] != ">r1\nACGT\n" {
		t.Fatalf("output = %q", outputs[0])
	}
}
