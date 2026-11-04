package align

import "testing"

func TestNearContainedThreshold(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		for _, tc := range []struct {
			name          string
			first, second int
			suppress      bool
		}{
			{"short_alternative_between_95_and_99", 36, 0, true},
			{"exactly_95_percent", 50, 50, true},
			{"below_95_percent", 50, 51, false},
			{"internal_100_bp_detour", 100, 0, true},
		} {
			t.Run(tc.name+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				ops := []Op{{'=', 500}, {'I', 1}, {'=', tc.first}, {'D', 1}, {'=', 500}}
				if tc.second > 0 {
					ops = append(ops, Op{'I', 1}, Op{'=', tc.second}, Op{'D', 1})
				}
				ops = append(ops, Op{'=', 1000 - tc.first - tc.second})
				small := containmentFixture(0, 0, reverse, ops)
				big := containmentFixture(0, 0, reverse, []Op{{'=', small.A.End}})
				if got := nearContained(pairedMatch(big), pairedMatch(small), 100); got != tc.suppress {
					t.Fatalf("suppression=%v, want %v", got, tc.suppress)
				}
			})
		}
	}
	for _, reverse := range []bool{false, true} {
		small := containmentFixture(0, 0, reverse, []Op{{'=', 900}, {'I', 1}, {'=', 100}})
		big := containmentFixture(0, 0, reverse, []Op{{'=', small.A.End}})
		if nearContained(pairedMatch(big), pairedMatch(small), 100) {
			t.Fatalf("reverse=%v: novel 100 bp end stretch was suppressed", reverse)
		}
	}
}
