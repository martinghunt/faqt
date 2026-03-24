package seq_test

import (
	"reflect"
	"testing"

	"github.com/martinghunt/faqt/seq"
)

func TestReverseComplement(t *testing.T) {
	got := string(seq.ReverseComplement([]byte("AaCGTNry-")))
	if got != "-ryNACGtT" {
		t.Fatalf("ReverseComplement() = %q", got)
	}

	got = string(seq.ReverseComplement([]byte("AX")))
	if got != "NT" {
		t.Fatalf("ReverseComplement() with unknown base = %q", got)
	}
}

func TestSubseq(t *testing.T) {
	in := []byte("ACGT")
	got, err := seq.Subseq(in, 1, 3)
	if err != nil {
		t.Fatalf("Subseq() error = %v", err)
	}
	if string(got) != "CG" {
		t.Fatalf("Subseq() = %q", got)
	}
	got[0] = 'T'
	if string(in) != "ACGT" {
		t.Fatalf("Subseq() returned alias into input, input = %q", in)
	}
	if _, err := seq.Subseq([]byte("ACGT"), -1, 3); err == nil {
		t.Fatal("Subseq() expected error for invalid bounds")
	}
	if _, err := seq.Subseq([]byte("ACGT"), 3, 2); err == nil {
		t.Fatal("Subseq() expected error for start > end")
	}
	if _, err := seq.Subseq([]byte("ACGT"), 0, 5); err == nil {
		t.Fatal("Subseq() expected error for end beyond input")
	}
}

func TestNormalizeDNA(t *testing.T) {
	in := []byte("acguN-x?")
	got := string(seq.NormalizeDNA(in))
	if got != "ACGTN-X?" {
		t.Fatalf("NormalizeDNA() = %q", got)
	}
	if string(in) != "acguN-x?" {
		t.Fatalf("NormalizeDNA() mutated input = %q", in)
	}
}

func TestFindGaps(t *testing.T) {
	got := seq.FindGaps([]byte("AA--NN.TT"), 2)
	want := []seq.Interval{{Start: 2, End: 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindGaps() = %#v, want %#v", got, want)
	}
}
