package seq

import (
	"reflect"
	"strings"
	"testing"
)

func TestAlphabetAcceptedBytes(t *testing.T) {
	for _, tc := range []struct{ name, letters string }{
		{"dna", "ACGTRYSWKMBDHVN"},
		{"rna", "ACGURYSWKMBDHVN"},
		{"protein", "ABCDEFGHIJKLMNOPQRSTUVWXYZ*"},
	} {
		for _, gaps := range []bool{false, true} {
			a, err := NewAlphabet(tc.name, gaps)
			if err != nil {
				t.Fatal(err)
			}
			allowed := tc.letters + strings.ToLower(tc.letters)
			if gaps {
				allowed += "-."
			}
			for b := 0; b < 256; b++ {
				invalid := false
				a.InvalidRuns([]byte{byte(b)}, func(start, end int) bool { invalid = true; return true })
				if invalid == strings.ContainsRune(allowed, rune(b)) {
					t.Errorf("alphabet=%s gaps=%v byte=0x%02X invalid=%v", tc.name, gaps, b, invalid)
				}
			}
		}
	}
}

func TestAlphabetInvalidRuns(t *testing.T) {
	a, err := NewAlphabet("dna", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input string
		want  [][2]int
	}{
		{"", nil}, {"acgtNRY", nil}, {"?!ACX9G\x00", [][2]int{{0, 2}, {4, 6}, {7, 8}}}, {"AéT", [][2]int{{1, 3}}},
	} {
		var got [][2]int
		input := []byte(tc.input)
		a.InvalidRuns(input, func(start, end int) bool { got = append(got, [2]int{start, end}); return true })
		if !reflect.DeepEqual(got, tc.want) || string(input) != tc.input {
			t.Errorf("InvalidRuns(%q)=%v want %v", tc.input, got, tc.want)
		}
	}
	calls := 0
	a.InvalidRuns([]byte("XAX"), func(start, end int) bool { calls++; return false })
	if calls != 1 {
		t.Fatalf("callback called %d times after requesting stop", calls)
	}
}

func TestNewAlphabetRejectsUnknown(t *testing.T) {
	for _, name := range []string{"", "auto", "DNA", "nucleotide"} {
		if _, err := NewAlphabet(name, false); err == nil {
			t.Errorf("NewAlphabet(%q) expected an error", name)
		}
	}
}
