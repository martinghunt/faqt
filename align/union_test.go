package align

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// A placement that starts one tandem copy over and then rejoins the main
// diagonal is trimmed to its copy-to-copy pairing, and is redundant once that
// pairing is reported on its own.
func TestUnionCoveredTandemHop(t *testing.T) {
	unit := dna(100, 91)
	s := append(append(dna(1500, 92), []byte(strings.Repeat(string(unit), 3))...), dna(1500, 93)...)
	g := genome(t, s)
	h := genome(t, append(slices.Clone(s), dna(50, 94)...))
	main := containmentFixture(0, 0, false, []Op{{'=', len(s)}})
	hop := containmentFixture(1600, 1500, false, []Op{{'=', 100}, {'D', 100}, {'=', 1500}})
	copyPair := containmentFixture(1600, 1500, false, []Op{{'=', 150}})
	trimmed := containmentFixture(1600, 1500, false, []Op{{'=', 100}})
	for _, tc := range []struct {
		name        string
		input, want []Match
	}{
		{"covered", []Match{main, hop, copyPair}, []Match{main, copyPair}},
		{"uncovered", []Match{main, hop}, []Match{main, trimmed}},
	} {
		got, err := cleanRedundantMatches(context.Background(), g, h, tc.input, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		SortMatches(tc.want)
		if len(got) != len(tc.want) {
			t.Fatalf("%s: got %+v", tc.name, got)
		}
		for i := range got {
			if got[i].A != tc.want[i].A || got[i].B != tc.want[i].B || got[i].CIGAR() != tc.want[i].CIGAR() || got[i].Score != tc.want[i].Score {
				t.Fatalf("%s: got %+v, want %+v", tc.name, got, tc.want)
			}
			replay(t, g, h, got[i], DefaultOptions())
		}
	}
}
