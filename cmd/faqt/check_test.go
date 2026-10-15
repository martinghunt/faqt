package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCommand(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(">one\nACGT\n>two\nNN\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	corrupt := append([]byte(nil), compressed.Bytes()...)
	corrupt[len(corrupt)-8] ^= 1 // Break the CRC without changing the records.
	tests := []struct {
		name string
		data string
		want string
		fail bool
	}{
		{"FASTA", ">one\nACGT\n>two\nNN\n", "OK\t2 records", false},
		{"FASTQ", "@one\nACGT\n+\n!!!!\n@two\nNN\n+\n!!\n", "OK\t2 records", false},
		{"gzip", compressed.String(), "OK\t2 records", false},
		{"corrupt gzip", string(corrupt), "ERROR\t", true},
		{"truncated gzip", compressed.String()[:compressed.Len()-4], "ERROR\t", true},
		{"NUL", ">one\nACGT\n>two\nA\x00C\n", "ERROR\tfasta: NUL byte (0x00) on line 4", true},
		{"bad FASTQ", "@one\nACGT\n+\n!!!\n", "ERROR\t", true},
		{"empty", "", "ERROR\t", true},
		{"unknown", "not a sequence file\n", "ERROR\t", true},
	}
	for _, tc := range tests {
		for _, source := range []string{"path", "default stdin", "explicit stdin"} {
			t.Run(tc.name+"/"+source, func(t *testing.T) {
				root := newRootCmd()
				root.SilenceErrors = true
				root.SilenceUsage = true
				args := []string{"check"}
				var out string
				var err error
				if source == "path" {
					path := filepath.Join(t.TempDir(), "misleading.bam")
					if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
						t.Fatal(err)
					}
					root.SetArgs(append(args, path))
					out, err = runWithCapturedOutput(t, root)
				} else {
					if source == "explicit stdin" {
						args = append(args, "-")
					}
					root.SetArgs(args)
					out, err = runWithCapturedStdinOutput(t, tc.data, root)
				}
				if (err != nil) != tc.fail {
					t.Fatalf("Execute() = (%q, %v), want %q, failure %v", out, err, tc.want, tc.fail)
				}
				if !strings.Contains(out, "\t"+tc.want) {
					t.Fatalf("stdout = %q, want %q", out, tc.want)
				}
			})
		}
	}
}

