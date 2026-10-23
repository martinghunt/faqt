package align

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestOneSNPEndExtension(t *testing.T) {
	tests := []struct {
		name, query, target string
		span                int
	}{
		{"empty", "", "ACG", 0},
		{"perfect", "ACGT", "ACGT", 4},
		{"initial SNP", "AACGTT", "TACGTT", 6},
		{"trailing SNP", "ACGT", "ACGA", 3},
		{"two SNPs", "AACGTT", "TACGTA", 5},
		{"no score gain", "AAA", "TAA", 0},
		{"ambiguity", "NACGTT", "TACGTT", 0},
		{"indel", "ACGTAC", "ACTGTAC", 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r, err := extendOneSNP(context.Background(), []byte(test.query), []byte(test.target), DefaultOptions())
			if err != nil {
				t.Fatal(err)
			}
			if r.q != test.span || r.t != test.span {
				t.Fatalf("consumed %d/%d, expected %d", r.q, r.t, test.span)
			}
			pos, mismatches, score := 0, 0, 0
			for _, op := range r.ops {
				if op.Code != '=' && op.Code != 'X' {
					t.Fatalf("unexpected indel: %+v", r.ops)
				}
				for i := 0; i < op.Length; i++ {
					equal := exact(test.query[pos], test.target[pos])
					if equal != (op.Code == '=') {
						t.Fatal("incorrect operation")
					}
					pos++
					if equal {
						score += 2
					} else {
						mismatches++
						score -= 4
					}
				}
			}
			if pos != test.span || score != r.score || mismatches > 1 {
				t.Fatalf("invalid result %+v", r)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := extendOneSNP(ctx, []byte("ACG"), []byte("ACG"), DefaultOptions()); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestOneSNPEndsPreserveInternalIndels(t *testing.T) {
	base := dna(1500, 511)
	inserted := append(append(slicesCopy(base[:700]), dna(31, 512)...), base[700:]...)
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			target := inserted
			if reverse {
				target = reverseComplement(target)
			}
			opts := DefaultOptions()
			opts.EndExtension = EndExtensionOneSNP
			matches := compare(t, genome(t, base), genome(t, target), opts)
			found := false
			for _, match := range matches {
				if match.Reverse == reverse && strings.Contains(match.CIGAR(), "31D") {
					found = true
				}
			}
			if !found {
				t.Fatal("fast end policy suppressed internal gapped alignment")
			}
		})
	}
}

func TestOneSNPEndsKeepRepeatPlacements(t *testing.T) {
	sequence := dna(400, 513)
	a := genome(t, sequence, sequence)
	var targets [][]byte
	for i := 0; i < 7; i++ {
		targets = append(targets, sequence)
	}
	targets = append(targets, reverseComplement(sequence))
	opts := DefaultOptions()
	opts.EndExtension = EndExtensionOneSNP
	matches := compare(t, a, genome(t, targets...), opts)
	if len(matches) != 16 {
		t.Fatalf("expected all 2x8 placements, got %d", len(matches))
	}
}

func TestEndExtensionModeValidation(t *testing.T) {
	if DefaultOptions().EndExtension != EndExtensionDP {
		t.Fatal("default sensitivity policy changed")
	}
	opts := DefaultOptions()
	opts.EndExtension = EndExtensionMode(255)
	g := genome(t, dna(100, 514))
	if matches, err := Compare(context.Background(), g, g, opts); err == nil || matches != nil {
		t.Fatal("accepted invalid end-extension policy")
	}
}
