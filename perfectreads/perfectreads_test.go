package perfectreads

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestQualityProfile(t *testing.T) {
	if got := string(qualityProfile(12)); got != "IIIIHGFEDCBA" {
		t.Fatalf("qualityProfile(12) = %q", got)
	}
	if got := string(qualityProfile(4)); got != "DCBA" {
		t.Fatalf("qualityProfile(4) = %q", got)
	}
}

func TestGeneratePairedReads(t *testing.T) {
	reader, err := seqio.OpenReader(strings.NewReader(">chr1\nACGTACGTACGTACGTACGTACGTACGTACGT\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	var fwd, rev bytes.Buffer
	fw := seqio.NewFASTQWriter(&fwd)
	rw := seqio.NewFASTQWriter(&rev)
	report, err := GeneratePaired(reader, fw, rw, Options{
		MeanInsert: 12,
		InsertStd:  0,
		Coverage:   1,
		ReadLength: 4,
		Seed:       1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.PairsWritten != 4 {
		t.Fatalf("PairsWritten = %d, want 4", report.PairsWritten)
	}
	if !strings.Contains(fwd.String(), "@chr1:1:10:18/1") {
		t.Fatalf("forward output = %q", fwd.String())
	}
	if !strings.Contains(rev.String(), "/2\n") {
		t.Fatalf("reverse output = %q", rev.String())
	}
	if !strings.Contains(fwd.String(), "DCBA") || !strings.Contains(rev.String(), "DCBA") {
		t.Fatalf("expected quality tail in outputs")
	}
}

func TestGenerateSingleReads(t *testing.T) {
	reader, err := seqio.OpenReader(strings.NewReader(">chr1\nACGTACGTACGTACGTACGTACGTACGTACGT\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	var out bytes.Buffer
	report, err := GenerateSingle(reader, seqio.NewFASTQWriter(&out), Options{
		Coverage:   1,
		ReadLength: 4,
		Seed:       1,
	})
	if err != nil {
		t.Fatalf("GenerateSingle() error = %v", err)
	}
	if report.ReadsWritten != 8 {
		t.Fatalf("ReadsWritten = %d, want 8", report.ReadsWritten)
	}
	if !strings.Contains(out.String(), "@chr1:1:") {
		t.Fatalf("single output = %q", out.String())
	}
	if !strings.Contains(out.String(), "DCBA") {
		t.Fatalf("expected quality tail in output")
	}
}

func TestGenerateNoNSkipsPairs(t *testing.T) {
	reader, err := seqio.OpenReader(strings.NewReader(">chr1\nACGTNNNNACGTNNNNACGTNNNNACGTNNNN\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	var fwd, rev bytes.Buffer
	report, err := GeneratePaired(reader, seqio.NewFASTQWriter(&fwd), seqio.NewFASTQWriter(&rev), Options{
		MeanInsert: 8,
		InsertStd:  0,
		Coverage:   1,
		ReadLength: 4,
		NoN:        true,
		Seed:       1,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if report.PairsWritten != 0 {
		t.Fatalf("PairsWritten = %d, want 0", report.PairsWritten)
	}
}
