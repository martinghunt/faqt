package sniff

import (
	"bufio"
	"errors"
	"fmt"
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

func TestFormatUnknownErrorIsTyped(t *testing.T) {
	_, err := Format(bufio.NewReader(strings.NewReader("not a known format\n")))
	if !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("Format() error = %v, want ErrUnknownFormat", err)
	}
}

func TestFormatFASTQPeekBoundaries(t *testing.T) {
	for _, size := range []int{4096, PeekSize} {
		for _, tc := range []struct {
			name  string
			input string
			want  string
		}{
			{"long header", "@" + strings.Repeat("r", 300000) + "\nAC\n+\n!!\n", "fastq"},
			{"long sequence", "@r\n" + strings.Repeat("A", 300000) + "\n+\n" + strings.Repeat("!", 300000) + "\n", "fastq"},
			{"sequence starts at boundary", "@" + strings.Repeat("r", size-2) + "\nAC\n+\n!!\n", "fastq"},
			{"separator starts at boundary", "@r\n" + strings.Repeat("A", size-4) + "\n+\n" + strings.Repeat("!", size-4) + "\n", "fastq"},
			{"long separator", "@r\nAC\n+" + strings.Repeat("r", 300000) + "\n!!\n", "fastq"},
			{"invalid separator", "@r\nAC\ninvalid" + strings.Repeat("r", 300000), ""},
			{"empty sequence", "@r\n\n+" + strings.Repeat("r", 300000), ""},
			{"truncated header", "@r", ""},
			{"truncated sequence", "@r\nAC", ""},
			{"truncated separator", "@r\nAC\n+", ""},
		} {
			t.Run(fmt.Sprintf("%d/%s", size, tc.name), func(t *testing.T) {
				r := bufio.NewReaderSize(strings.NewReader(tc.input), size)
				got, err := Format(r)
				if tc.want == "" {
					if !errors.Is(err, ErrUnknownFormat) {
						t.Fatalf("Format() = (%q, %v), want ErrUnknownFormat", got, err)
					}
				} else if err != nil || got != tc.want {
					t.Fatalf("Format() = (%q, %v), want %q", got, err, tc.want)
				}
				remaining, err := io.ReadAll(r)
				if err != nil || string(remaining) != tc.input {
					t.Fatalf("Format() consumed or changed input: ReadAll error = %v", err)
				}
			})
		}
	}
}

func TestFormatSAMPeekBoundaries(t *testing.T) {
	prefix := "read1\t16\tref\t1\t60\t300000M\t*\t0\t0\t"
	for _, size := range []int{4096, PeekSize} {
		for _, tc := range []struct {
			name, input, want string
		}{
			{"long sequence", prefix + strings.Repeat("A", 300000) + "\t" + strings.Repeat("!", 300000) + "\n", "sam"},
			{"sequence starts at boundary", strings.Repeat("r", size-len(prefix)+len("read1")) + strings.TrimPrefix(prefix, "read1") + "AC\t!!\n", "sam"},
			{"quality starts at boundary", prefix + strings.Repeat("A", size-len(prefix)-1) + "\t!!\n", "sam"},
			{"invalid flag", strings.Replace(prefix, "\t16\t", "\tX\t", 1) + strings.Repeat("A", 300000), ""},
			{"invalid position", strings.Replace(prefix, "\t1\t", "\tX\t", 1) + strings.Repeat("A", 300000), ""},
			{"invalid mate position", strings.Replace(prefix, "\t0\t0\t", "\tX\t0\t", 1) + strings.Repeat("A", 300000), ""},
			{"missing quality at EOF", prefix + "AC", ""},
			{"missing quality before newline", prefix + "AC\n" + strings.Repeat("A", 300000), ""},
		} {
			t.Run(fmt.Sprintf("%d/%s", size, tc.name), func(t *testing.T) {
				r := bufio.NewReaderSize(strings.NewReader(tc.input), size)
				got, err := Format(r)
				if tc.want == "" {
					if !errors.Is(err, ErrUnknownFormat) {
						t.Fatalf("Format() = (%q, %v), want ErrUnknownFormat", got, err)
					}
				} else if err != nil || got != tc.want {
					t.Fatalf("Format() = (%q, %v), want %q", got, err, tc.want)
				}
				remaining, err := io.ReadAll(r)
				if err != nil || string(remaining) != tc.input {
					t.Fatalf("Format() consumed or changed input: ReadAll error = %v", err)
				}
			})
		}
	}
}

func TestHelperDetectors(t *testing.T) {
	if looksLikeFASTQ([]byte("@r1\nAC\n"), false) {
		t.Fatal("looksLikeFASTQ() = true, want false for truncated record")
	}
	if !looksLikeFASTQ([]byte("@r1\nAC\n+\n!!\n"), false) {
		t.Fatal("looksLikeFASTQ() = false, want true")
	}
	if looksLikeSAM([]byte("read1\tX\t*\t0\t0\t*\t*\t0\t0\tACGT\t!!!!\n"), false) {
		t.Fatal("looksLikeSAM() = true, want false for non-integer flag")
	}
	if !looksLikeSAM([]byte("read1\t16\t*\t0\t0\t*\t*\t0\t0\tACGT\t!!!!\n"), false) {
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
	if !isShortPeek(fmt.Errorf("wrapped: %w", io.EOF)) {
		t.Fatal("isShortPeek(wrapped EOF) = false, want true")
	}
	if isShortPeek(errors.New("boom")) {
		t.Fatal("isShortPeek(boom) = true, want false")
	}
}
