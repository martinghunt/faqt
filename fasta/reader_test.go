package fasta

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReaderRejectsNULBytes(t *testing.T) {
	tests := []struct {
		name  string
		input string
		line  int
	}{
		{"name", ">seq\x00one\nACGT\n", 1},
		{"description", ">seq description\x00here\nACGT\n", 1},
		{"sequence start", ">seq\n\x00ACGT\n", 2},
		{"sequence middle", ">seq\nAC\x00GT\n", 2},
		{"sequence end without newline", ">seq\nACGT\x00", 2},
		{"NUL only line", ">seq\n\x00\n", 2},
		{"wrapped sequence", ">seq\nACGT\nAC\x00GT\n", 3},
		{"long sequence", ">seq\n" + strings.Repeat("A", 10000) + "\x00GT\n", 2},
		{"long header", ">seq " + strings.Repeat("d", 10000) + "\x00\nACGT\n", 1},
		{"later record", ">one\nACGT\n>two\nAC\x00GT\n", 4},
		{"later header", ">one\nACGT\n>two\x00\nACGT\n", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReader(bufio.NewReader(strings.NewReader(tc.input)))
			for {
				rec, err := r.Read()
				if err == nil {
					continue
				}
				want := fmt.Sprintf("fasta: NUL byte (0x00) on line %d", tc.line)
				if !strings.HasPrefix(err.Error(), want+",") {
					t.Fatalf("Read() error = %v, want %q", err, want)
				}
				if rec != nil {
					t.Fatalf("Read() returned a record with invalid input: %+v", rec)
				}
				break
			}
		})
	}
}

func TestReaderMultiRecord(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader(">seq1 desc one\nACG\nT\n>seq2\nNN-\n")))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "seq1" || rec.Description != "desc one" || string(rec.Seq) != "ACGT" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "seq2" || string(rec.Seq) != "NN-" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}

// TestReaderLongLines covers sequence and header lines longer than the read
// buffer, which are stitched together from several ReadSlice calls.
func TestReaderLongLines(t *testing.T) {
	longSeq := strings.Repeat("ACGT", 200000) // 800 kB, far past any buffer
	longDesc := strings.Repeat("d", 300000)
	input := ">rec1 " + longDesc + "\n" + longSeq + "\n>rec2\nACGT\n"

	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "rec1" || rec.Description != longDesc {
		t.Fatalf("first record name = %q, description length = %d", rec.Name, len(rec.Description))
	}
	if string(rec.Seq) != longSeq {
		t.Fatalf("first record sequence length = %d, want %d", len(rec.Seq), len(longSeq))
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "ACGT" {
		t.Fatalf("second record = %+v", rec)
	}
	if _, err := r.Read(); err != io.EOF {
		t.Fatalf("Read() third error = %v, want io.EOF", err)
	}
}

// TestReaderLongWrappedLines mixes a long line with wrapped lines in one
// record, so the stitched scratch buffer and the accumulator must not collide.
func TestReaderLongWrappedLines(t *testing.T) {
	long := strings.Repeat("A", 500000)
	input := ">rec1\nACGT\n" + long + "\nGGGG\n>rec2\nTTTT\n"

	r := NewReader(bufio.NewReader(strings.NewReader(input)))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	want := "ACGT" + long + "GGGG"
	if string(rec.Seq) != want {
		t.Fatalf("sequence length = %d, want %d", len(rec.Seq), len(want))
	}
	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "TTTT" {
		t.Fatalf("second record = %+v", rec)
	}
}

// TestReaderRecordsStayValid checks the ownership contract: a record must not
// change when later records are read, even though the Reader reuses buffers.
func TestReaderRecordsStayValid(t *testing.T) {
	input := ">rec1\nACGT\n>rec2\nNNNNNNNN\n>rec3\nTT\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	first, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	second, err := r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if _, err := r.Read(); err != nil {
		t.Fatalf("Read() third error = %v", err)
	}
	if first.Name != "rec1" || string(first.Seq) != "ACGT" {
		t.Fatalf("first record changed: %+v", first)
	}
	if second.Name != "rec2" || string(second.Seq) != "NNNNNNNN" {
		t.Fatalf("second record changed: %+v", second)
	}
}

