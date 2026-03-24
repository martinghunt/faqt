package seq

func FindGaps(in []byte, minRun int) []Interval {
	if minRun <= 0 {
		minRun = 1
	}
	var out []Interval
	start := -1
	for i, ch := range in {
		if ch == '-' || ch == '.' || ch == 'N' || ch == 'n' {
			if start == -1 {
				start = i
			}
			continue
		}
		if start != -1 && i-start >= minRun {
			out = append(out, Interval{Start: start, End: i})
		}
		start = -1
	}
	if start != -1 && len(in)-start >= minRun {
		out = append(out, Interval{Start: start, End: len(in)})
	}
	return out
}
