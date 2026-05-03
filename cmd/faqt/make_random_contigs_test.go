package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeRandomContigsCommand(t *testing.T) {
	out := filepath.Join(t.TempDir(), "random.fa")
	cmd := newMakeRandomContigsCmd()
	cmd.SetArgs([]string{
		"--seed", "11",
		"--first-number", "42",
		"--prefix", "p",
		"-o", out,
		"2",
		"3",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if !strings.Contains(got, ">p42\n") || !strings.Contains(got, ">p43\n") {
		t.Fatalf("unexpected output names:\n%s", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, ">") {
			continue
		}
		if len(line) != 3 {
			t.Fatalf("sequence line length = %d, want 3 in output:\n%s", len(line), got)
		}
	}
}

func TestMakeRandomContigsCommandNameByLetters(t *testing.T) {
	out := filepath.Join(t.TempDir(), "random.fa")
	cmd := newMakeRandomContigsCmd()
	cmd.SetArgs([]string{"--name-by-letters", "--seed", "1", "-o", out, "28", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if strings.Count(got, ">A\n") != 2 || strings.Count(got, ">B\n") != 2 || !strings.Contains(got, ">Z\n") {
		t.Fatalf("unexpected letter names:\n%s", got)
	}
}

func TestMakeRandomContigsCommandStdoutDefault(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "stdout-*")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = out
	defer func() { os.Stdout = oldStdout }()

	cmd := newMakeRandomContigsCmd()
	cmd.SetArgs([]string{"--seed", "5", "1", "4"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(out.Name())
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	got := string(data)
	if !strings.HasPrefix(got, ">1\n") {
		t.Fatalf("stdout output = %q", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, ">") {
			continue
		}
		if len(line) != 4 {
			t.Fatalf("sequence line length = %d, want 4 in output:\n%s", len(line), got)
		}
	}
}
