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
}

func TestSubseq(t *testing.T) {
	got, err := seq.Subseq([]byte("ACGT"), 1, 3)
	if err != nil {
		t.Fatalf("Subseq() error = %v", err)
	}
	if string(got) != "CG" {
		t.Fatalf("Subseq() = %q", got)
	}
	if _, err := seq.Subseq([]byte("ACGT"), -1, 3); err == nil {
		t.Fatal("Subseq() expected error for invalid bounds")
	}
}

func TestFindGaps(t *testing.T) {
	got := seq.FindGaps([]byte("AA--NN.TT"), 2)
	want := []seq.Interval{{Start: 2, End: 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindGaps() = %#v, want %#v", got, want)
	}
}
