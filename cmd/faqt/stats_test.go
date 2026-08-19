package main

import (
	"os"
	"path/filepath"
	"strings"
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

	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-t", path})
	got, err := runWithCapturedStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "filename\ttotal_length\tnumber\tmean_length\tlongest\tshortest\tN_count\tGaps\tN50\tN50n\tN70\tN70n\tN90\tN90n\n" +
		path + "\t21\t5\t4.20\t8\t1\t4\t2\t6\t2\t4\t3\t2\t4\n"
	if got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestStatsCommandDefaultsToStdin(t *testing.T) {
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-u"})
	got, err := runWithCapturedStdinStdout(t, ">a\nAAAA\n>b\nNN\n", cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "-\t6\t2\t3.00\t4\t2\t2\t1\t4\t1\t2\t2\t2\t2\n"
	if got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestStatsCommandReportsAGCPerSampleByDefault(t *testing.T) {
	path := writeCommandAGC(t)
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-u", path})
	got, err := runWithCapturedStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	for _, sample := range []string{"ref", "a", "b", "c"} {
		if !strings.Contains(got, path+":"+sample+"\t") {
			t.Errorf("stdout lacks sample %q: %q", sample, got)
		}
	}
	if strings.Count(got, "\n") != 4 {
		t.Fatalf("stdout line count = %d, want 4: %q", strings.Count(got, "\n"), got)
	}
}

func TestStatsCommandCombineInputsMergesMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.fa")
	two := filepath.Join(dir, "two.fa")
	if err := os.WriteFile(one, []byte(">a\nAAAA\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(two, []byte(">b\nNN\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-u", "--combine-inputs", one, two})
	got, err := runWithCapturedStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "combined\t6\t2\t3.00\t4\t2\t2\t1\t4\t1\t2\t2\t2\t2\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