func TestCheckCommandFailurePolicy(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.fa")
	if err := os.WriteFile(good, []byte(">seq\nACGT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.fa")
	for _, allErrors := range []bool{false, true} {
		t.Run(fmt.Sprint(allErrors), func(t *testing.T) {
			cmd := newCheckCmd()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			args := []string{missing, good}
			if allErrors {
				args = append([]string{"--all-errors"}, args...)
			}
			cmd.SetArgs(args)
			out, err := runWithCapturedOutput(t, cmd)
			if err == nil {
				t.Fatal("Execute() expected an error")
			}
			if !strings.Contains(out, missing+"\tERROR\t") {
				t.Fatalf("stdout = %q, want missing file error", out)
			}
			if strings.Contains(out, good+"\tOK\t1 records\n") != allErrors {
				t.Fatalf("stdout = %q, continuation = %v", out, allErrors)
			}
		})
	}
}

func TestCheckCommandAllErrorsWithinFile(t *testing.T) {
	for _, allErrors := range []bool{false, true} {
		t.Run(fmt.Sprint(allErrors), func(t *testing.T) {
			cmd := newCheckCmd()
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			args := []string{"-"}
			want := 1
			if allErrors {
				args = append([]string{"--all-errors"}, args...)
				want = 3
			}
			cmd.SetArgs(args)
			out, err := runWithCapturedStdinOutput(t, ">one\nA\x00C\nG\x00T\n>two\nT\x00A\n", cmd)
			if err == nil || strings.Count(out, "\tERROR\t") != want || strings.Contains(out, "\tOK\t") {
				t.Fatalf("Execute() = (%q, %v), want %d errors", out, err, want)
			}
		})
	}
}

func TestCheckCommandDoesNotPrintUsageOnFailure(t *testing.T) {
	for _, allErrors := range []bool{false, true} {
		t.Run(fmt.Sprint(allErrors), func(t *testing.T) {
			root := newRootCmd()
			var stderr bytes.Buffer
			root.SetErr(&stderr)
			args := []string{"check"}
			if allErrors {
				args = append(args, "--all-errors")
			}
			root.SetArgs(args)
			out, err := runWithCapturedStdinOutput(t, ">one\nA\x00\x00C\x00G\x00\x00\n", root)
			want := 1
			if allErrors {
				want = 3
			}
			if err == nil || strings.Count(out, "\tERROR\t") != want {
				t.Fatalf("Execute() = (%q,%v), want %d errors", out, err, want)
			}
			if strings.Contains(out+stderr.String(), "Usage:") {
				t.Fatalf("unexpected usage: stdout=%q stderr=%q", out, stderr.String())
			}
			if !strings.Contains(out, `contig "one", sequence positions 2-3`) {
				t.Fatalf("missing contig and positions in %q", out)
			}
		})
	}
}

func TestCheckCommandAlphabet(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		args        []string
		gzip        bool
		errors      int
		want        string
	}{
		{"off by default", ">one\nAX?\n", nil, false, 0, "OK"},
		{"DNA ambiguity", ">one\nacgtryswkmbdhvn\n", []string{"--alphabet", "dna"}, false, 0, "OK"},
		{"DNA invalid", ">one\nACXE\n", []string{"--alphabet", "dna"}, false, 1, `record "one", characters outside dna alphabet at sequence positions 3-4`},
		{"RNA", ">one\nacgu\n", []string{"--alphabet", "rna"}, false, 0, "OK"},
		{"protein", ">one\nMEKXUOBZJ*\n", []string{"--alphabet", "protein"}, false, 0, "OK"},
		{"gaps", ">one\nAC-.N\n", []string{"--alphabet", "dna", "--allow-gaps"}, false, 0, "OK"},
		{"gaps rejected", ">one\nAC-.N\n", []string{"--alphabet", "dna"}, false, 1, `sequence positions 3-4`},
		{"NUL still rejected", ">one\nAC\x00N\n", nil, false, 1, "NUL byte"},
		{"first invalid run", ">one\nACXXG?\n>two\nE\n", []string{"--alphabet", "dna"}, false, 1, `sequence positions 3-4`},
		{"all invalid runs", ">one\nACXXG?\n>two\nE\n", []string{"--alphabet", "dna", "--all-errors"}, false, 3, `record "two"`},
		{"gzip invalid runs", ">one\nACXXG?\n>two\nE\n", []string{"--alphabet", "dna", "--all-errors"}, true, 3, `record "two"`},
		{"unknown alphabet", ">one\nACGT\n", []string{"--alphabet", "auto"}, false, 1, "unknown alphabet"},
		{"gaps without alphabet", ">one\nACGT\n", []string{"--allow-gaps"}, false, 1, "requires an alphabet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := newRootCmd()
			root.SetArgs(append([]string{"check"}, tc.args...))
			var stderr bytes.Buffer
			root.SetErr(&stderr)
			input := tc.input
			if tc.gzip {
				var buf bytes.Buffer
				zw := gzip.NewWriter(&buf)
				if _, err := zw.Write([]byte(input)); err != nil {
					t.Fatal(err)
				}
				if err := zw.Close(); err != nil {
					t.Fatal(err)
				}
				input = buf.String()
			}
			out, err := runWithCapturedStdinOutput(t, input, root)
			if (err != nil) != (tc.errors > 0) || strings.Count(out, "\tERROR\t") != tc.errors || !strings.Contains(out, tc.want) {
				t.Fatalf("Execute()=(%q,%v), want %d errors and %q", out, err, tc.errors, tc.want)
			}
			if strings.Contains(out+stderr.String(), "Usage:") {
				t.Fatalf("unexpected usage: %q %q", out, stderr.String())
			}
		})
	}
}
