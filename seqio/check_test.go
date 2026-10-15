package seqio_test

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestCheckPathFindsCompressionErrorAfterRecoverableError(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte(">one\nA\x00C\n" + strings.Repeat("ACGT\n", 3000))); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	data := compressed.Bytes()
	data[len(data)-8] ^= 1
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, allErrors := range []bool{false, true} {
		var got []string
		_, err := seqio.CheckPath(path, allErrors, func(err error) { got = append(got, err.Error()) })
		want := 1
		if allErrors {
			want = 2
		}
		if err == nil || len(got) != want || !strings.Contains(got[0], "NUL") {
			t.Fatalf("allErrors=%v: diagnostics=%q, error=%v", allErrors, got, err)
		}
		if allErrors && !strings.Contains(got[1], "checksum") {
			t.Errorf("last diagnostic = %q, want checksum error", got[1])
		}
	}
}

func TestCheckPathAGC(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input.fa")
	if err := os.WriteFile(path, toyAGC(t), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, allErrors := range []bool{false, true} {
		count, err := seqio.CheckPath(path, allErrors, nil)
		if err != nil || count != 13 {
			t.Fatalf("CheckPath() = (%d, %v), want 13 records", count, err)
		}
	}
}

func TestCheckPathDiagnostics(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"FASTA", ">one\nA\x00C\nG\x00\x00T\n>two\nT\x00A\n", []string{"line 2", "line 3", "line 5"}},
		{"FASTA headers", ">one\x00\nACGT\n>two bad\x00description\nTT\x00\n", []string{"line 1", "line 3", "line 4"}},
		{"long FASTA", ">one\n" + strings.Repeat("A", 300000) + "\x00\n>two\nT\x00\n", []string{"line 2", "line 4"}},
		{"FASTQ", "@one\nACGT\n+\n!!!\n@two\nAC\n+\n!!!\n", []string{"record \"one\"", "record \"two\""}},
		{"FASTQ structural failure", "@one\nACGT\n+\n!!!\n@two\nAC\ninvalid separator\n!!\n@three\nAC\n+\n!\n", []string{"record \"one\"", "separator line"}},
		{"FASTQ truncated", "@one\nACGT\n+\n!!!\n@two\nAC\n+\n", []string{"record \"one\"", "unexpected EOF"}},
		{"GFF3", "##gff-version 3\n##FASTA\n>one\nA\x00C\n>two\nT\x00\n", []string{"line 2", "line 4"}},
		{"GFF3 empty", "##gff-version 3\n##FASTA\n", []string{"does not contain sequence records"}},
	}
	for _, tc := range tests {
		for _, allErrors := range []bool{false, true} {
			t.Run(tc.name+fmt.Sprint(allErrors), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "input")
				if err := os.WriteFile(path, []byte(tc.input), 0o600); err != nil {
					t.Fatal(err)
				}
				var got []string
				_, err := seqio.CheckPath(path, allErrors, func(err error) { got = append(got, err.Error()) })
				want := tc.want
				if !allErrors {
					want = want[:1]
				}
				if err == nil || len(got) != len(want) {
					t.Fatalf("CheckPath() error = %v, diagnostics = %q, want %q", err, got, want)
				}
				for i, match := range want {
					if !strings.Contains(got[i], match) {
						t.Errorf("diagnostic %d = %q, want %q", i, got[i], match)
					}
				}
				if err.Error() != got[0] {
					t.Errorf("returned error = %v, want first diagnostic %q", err, got[0])
				}
			})
		}
	}
}

func TestCheckEveryTextFormatAndCompression(t *testing.T) {
	for _, fixture := range textFormatFixtures() {
		for _, comp := range readCompressors() {
			t.Run(fixture.format+"/"+comp.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "input")
				if err := os.WriteFile(path, comp.compress(t, []byte(fixture.plain)), 0o600); err != nil {
					t.Fatal(err)
				}
				count, err := seqio.CheckPath(path, true, func(err error) { t.Errorf("unexpected diagnostic: %v", err) })
				if err != nil || count != len(fixture.want) {
					t.Fatalf("CheckPath() = (%d, %v), want %d records", count, err, len(fixture.want))
				}
			})
		}
	}
}
