package seq

import (
	"fmt"
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

var mycoplasmaGeneticCode = map[string]byte{
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
	"TAA": '*', "TAG": '*', "TGA": 'W',
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

var geneticCodes = map[int]map[string]byte{
	1:  standardGeneticCode,
	4:  mycoplasmaGeneticCode,
	11: standardGeneticCode,
}

var startCodons = map[int]map[string]struct{}{
	1:  {"TTG": {}, "CTG": {}, "ATG": {}},
	4:  {"TTA": {}, "TTG": {}, "CTG": {}, "ATT": {}, "ATC": {}, "ATA": {}, "ATG": {}, "GTG": {}},
	11: {"TTG": {}, "CTG": {}, "ATT": {}, "ATC": {}, "ATA": {}, "ATG": {}, "GTG": {}},
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
	return TranslateCodonWithCode(codon, 1)
}

func TranslateCodonWithCode(codon []byte, geneticCode int) byte {
	if len(codon) != 3 {
		return 'X'
	}
	code, ok := geneticCodes[geneticCode]
	if !ok {
		return 'X'
	}
	normalized := string(NormalizeDNA(codon))
	aa, ok := code[normalized]
	if !ok {
		return 'X'
	}
	return aa
}

func Translate(in []byte) []byte {
	return TranslateWithCode(in, 1)
}

func TranslateWithCode(in []byte, geneticCode int) []byte {
	out := make([]byte, 0, len(in)/3)
	for i := 0; i+3 <= len(in); i += 3 {
		out = append(out, TranslateCodonWithCode(in[i:i+3], geneticCode))
	}
	return out
}

func IsStartCodon(codon []byte, geneticCode int) bool {
	if len(codon) != 3 {
		return false
	}
	starts, ok := startCodons[geneticCode]
	if !ok {
		return false
	}
	_, ok = starts[string(NormalizeDNA(codon))]
	return ok
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
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		if ch == 'U' {
			out[i] = 'T'
			continue
		}
		out[i] = ch
	}
	return out
}
