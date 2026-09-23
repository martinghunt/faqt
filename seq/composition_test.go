package seq_test

import (
	"math"
	"testing"

	"github.com/martinghunt/faqt/seq"
)

func TestCountComposition(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want seq.Composition
	}{
		{
			name: "empty",
			in:   "",
			want: seq.Composition{},
		},
		{
			name: "plain DNA",
			in:   "ACGTACGG",
			want: seq.Composition{A: 2, C: 2, G: 3, T: 1},
		},
		{
			name: "case is ignored",
			in:   "acgtACGT",
			want: seq.Composition{A: 2, C: 2, G: 2, T: 2},
		},
		{
			name: "U counts as T",
			in:   "ACGUacgu",
			want: seq.Composition{A: 2, C: 2, G: 2, T: 2},
		},
		{
			name: "ambiguity codes and gaps are counted as neither",
			in:   "ACGT-RYKMSWBDHV.",
			want: seq.Composition{A: 1, C: 1, G: 1, T: 1},
		},
		{
			name: "runs of Ns",
			in:   "NNACnnGTN",
			want: seq.Composition{A: 1, C: 1, G: 1, T: 1, N: 5, NRuns: 3},
		},
		{
			name: "one N run spanning the whole sequence",
			in:   "nnnNNN",
			want: seq.Composition{N: 6, NRuns: 1},
		},
		{
			name: "single Ns are runs of one",
			in:   "ANANA",
			want: seq.Composition{A: 3, N: 2, NRuns: 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := seq.CountComposition([]byte(tc.in)); got != tc.want {
				t.Fatalf("CountComposition(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompositionGCPercent(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{name: "empty is zero", in: "", want: 0},
		{name: "all GC", in: "GGCC", want: 100},
		{name: "no GC", in: "AATT", want: 0},
		{name: "half", in: "ACGT", want: 50},
		{name: "Ns are left out of the denominator", in: "GCNNNNNN", want: 100},
		{name: "ambiguity codes are left out of the denominator", in: "GCATRYSW", want: 50},
		{name: "gaps are left out of the denominator", in: "GC--AT--", want: 50},
		{name: "nothing but Ns is zero, not undefined", in: "NNNN", want: 0},
		{name: "soft-masked bases count", in: "gcAT", want: 50},
		{name: "one in three", in: "ACT", want: 1.0 / 3.0 * 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := seq.CountComposition([]byte(tc.in)).GCPercent()
			if math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("GCPercent(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestCompositionAdd(t *testing.T) {
	// Ns at the end of one sequence and the start of the next stay two runs.
	total := seq.CountComposition([]byte("ACGTNN"))
	total.Add(seq.CountComposition([]byte("NNGGGG")))

	want := seq.Composition{A: 1, C: 1, G: 5, T: 1, N: 4, NRuns: 2}
	if total != want {
		t.Fatalf("Add() = %+v, want %+v", total, want)
	}
	if got, want := total.ACGT(), 8; got != want {
		t.Fatalf("ACGT() = %d, want %d", got, want)
	}
	if got, want := total.GCPercent(), 75.0; got != want {
		t.Fatalf("GCPercent() = %v, want %v", got, want)
	}
}