// TestReaderCRLF checks Windows line endings are trimmed from headers and
// sequence lines.
func TestReaderCRLF(t *testing.T) {
	input := ">rec1 desc\r\nACGT\r\nAAAA\r\n>rec2\r\nTT\r\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "rec1" || rec.Description != "desc" || string(rec.Seq) != "ACGTAAAA" {
		t.Fatalf("record = %+v", rec)
	}
	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "rec2" || string(rec.Seq) != "TT" {
		t.Fatalf("second record = %+v", rec)
	}
}

func TestCheckReportsNULsAndKeepsNormalReadsStrict(t *testing.T) {
	r := NewReader(bufio.NewReader(strings.NewReader(">one\nA\x00C\n>two\x00\nT\x00\n")))
	var diagnostics []error
	count, err := r.Check(func(err error) { diagnostics = append(diagnostics, err) })
	if err != nil || count != 2 || len(diagnostics) != 3 {
		t.Fatalf("Check() = (%d, %v), diagnostics = %v", count, err, diagnostics)
	}
	if r.report != nil {
		t.Fatal("Check() left diagnostic mode enabled")
	}
}

func TestCheckNULRunPositions(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"multiple runs on one line", ">one description\n\x00\x00AC\x00G\x00\x00\n", []string{
			`line 2, contig "one", sequence positions 1-2 (2 bytes; line:column 2:1-2:2)`,
			`line 2, contig "one", sequence positions 5-5 (1 bytes; line:column 2:5-2:5)`,
			`line 2, contig "one", sequence positions 7-8 (2 bytes; line:column 2:7-2:8)`,
		}},
		{"wrapped and multiple records", ">one\r\nAC\x00\r\n\x00\x00T\x00\r\n>two\r\nG\x00", []string{
			`line 2, contig "one", sequence positions 3-5 (3 bytes; line:column 2:3-3:2)`,
			`line 3, contig "one", sequence positions 7-7 (1 bytes; line:column 3:4-3:4)`,
			`line 5, contig "two", sequence positions 2-2 (1 bytes; line:column 5:2-5:2)`,
		}},
		{"trimmed whitespace", ">one\n AC\x00 \n \x00T \n", []string{
			`line 2, contig "one", sequence positions 3-4 (2 bytes; line:column 2:4-3:2)`,
		}},
		{"header runs", ">one bad\x00text\x00\x00\nAC\n>two bad\x00\nTT\n", []string{
			`line 1, contig "one", header columns 9-9 (1 bytes)`,
			`line 1, contig "one", header columns 14-15 (2 bytes)`,
			`line 3, contig "two", header columns 9-9 (1 bytes)`,
		}},
		{"long line", ">long\n" + strings.Repeat("A", 10000) + "\x00\x00T\x00", []string{
			`line 2, contig "long", sequence positions 10001-10002 (2 bytes; line:column 2:10001-2:10002)`,
			`line 2, contig "long", sequence positions 10004-10004 (1 bytes; line:column 2:10004-2:10004)`,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewReader(bufio.NewReader(strings.NewReader(tc.input)))
			var got []string
			_, err := r.Check(func(err error) { got = append(got, err.Error()) })
			if err != nil || len(got) != len(tc.want) {
				t.Fatalf("Check() error=%v, diagnostics=%q, want %q", err, got, tc.want)
			}
			for i, want := range tc.want {
				if got[i] != "fasta: NUL byte (0x00) on "+want {
					t.Errorf("diagnostic %d=%q, want %q", i, got[i], want)
				}
			}
		})
	}
}
