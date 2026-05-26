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

	got = string(seq.NormalizeDNA([]byte("ryswkmbdhvu")))
	if got != "RYSWKMBDHVT" {
		t.Fatalf("NormalizeDNA() lowercase ambiguity = %q", got)
	}
}

func TestTranslateCodon(t *testing.T) {
	tests := []struct {
		codon string
		want  byte
	}{
		{codon: "ATG", want: 'M'},
		{codon: "TAA", want: '*'},
		{codon: "NNN", want: 'X'},
		{codon: "ATN", want: 'X'},
		{codon: "atg", want: 'M'},
		{codon: "A-T", want: 'X'},
		{codon: "AT", want: 'X'},
	}
	for _, test := range tests {
		if got := seq.TranslateCodon([]byte(test.codon)); got != test.want {
			t.Fatalf("TranslateCodon(%q) = %q, want %q", test.codon, got, test.want)
		}
	}
}

func TestTranslateCodonWithCode(t *testing.T) {
	if got := seq.TranslateCodonWithCode([]byte("TGA"), 1); got != '*' {
		t.Fatalf("TranslateCodonWithCode(TGA, 1) = %q, want *", got)
	}
	if got := seq.TranslateCodonWithCode([]byte("TGA"), 4); got != 'W' {
		t.Fatalf("TranslateCodonWithCode(TGA, 4) = %q, want W", got)
	}
	if got := seq.TranslateCodonWithCode([]byte("ATG"), 999); got != 'X' {
		t.Fatalf("TranslateCodonWithCode(unknown code) = %q, want X", got)
	}
}

func TestTranslate(t *testing.T) {
	got := string(seq.Translate([]byte("GATCGCGAATGAN")))
	if got != "DRE*" {
		t.Fatalf("Translate() = %q", got)
	}
}

func TestTranslateWithCode(t *testing.T) {
	got := string(seq.TranslateWithCode([]byte("ATGTGATAA"), 4))
	if got != "MW*" {
		t.Fatalf("TranslateWithCode() = %q", got)
	}
}

func TestIsStartCodon(t *testing.T) {
	if !seq.IsStartCodon([]byte("ATT"), 11) {
		t.Fatal("IsStartCodon(ATT, 11) = false, want true")
	}
	if seq.IsStartCodon([]byte("ATT"), 1) {
		t.Fatal("IsStartCodon(ATT, 1) = true, want false")
	}
}

func TestFindGaps(t *testing.T) {
	got := seq.FindGaps([]byte("AA--NN.TT"), 2)
	want := []seq.Interval{{Start: 2, End: 7}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FindGaps() = %#v, want %#v", got, want)
	}
}
