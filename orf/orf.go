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

var stopCodons = map[string]struct{}{
	"TAA": {},
	"TAG": {},
	"TGA": {},
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
