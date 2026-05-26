package orf_test

import (
	"testing"

	"github.com/martinghunt/faqt/orf"
)

func TestFindORFsForwardAndReverse(t *testing.T) {
	seq := []byte("CCCATGAAATAGGGGTTATTTCAT")
	got := orf.FindORFs(seq, orf.ORFOptions{MinLength: 9, BothStrands: true})
	if len(got) < 2 {
		t.Fatalf("FindORFs() returned %d ORFs, want at least 2", len(got))
	}
	if got[0].Strand != "+" {
		t.Fatalf("first ORF strand = %q, want +", got[0].Strand)
	}
	foundReverse := false
	for _, entry := range got {
		if entry.Strand == "-" {
			foundReverse = true
		}
	}
	if !foundReverse {
		t.Fatal("FindORFs() did not report a reverse-strand ORF")
	}
}

func TestFindORFsCustomStartCodons(t *testing.T) {
	got := orf.FindORFs([]byte("TTGAAATAG"), orf.ORFOptions{
		MinLength:   9,
		StartCodons: []string{"TTG"},
	})
	if len(got) != 1 {
		t.Fatalf("FindORFs() returned %d ORFs, want 1", len(got))
	}
}

func TestMakeIntoGeneMatchesFastaqOrder(t *testing.T) {
	tests := []struct {
		name       string
		in         string
		wantSeq    string
		wantStrand string
		wantFrame  int
		wantOK     bool
	}{
		{name: "too short", in: "TTG", wantOK: false},
		{name: "forward frame0 exact", in: "TTGAAATAA", wantSeq: "TTGAAATAA", wantStrand: "+", wantFrame: 0, wantOK: true},
		{name: "forward frame0 trims overhang", in: "TTGTAAA", wantSeq: "TTGTAA", wantStrand: "+", wantFrame: 0, wantOK: true},
		{name: "forward frame1", in: "ATTGTAA", wantSeq: "TTGTAA", wantStrand: "+", wantFrame: 1, wantOK: true},
		{name: "forward frame2", in: "AATTGTAA", wantSeq: "TTGTAA", wantStrand: "+", wantFrame: 2, wantOK: true},
		{name: "reverse frame0", in: "TTACAA", wantSeq: "TTGTAA", wantStrand: "-", wantFrame: 0, wantOK: true},
		{name: "reverse frame1", in: "TTACAAA", wantSeq: "TTGTAA", wantStrand: "-", wantFrame: 1, wantOK: true},
		{name: "reverse frame2", in: "TTACAAAA", wantSeq: "TTGTAA", wantStrand: "-", wantFrame: 2, wantOK: true},
		{name: "bad terminal", in: "TTGAAATAT", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := orf.MakeIntoGene([]byte(test.in), orf.GeneOptions{GeneticCode: 1})
			if ok != test.wantOK {
				t.Fatalf("MakeIntoGene() ok = %v, want %v", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if string(got.Seq) != test.wantSeq || got.Strand != test.wantStrand || got.Frame != test.wantFrame {
				t.Fatalf("MakeIntoGene() = %#v, want seq=%q strand=%q frame=%d", got, test.wantSeq, test.wantStrand, test.wantFrame)
			}
		})
	}
}

func TestMakeIntoGeneGeneticCodeStarts(t *testing.T) {
	if _, ok := orf.MakeIntoGene([]byte("ATTCAGTAA"), orf.GeneOptions{GeneticCode: 1}); ok {
		t.Fatal("MakeIntoGene() accepted ATT start in code 1")
	}
	if got, ok := orf.MakeIntoGene([]byte("ATTCAGTAA"), orf.GeneOptions{GeneticCode: 11}); !ok || string(got.Seq) != "ATTCAGTAA" {
		t.Fatalf("MakeIntoGene() with code 11 = %#v/%v, want ATT gene", got, ok)
	}
	if _, ok := orf.MakeIntoGene([]byte("ATGTGATAA"), orf.GeneOptions{GeneticCode: 1}); ok {
		t.Fatal("MakeIntoGene() accepted TGA as internal stop in code 1")
	}
	if got, ok := orf.MakeIntoGene([]byte("ATGTGATAA"), orf.GeneOptions{GeneticCode: 4}); !ok || string(got.Seq) != "ATGTGATAA" {
		t.Fatalf("MakeIntoGene() with code 4 = %#v/%v, want TGA translated as W", got, ok)
	}
}
