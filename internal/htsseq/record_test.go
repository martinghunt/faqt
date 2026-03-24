package htsseq

import (
	"testing"

	htssam "github.com/biogo/hts/sam"
)

func TestFromSAMRecord(t *testing.T) {
	if got := FromSAMRecord(nil); got != nil {
		t.Fatalf("FromSAMRecord(nil) = %+v, want nil", got)
	}

	forward, err := htssam.NewRecord("read1", nil, nil, -1, -1, 0, 0, nil, []byte("ACGT"), []byte{0xff, 0xff, 0xff, 0xff}, nil)
	if err != nil {
		t.Fatalf("NewRecord(forward) error = %v", err)
	}
	got := FromSAMRecord(forward)
	if got.Name != "read1" || string(got.Seq) != "ACGT" || got.Qual != nil {
		t.Fatalf("forward record = %+v", got)
	}

	reverse, err := htssam.NewRecord("read2", nil, nil, -1, -1, 0, 0, nil, []byte("ATGC"), []byte{32, 33, 34, 35}, nil)
	if err != nil {
		t.Fatalf("NewRecord(reverse) error = %v", err)
	}
	reverse.Flags = htssam.Reverse
	got = FromSAMRecord(reverse)
	if got.Name != "read2" || string(got.Seq) != "GCAT" || string(got.Qual) != "DCBA" {
		t.Fatalf("reverse record = %+v", got)
	}
}

func TestConvertQualAndReverseBytes(t *testing.T) {
	if got := convertQual(nil); got != nil {
		t.Fatalf("convertQual(nil) = %v, want nil", got)
	}
	if got := string(convertQual([]byte{30, 31})); got != "?@" {
		t.Fatalf("convertQual() = %q, want %q", got, "?@")
	}

	buf := []byte("ABCD")
	reverseBytes(buf)
	if got := string(buf); got != "DCBA" {
		t.Fatalf("reverseBytes() = %q, want DCBA", got)
	}
}
