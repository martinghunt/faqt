package seqio_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestInterleaveAlternatesRecordsAndAddsSuffixes(t *testing.T) {
	reader1, err := seqio.OpenReader(strings.NewReader(">r1\nAC\n>r2/1\nGT\n"))
	if err != nil {
		t.Fatalf("OpenReader(reader1) error = %v", err)
	}
	reader2, err := seqio.OpenReader(strings.NewReader(">r1\nTG\n>r2/2\nCA\n"))
	if err != nil {
		t.Fatalf("OpenReader(reader2) error = %v", err)
	}
	var out bytes.Buffer
	writer := seqio.NewFASTAWriter(&out)

	err = seqio.Interleave(reader1, reader2, writer, seqio.InterleaveOptions{Suffix1: "/1", Suffix2: "/2"})
	if err != nil {
		t.Fatalf("Interleave() error = %v", err)
	}

	want := ">r1/1\nAC\n>r1/2\nTG\n>r2/1\nGT\n>r2/2\nCA\n"
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}

func TestInterleaveErrorsWhenFirstInputHasExtraRecord(t *testing.T) {
	reader1, err := seqio.OpenReader(strings.NewReader(">r1\nAC\n>r2\nGT\n"))
	if err != nil {
		t.Fatalf("OpenReader(reader1) error = %v", err)
	}
	reader2, err := seqio.OpenReader(strings.NewReader(">r1\nTG\n"))
	if err != nil {
		t.Fatalf("OpenReader(reader2) error = %v", err)
	}
	var out bytes.Buffer
	writer := seqio.NewFASTAWriter(&out)

	err = seqio.Interleave(reader1, reader2, writer, seqio.InterleaveOptions{})
	if err == nil || !strings.Contains(err.Error(), "r2") {
		t.Fatalf("Interleave() error = %v, want unmatched r2", err)
	}
}

func TestInterleaveErrorsWhenSecondInputHasExtraRecord(t *testing.T) {
	reader1, err := seqio.OpenReader(strings.NewReader(">r1\nAC\n"))
	if err != nil {
		t.Fatalf("OpenReader(reader1) error = %v", err)
	}
	reader2, err := seqio.OpenReader(strings.NewReader(">r1\nTG\n>r2\nCA\n"))
	if err != nil {
		t.Fatalf("OpenReader(reader2) error = %v", err)
	}
	var out bytes.Buffer
	writer := seqio.NewFASTAWriter(&out)

	err = seqio.Interleave(reader1, reader2, writer, seqio.InterleaveOptions{})
	if err == nil || !strings.Contains(err.Error(), "r2") {
		t.Fatalf("Interleave() error = %v, want unmatched r2", err)
	}
}

func TestInterleavePathDetectsFASTQOutput(t *testing.T) {
	dir := t.TempDir()
	input1 := dir + "/input1.dat"
	input2 := dir + "/input2.dat"
	output := dir + "/out.dat"
	if err := os.WriteFile(input1, []byte("@r1\nAC\n+\n!!\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(input1) error = %v", err)
	}
	if err := os.WriteFile(input2, []byte("@r1\nTG\n+\n##\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(input2) error = %v", err)
	}

	err := seqio.InterleavePath(input1, input2, output, seqio.InterleaveOptions{Suffix1: "/1", Suffix2: "/2"})
	if err != nil {
		t.Fatalf("InterleavePath() error = %v", err)
	}

	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile(output) error = %v", err)
	}
	got := string(data)
	want := "@r1/1\nAC\n+\n!!\n@r1/2\nTG\n+\n##\n"
	if got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}
