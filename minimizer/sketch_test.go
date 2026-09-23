package minimizer

import (
	"bytes"
	"reflect"
	"testing"
)

func TestWindowMinimumUsesCircularOrderForTies(t *testing.T) {
	buffer := []Minimizer{
		{Hash: 5, Pos: 0},
		{Hash: 2, Pos: 1},
		{Hash: 2, Pos: 2},
		{Hash: 7, Pos: 3},
	}

	got, gotPos := windowMinimum(buffer, 1)
	if got != buffer[1] || gotPos != 1 {
		t.Fatalf("windowMinimum() = %#v at %d, want %#v at 1", got, gotPos, buffer[1])
	}
}

func TestAppendMatchingMinimizersPreservesCandidateOrder(t *testing.T) {
	min := Minimizer{Hash: 2, Pos: 1}
	candidates := []Minimizer{
		{Hash: 2, Pos: 4},
		{Hash: 3, Pos: 5},
		min,
		{Hash: 2, Pos: 6},
	}

	got := appendMatchingMinimizers(nil, candidates, min)
	want := []Minimizer{{Hash: 2, Pos: 4}, {Hash: 2, Pos: 6}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("appendMatchingMinimizers() = %#v, want %#v", got, want)
	}
}

func BenchmarkSketch(b *testing.B) {
	seq := bytes.Repeat([]byte("ACGTTGCATGTCGCATGATGCATGAGAGCT"), 1024)
	b.ReportAllocs()
	b.SetBytes(int64(len(seq)))
	for b.Loop() {
		Sketch(seq, 15, 10)
	}
}
