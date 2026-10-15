package seqio_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestCheckAlphabet(t *testing.T) {
	for _, tc := range []struct {
		name, input, alphabet string
		gaps                  bool
		want                  []string
	}{
		{"off", ">one\nAC?X-\n", "", false, nil},
		{"DNA ambiguity", ">one\nACGTRYSWKMBDHVN\n>two\nacgtryswkmbdhvn\n", "dna", false, nil},
		{"RNA ambiguity", ">one\nacguryswkmbdhvn\n", "rna", false, nil},
		{"DNA rejects U", ">one\nACUU\n", "dna", false, []string{`record "one", characters outside dna alphabet at sequence positions 3-4`}},
		{"RNA rejects T", ">one\nacgt\n", "rna", false, []string{`sequence positions 4-4`}},
		{"protein", ">one\nABCDEFGHIJKLMNOPQRSTUVWXYZ*\n>two\nabcdefghijklmnopqrstuvwxyz*\n", "protein", false, nil},
		{"protein punctuation", ">one\nMEX?2\n", "protein", false, []string{`sequence positions 4-5`}},
		{"gaps rejected", ">one\nA-.C\n", "dna", false, []string{`sequence positions 2-3`}},
		{"gaps accepted", ">one\nA-.C\n", "dna", true, nil},
		{"all records and wrapped positions", ">one\nACX\nG??T\n>two\nZ\n", "dna", false, []string{`record "one", characters outside dna alphabet at sequence positions 3-3`, `sequence positions 5-6`, `record "two", characters outside dna alphabet at sequence positions 1-1`}},
		{"headers and qualities ignored", "@one E?!2\nACGT\n+\nE?!2\n@two\nNN\n+\n##\n", "dna", false, nil},
		{"FASTQ parser and alphabet errors", "@one\nACX\n+\n!!\n@two\nZ\n+\n!\n", "dna", false, []string{`sequence length 3 and quality length 2`, `record "one", characters outside dna alphabet at sequence positions 3-3`, `record "two", characters outside dna alphabet at sequence positions 1-1`}},
	} {
		for _, allErrors := range []bool{false, true} {
			t.Run(tc.name+fmt.Sprint(allErrors), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "input")
				if err := os.WriteFile(path, []byte(tc.input), 0o600); err != nil {
					t.Fatal(err)
				}
				var got []string
				_, err := seqio.CheckPath(path, allErrors, func(err error) { got = append(got, err.Error()) }, seqio.WithCheckAlphabet(tc.alphabet), seqio.WithCheckGaps(tc.gaps))
				want := tc.want
				if !allErrors && len(want) > 0 {
					want = want[:1]
				}
				if (err != nil) != (len(want) > 0) || len(got) != len(want) {
					t.Fatalf("CheckPath() error=%v diagnostics=%q, want %q", err, got, want)
				}
				for i, s := range want {
					if !strings.Contains(got[i], s) {
						t.Errorf("diagnostic %d=%q want %q", i, got[i], s)
					}
				}
			})
		}
	}
}

func TestCheckAlphabetConfigurationErrorsBeforeOpeningInput(t *testing.T) {
	for _, option := range []seqio.Option{seqio.WithCheckAlphabet("auto"), seqio.WithCheckGaps(true)} {
		var diagnostics []error
		_, err := seqio.CheckPath(filepath.Join(t.TempDir(), "missing"), true, func(err error) { diagnostics = append(diagnostics, err) }, option)
		if err == nil || !strings.Contains(err.Error(), "alphabet") || len(diagnostics) != 1 {
			t.Fatalf("CheckPath() error=%v diagnostics=%v", err, diagnostics)
		}
	}
}

func TestCheckAlphabetDoesNotDuplicateFASTANULDiagnostics(t *testing.T) {
	for _, prefix := range []string{"", "##gff-version 3\n##FASTA\n"} {
		path := filepath.Join(t.TempDir(), "input")
		if err := os.WriteFile(path, []byte(prefix+">one\nAX\x00?T\x00\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		var got []string
		_, err := seqio.CheckPath(path, true, func(err error) { got = append(got, err.Error()) }, seqio.WithCheckAlphabet("dna"))
		if err == nil || len(got) != 4 {
			t.Fatalf("CheckPath() error=%v diagnostics=%q, want 2 NUL runs and 2 alphabet runs", err, got)
		}
		for i, want := range []string{"NUL byte", "NUL byte", "sequence positions 2-2", "sequence positions 4-4"} {
			if !strings.Contains(got[i], want) {
				t.Errorf("diagnostic %d=%q want %q", i, got[i], want)
			}
		}
	}
}
