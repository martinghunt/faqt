package seq

import (
	"fmt"
	"slices"
)

// Interval represents a 0-based half-open coordinate range [Start, End).
// Start is included, End is excluded.
type Interval struct {
	Start int
	End   int
}

func NewInterval(start, end int) (Interval, error) {
	i := Interval{Start: start, End: end}
	if err := i.Validate(); err != nil {
		return Interval{}, err
	}
	return i, nil
}

func (i Interval) Validate() error {
	if i.Start < 0 {
		return fmt.Errorf("interval start %d is negative", i.Start)
	}
	if i.End < i.Start {
		return fmt.Errorf("interval end %d is before start %d", i.End, i.Start)
	}
	return nil
}

func (i Interval) Len() int {
	if i.End <= i.Start {
		return 0
	}
	return i.End - i.Start
}

func (i Interval) Empty() bool {
	return i.Len() == 0
}

func (i Interval) Contains(pos int) bool {
	return i.Start <= pos && pos < i.End
}

func (i Interval) ContainsInterval(j Interval) bool {
	return i.Start <= j.Start && j.End <= i.End
}

func (i Interval) Intersects(j Interval) bool {
	if i.Empty() || j.Empty() {
		return false
	}
	return i.Start < j.End && j.Start < i.End
}

func (i Interval) DistanceToPoint(pos int) int {
	if i.Contains(pos) {
		return 0
	}
	if pos < i.Start {
		return i.Start - pos
	}
	return pos - i.End + 1
}

func MergeOverlappingIntervals(intervals []Interval) []Interval {
	if len(intervals) == 0 {
		return nil
	}

	merged := append([]Interval(nil), intervals...)
	slices.SortFunc(merged, func(a, b Interval) int {
		if a.Start != b.Start {
			return a.Start - b.Start
		}
		return a.End - b.End
	})

	out := merged[:1]
	for _, interval := range merged[1:] {
		last := &out[len(out)-1]
		// Match pyfastaq merge_overlapping_in_list behavior: merge intervals
		// that overlap, plus adjacent closed intervals after conversion to
		// half-open coordinates.
		if interval.Start <= last.End {
			if interval.End > last.End {
				last.End = interval.End
			}
			continue
		}
		out = append(out, interval)
	}

	return out
}

func IntervalLengthSum(intervals []Interval) int {
	total := 0
	for _, interval := range intervals {
		total += interval.Len()
	}
	return total
}
