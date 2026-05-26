package orf

import (
	"strings"

	"github.com/martinghunt/faqt/seq"
)

type ORFOptions struct {
	MinLength   int
	StartCodons []string
	BothStrands bool
}

type ORF struct {
	Start  int
	End    int
	Frame  int
	Strand string
	Seq    []byte
}

type GeneOptions struct {
	GeneticCode int
}

type Gene struct {
	Seq    []byte
	Strand string
	Frame  int
}

var stopCodons = map[string]struct{}{
	"TAA": {},
	"TAG": {},
	"TGA": {},
}

func MakeIntoGene(in []byte, opts GeneOptions) (Gene, bool) {
	geneticCode := opts.GeneticCode
	if geneticCode == 0 {
		geneticCode = 1
	}
	for _, reverse := range []bool{true, false} {
		working := seq.NormalizeDNA(in)
		if reverse {
			working = seq.ReverseComplement(working)
		}
		for frame := 0; frame < 3; frame++ {
			if frame >= len(working) {
				continue
			}
			geneSeq := append([]byte(nil), working[frame:]...)
			if extra := len(geneSeq) % 3; extra != 0 {
				geneSeq = geneSeq[:len(geneSeq)-extra]
			}
			if isCompleteGene(geneSeq, geneticCode) {
				strand := "+"
				if reverse {
					strand = "-"
				}
				return Gene{Seq: geneSeq, Strand: strand, Frame: frame}, true
			}
		}
	}
	return Gene{}, false
}

func isCompleteGene(in []byte, geneticCode int) bool {
	if len(in) < 6 || len(in)%3 != 0 {
		return false
	}
	if !seq.IsStartCodon(in[:3], geneticCode) {
		return false
	}
	translated := seq.TranslateWithCode(in, geneticCode)
	if len(translated) < 2 || translated[len(translated)-1] != '*' {
		return false
	}
	for _, aa := range translated[:len(translated)-1] {
		if aa == '*' {
			return false
		}
	}
	return true
}

func FindORFs(in []byte, opts ORFOptions) []ORF {
	starts := map[string]struct{}{"ATG": {}}
	for _, codon := range opts.StartCodons {
		starts[strings.ToUpper(strings.ReplaceAll(codon, "U", "T"))] = struct{}{}
	}
	forward := findStrandORFs(seq.NormalizeDNA(in), starts, opts, "+")
	if !opts.BothStrands {
		return forward
	}
	rc := seq.NormalizeDNA(seq.ReverseComplement(in))
	reverse := findStrandORFs(rc, starts, opts, "-")
	for i := range reverse {
		reverse[i].Start, reverse[i].End = len(in)-reverse[i].End, len(in)-reverse[i].Start
	}
	return append(forward, reverse...)
}

func findStrandORFs(in []byte, starts map[string]struct{}, opts ORFOptions, strand string) []ORF {
	var out []ORF
	for frame := 0; frame < 3; frame++ {
		for i := frame; i+3 <= len(in); i += 3 {
			codon := string(in[i : i+3])
			if _, ok := starts[codon]; !ok {
				continue
			}
			for j := i + 3; j+3 <= len(in); j += 3 {
				if _, stop := stopCodons[string(in[j:j+3])]; !stop {
					continue
				}
				if j+3-i < opts.MinLength {
					break
				}
				out = append(out, ORF{
					Start:  i,
					End:    j + 3,
					Frame:  frame,
					Strand: strand,
					Seq:    append([]byte(nil), in[i:j+3]...),
				})
				break
			}
		}
	}
	return out
}
