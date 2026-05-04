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

var standardGeneticCode = map[string]byte{
	"TTT": 'F', "TTC": 'F',
	"TTA": 'L', "TTG": 'L', "CTT": 'L', "CTC": 'L', "CTA": 'L', "CTG": 'L',
	"ATT": 'I', "ATC": 'I', "ATA": 'I',
	"ATG": 'M',
	"GTT": 'V', "GTC": 'V', "GTA": 'V', "GTG": 'V',
	"TCT": 'S', "TCC": 'S', "TCA": 'S', "TCG": 'S', "AGT": 'S', "AGC": 'S',
	"CCT": 'P', "CCC": 'P', "CCA": 'P', "CCG": 'P',
	"ACT": 'T', "ACC": 'T', "ACA": 'T', "ACG": 'T',
	"GCT": 'A', "GCC": 'A', "GCA": 'A', "GCG": 'A',
	"TAT": 'Y', "TAC": 'Y',
	"TAA": '*', "TAG": '*', "TGA": '*',
	"CAT": 'H', "CAC": 'H',
	"CAA": 'Q', "CAG": 'Q',
	"AAT": 'N', "AAC": 'N',
	"AAA": 'K', "AAG": 'K',
	"GAT": 'D', "GAC": 'D',
	"GAA": 'E', "GAG": 'E',
	"TGT": 'C', "TGC": 'C',
	"TGG": 'W',
	"CGT": 'R', "CGC": 'R', "CGA": 'R', "CGG": 'R', "AGA": 'R', "AGG": 'R',
	"GGT": 'G', "GGC": 'G', "GGA": 'G', "GGG": 'G',
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

func TranslateCodon(codon []byte) byte {
	if len(codon) != 3 {
		return 'X'
	}
	normalized := string(NormalizeDNA(codon))
	aa, ok := standardGeneticCode[normalized]
	if !ok {
		return 'X'
	}
	return aa
}

func Translate(in []byte) []byte {
	out := make([]byte, 0, len(in)/3)
	for i := 0; i+3 <= len(in); i += 3 {
		out = append(out, TranslateCodon(in[i:i+3]))
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
