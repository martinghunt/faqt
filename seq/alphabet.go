package seq

import "fmt"

// Alphabet is a case-insensitive table of accepted sequence bytes.
type Alphabet struct {
	allowed [256]bool
}

// NewAlphabet selects dna (ACGTRYSWKMBDHVN), rna (ACGURYSWKMBDHVN),
// or protein (A-Z, including ambiguity codes and U/O, plus the stop marker *).
// allowGaps additionally accepts '-' and '.' as alignment gap symbols.
func NewAlphabet(name string, allowGaps bool) (*Alphabet, error) {
	var letters string
	switch name {
	case "dna":
		letters = "ACGTRYSWKMBDHVN"
	case "rna":
		letters = "ACGURYSWKMBDHVN"
	case "protein":
		letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ*"
	default:
		return nil, fmt.Errorf("unknown alphabet %q: choose dna, rna, or protein", name)
	}
	a := &Alphabet{}
	for i := 0; i < len(letters); i++ {
		b := letters[i]
		a.allowed[b] = true
		if b >= 'A' && b <= 'Z' {
			a.allowed[b+'a'-'A'] = true
		}
	}
	if allowGaps {
		a.allowed['-'] = true
		a.allowed['.'] = true
	}
	return a, nil
}

// InvalidRuns calls yield for every contiguous run of unaccepted bytes in s.
// Coordinates are zero-based, half-open byte offsets. Returning false stops
// iteration. The input is never modified.
func (a *Alphabet) InvalidRuns(s []byte, yield func(start, end int) bool) {
	for i := 0; i < len(s); {
		if a.allowed[s[i]] {
			i++
			continue
		}
		start := i
		for i < len(s) && !a.allowed[s[i]] {
			i++
		}
		if !yield(start, i) {
			return
		}
	}
}
