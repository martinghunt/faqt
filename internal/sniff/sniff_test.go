package sniff

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "fasta", input: "\n  >r1\nACGT\n", want: "fasta"},
		{name: "fastq", input: "@r1\nACGT\n+\n!!!!\n", want: "fastq"},
		{name: "clustal", input: "CLUSTAL W (1.83)\n", want: "clustal"},
		{name: "phylip", input: "2 4\nseq1 ACGT\nseq2 TTAA\n", want: "phylip"},
		{name: "sam header", input: "@HD\tVN:1.6\n", want: "sam"},
		{name: "sam record", input: "read1\t16\t*\t0\t0\t*\t*\t0\t0\tACGT\t!!!!\n", want: "sam"},
		{name: "genbank", input: "LOCUS       REC1\n", want: "genbank"},
		{name: "embl", input: "ID   REC1;\n", want: "embl"},
		{name: "gff3", input: "##gff-version 3\n", want: "gff3"},
		{name: "empty", input: " \n\t\r", wantErr: "empty input"},
		{name: "unknown", input: "not a known format\n", wantErr: "could not detect"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Format(bufio.NewReader(strings.NewReader(tc.input)))
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Format() error = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Format() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Format() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHelperDetectors(t *testing.T) {
	if looksLikeFASTQ([]byte("@r1\nAC\n")) {
		t.Fatal("looksLikeFASTQ() = true, want false for truncated record")
	}
	if !looksLikeFASTQ([]byte("@r1\nAC\n+\n!!\n")) {
		t.Fatal("looksLikeFASTQ() = false, want true")
	}
	if looksLikeSAM([]byte("read1\tX\t*\t0\t0\t*\t*\t0\t0\tACGT\t!!!!\n")) {
		t.Fatal("looksLikeSAM() = true, want false for non-integer flag")
	}
	if !looksLikeSAM([]byte("read1\t16\t*\t0\t0\t*\t*\t0\t0\tACGT\t!!!!\n")) {
		t.Fatal("looksLikeSAM() = false, want true")
	}
	if !looksLikeClustal([]byte("clustal omega\n")) {
		t.Fatal("looksLikeClustal() = false, want true")
	}
	if !looksLikePhylip([]byte("2 10\n")) {
		t.Fatal("looksLikePhylip() = false, want true")
	}
	if !isIntegerField([]byte("-10")) {
		t.Fatal("isIntegerField() = false, want true")
	}
	if isIntegerField([]byte("10x")) {
		t.Fatal("isIntegerField() = true, want false")
	}
}

func TestIsShortPeek(t *testing.T) {
	if !isShortPeek(bufio.ErrBufferFull) {
		t.Fatal("isShortPeek(bufio.ErrBufferFull) = false, want true")
	}
	if !isShortPeek(io.EOF) {
		t.Fatal("isShortPeek(io.EOF) = false, want true")
	}
	if isShortPeek(errors.New("boom")) {
		t.Fatal("isShortPeek(boom) = true, want false")
	}
}
