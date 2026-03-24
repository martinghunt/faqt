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
