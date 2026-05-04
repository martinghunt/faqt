package seq_test

import (
	"reflect"
	"testing"

	"github.com/martinghunt/faqt/seq"
)

func TestNewInterval(t *testing.T) {
	got, err := seq.NewInterval(2, 5)
	if err != nil {
		t.Fatalf("NewInterval() error = %v", err)
	}
	if got != (seq.Interval{Start: 2, End: 5}) {
		t.Fatalf("NewInterval() = %#v", got)
	}
	if _, err := seq.NewInterval(-1, 5); err == nil {
		t.Fatal("NewInterval() expected error for negative start")
	}
	if _, err := seq.NewInterval(5, 4); err == nil {
		t.Fatal("NewInterval() expected error for end before start")
	}
}

func TestIntervalLenAndEmpty(t *testing.T) {
	tests := []struct {
		name  string
		in    seq.Interval
		len   int
		empty bool
	}{
		{name: "non-empty", in: seq.Interval{Start: 2, End: 5}, len: 3, empty: false},
		{name: "empty", in: seq.Interval{Start: 2, End: 2}, len: 0, empty: true},
		{name: "invalid defensive length", in: seq.Interval{Start: 5, End: 2}, len: 0, empty: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.in.Len(); got != test.len {
				t.Fatalf("Len() = %d, want %d", got, test.len)
			}
			if got := test.in.Empty(); got != test.empty {
				t.Fatalf("Empty() = %v, want %v", got, test.empty)
			}
		})
	}
}

func TestIntervalContainsUsesHalfOpenCoordinates(t *testing.T) {
	interval := seq.Interval{Start: 2, End: 5}

	for _, pos := range []int{2, 3, 4} {
		if !interval.Contains(pos) {
			t.Fatalf("Contains(%d) = false, want true", pos)
		}
	}
	for _, pos := range []int{1, 5} {
		if interval.Contains(pos) {
			t.Fatalf("Contains(%d) = true, want false", pos)
		}
	}
}

func TestIntervalContainsInterval(t *testing.T) {
	interval := seq.Interval{Start: 2, End: 8}
	tests := []struct {
		other seq.Interval
		want  bool
	}{
		{other: seq.Interval{Start: 2, End: 8}, want: true},
		{other: seq.Interval{Start: 3, End: 7}, want: true},
		{other: seq.Interval{Start: 2, End: 2}, want: true},
		{other: seq.Interval{Start: 1, End: 7}, want: false},
		{other: seq.Interval{Start: 3, End: 9}, want: false},
	}

	for _, test := range tests {
		if got := interval.ContainsInterval(test.other); got != test.want {
			t.Fatalf("ContainsInterval(%#v) = %v, want %v", test.other, got, test.want)
		}
	}
}

func TestIntervalIntersectsUsesHalfOpenCoordinates(t *testing.T) {
	interval := seq.Interval{Start: 2, End: 5}
	tests := []struct {
		other seq.Interval
		want  bool
	}{
		{other: seq.Interval{Start: 0, End: 2}, want: false},
		{other: seq.Interval{Start: 0, End: 3}, want: true},
		{other: seq.Interval{Start: 4, End: 7}, want: true},
		{other: seq.Interval{Start: 5, End: 7}, want: false},
		{other: seq.Interval{Start: 3, End: 3}, want: false},
	}

	for _, test := range tests {
		if got := interval.Intersects(test.other); got != test.want {
			t.Fatalf("Intersects(%#v) = %v, want %v", test.other, got, test.want)
		}
	}
}

func TestIntervalDistanceToPoint(t *testing.T) {
	interval := seq.Interval{Start: 2, End: 5}
	tests := []struct {
		pos  int
		want int
	}{
		{pos: 0, want: 2},
		{pos: 1, want: 1},
		{pos: 2, want: 0},
		{pos: 4, want: 0},
		{pos: 5, want: 1},
		{pos: 7, want: 3},
	}

	for _, test := range tests {
		if got := interval.DistanceToPoint(test.pos); got != test.want {
			t.Fatalf("DistanceToPoint(%d) = %d, want %d", test.pos, got, test.want)
		}
	}
}

func TestMergeOverlappingIntervals(t *testing.T) {
	in := []seq.Interval{
		{Start: 10, End: 12},
		{Start: 2, End: 4},
		{Start: 3, End: 8},
		{Start: 8, End: 9},
		{Start: 20, End: 21},
	}
	want := []seq.Interval{
		{Start: 2, End: 9},
		{Start: 10, End: 12},
		{Start: 20, End: 21},
	}

	got := seq.MergeOverlappingIntervals(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MergeOverlappingIntervals() = %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(in, []seq.Interval{
		{Start: 10, End: 12},
		{Start: 2, End: 4},
		{Start: 3, End: 8},
		{Start: 8, End: 9},
		{Start: 20, End: 21},
	}) {
		t.Fatalf("MergeOverlappingIntervals() mutated input to %#v", in)
	}
}

func TestMergeOverlappingIntervalsEmpty(t *testing.T) {
	if got := seq.MergeOverlappingIntervals(nil); got != nil {
		t.Fatalf("MergeOverlappingIntervals(nil) = %#v, want nil", got)
	}
}

func TestIntervalLengthSum(t *testing.T) {
	intervals := []seq.Interval{
		{Start: 2, End: 9},
		{Start: 10, End: 12},
		{Start: 20, End: 21},
	}
	if got := seq.IntervalLengthSum(intervals); got != 10 {
		t.Fatalf("IntervalLengthSum() = %d, want 10", got)
	}
}
