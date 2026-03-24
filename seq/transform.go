package seq

import (
	"fmt"
	"strings"
)

var complementTable = map[byte]byte{
	'A': 'T', 'a': 't',
	'C': 'G', 'c': 'g',
	'G': 'C', 'g': 'c',
	'T': 'A', 't': 'a',
	'U': 'A', 'u': 'a',
	'R': 'Y', 'r': 'y',
	'Y': 'R', 'y': 'r',
	'S': 'S', 's': 's',
	'W': 'W', 'w': 'w',
	'K': 'M', 'k': 'm',
	'M': 'K', 'm': 'k',
	'B': 'V', 'b': 'v',
	'D': 'H', 'd': 'h',
	'H': 'D', 'h': 'd',
	'V': 'B', 'v': 'b',
	'N': 'N', 'n': 'n',
	'-': '-',
}

func ReverseComplement(in []byte) []byte {
	out := make([]byte, len(in))
	for i := range in {
		base := in[len(in)-1-i]
		comp, ok := complementTable[base]
		if !ok {
			comp = 'N'
		}
		out[i] = comp
	}
	return out
}

func Subseq(in []byte, start, end int) ([]byte, error) {
	if start < 0 || end < 0 || start > end || end > len(in) {
		return nil, fmt.Errorf("invalid interval [%d,%d) for sequence length %d", start, end, len(in))
	}
	return append([]byte(nil), in[start:end]...), nil
}

func NormalizeDNA(in []byte) []byte {
	out := make([]byte, len(in))
	for i, ch := range in {
		switch ch {
		case 'u', 'U':
			out[i] = 'T'
		default:
			out[i] = byte(strings.ToUpper(string([]byte{ch}))[0])
		}
	}
	return out
}
