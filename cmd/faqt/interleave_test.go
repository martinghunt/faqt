package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInterleaveCommandWritesInterleavedFASTA(t *testing.T) {
	dir := t.TempDir()
	input1 := filepath.Join(dir, "left.txt")
	input2 := filepath.Join(dir, "right.txt")
	output := filepath.Join(dir, "interleaved.fa")
	if err := os.WriteFile(input1, []byte(">readA\nACGT\n>readB/1\nTT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(input1) error = %v", err)
	}
	if err := os.WriteFile(input2, []byte(">readA\nTGCA\n>readB/2\nAA\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(input2) error = %v", err)
	}

	cmd := newInterleaveCmd()
	cmd.SetArgs([]string{"--suffix1", "/1", "--suffix2", "/2", "-o", output, input1, input2})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	want := ">readA/1\nACGT\n>readA/2\nTGCA\n>readB/1\nTT\n>readB/2\nAA\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}
