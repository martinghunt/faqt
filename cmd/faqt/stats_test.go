package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStatsCommandTabDelimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.fa")
	data := "" +
		">a\nNNAAAAAA\n" +
		">b\nAAAAAA\n" +
		">c\nAAAA\n" +
		">d\nNN\n" +
		">e\nA\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	oldStdout := os.Stdout
	out, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	os.Stdout = out
	defer func() { os.Stdout = oldStdout }()

	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-t", path})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	got, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	expected := "filename\ttotal_length\tnumber\tmean_length\tlongest\tshortest\tN_count\tGaps\tN50\tN50n\tN70\tN70n\tN90\tN90n\n" +
		path + "\t21\t5\t4.20\t8\t1\t4\t2\t6\t2\t4\t3\t2\t4\n"
	if string(got) != expected {
		t.Fatalf("stdout = %q, want %q", string(got), expected)
	}
}

func TestRootVersionFlag(t *testing.T) {
	cmd := newRootCmd()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if stdout.String() == "" {
		t.Fatal("expected version output")
	}
}
