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
		">a\nNNGGAAAA\n" +
		">b\nGGGCCC\n" +
		">c\nACGT\n" +
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
	expected := "filename\ttotal_length\tnumber\tmean_length\tlongest\tshortest\tN_count\tGaps\tGC\tN50\tN50n\tN70\tN70n\tN90\tN90n\n" +
		path + "\t21\t5\t4.20\t8\t1\t4\t2\t58.82\t6\t2\t4\t3\t2\t4\n"
	if got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestStatsCommandDefaultsToStdin(t *testing.T) {
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-u"})
	got, err := runWithCapturedStdinStdout(t, ">a\nACGT\n>b\nNN\n", cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "-\t6\t2\t3.00\t4\t2\t2\t1\t50.00\t4\t1\t2\t2\t2\t2\n"
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
	if err := os.WriteFile(one, []byte(">a\nACGT\n"), 0o600); err != nil {
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
	want := "combined\t6\t2\t3.00\t4\t2\t2\t1\t50.00\t4\t1\t2\t2\t2\t2\n"
	if got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestStatsCommandPerSequence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stats.fa")
	data := "" +
		">a desc here\nNNGGAAAA\n" +
		">b\nGGGCCC\n" +
		">c\nACGT\n" +
		">d\nNN\n" +
		">e\nA\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-p", path})
	got, err := runWithCapturedStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "file\tname\tlength\tN_count\tGaps\tGC\n" +
		path + "\ta\t8\t2\t1\t33.33\n" +
		path + "\tb\t6\t0\t0\t100.00\n" +
		path + "\tc\t4\t0\t0\t50.00\n" +
		path + "\td\t2\t2\t1\t0.00\n" +
		path + "\te\t1\t0\t0\t0.00\n"
	if got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestStatsCommandPerSequenceNoHeaderFromStdin(t *testing.T) {
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-p", "-u"})
	got, err := runWithCapturedStdinStdout(t, ">a\nACGT\n>b\nNN\n", cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "-\ta\t4\t0\t0\t50.00\n" +
		"-\tb\t2\t2\t1\t0.00\n"
	if got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestStatsCommandPerSequenceMinimumLength(t *testing.T) {
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-p", "-u", "-l", "4"})
	got, err := runWithCapturedStdinStdout(t, ">a\nACGT\n>b\nNN\n", cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	expected := "-\ta\t4\t0\t0\t50.00\n"
	if got != expected {
		t.Fatalf("stdout = %q, want %q", got, expected)
	}
}

func TestStatsCommandPerSequenceRejectsIncompatibleFlags(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.fa")
	if err := os.WriteFile(one, []byte(">a\nACGT\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "greppy",
			args: []string{"-p", "-s", one},
			want: "grep friendly",
		},
		{
			name: "combine inputs",
			args: []string{"-p", "--combine-inputs", one},
			want: "--combine-inputs cannot be used with --per-sequence",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newStatsCmd()
			cmd.SetArgs(tc.args)
			cmd.SilenceUsage = true
			cmd.SilenceErrors = true
			got, err := runWithCapturedStdout(t, cmd.Execute)
			if err == nil {
				t.Fatalf("Execute() expected an error, stdout = %q", got)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Execute() error = %v, want it to mention %q", err, tc.want)
			}
			if got != "" {
				t.Fatalf("stdout = %q, want nothing", got)
			}
		})
	}
}

func TestStatsCommandPerSequenceReportsAGCSamples(t *testing.T) {
	path := writeCommandAGC(t)
	cmd := newStatsCmd()
	cmd.SetArgs([]string{"-p", "-u", path})
	got, err := runWithCapturedStdout(t, cmd.Execute)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if lines := strings.Count(got, "\n"); lines != 13 {
		t.Fatalf("stdout line count = %d, want 13: %q", lines, got)
	}
	for _, sample := range []string{"ref", "a", "b", "c"} {
		if !strings.Contains(got, path+":"+sample+"\t") {
			t.Errorf("stdout lacks sample %q: %q", sample, got)
		}
	}
}
