package htsseq

import (
	htssam "github.com/biogo/hts/sam"
	"github.com/martinghunt/faqt/internal/seqrecord"
	"github.com/martinghunt/faqt/seq"
)

func FromSAMRecord(rec *htssam.Record) *seqrecord.SeqRecord {
	if rec == nil {
		return nil
	}
	out := &seqrecord.SeqRecord{
		Name: rec.Name,
		Seq:  append([]byte(nil), rec.Seq.Expand()...),
		Qual: convertQual(rec.Qual),
	}
	if rec.Flags&htssam.Reverse != 0 {
		out.Seq = seq.ReverseComplement(out.Seq)
		reverseBytes(out.Qual)
	}
	return out
}

func convertQual(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, 0, len(in))
	allMissing := true
	for _, q := range in {
		if q == 0xff {
			out = append(out, '!')
			continue
		}
		allMissing = false
		out = append(out, q+33)
	}
	if allMissing {
		return nil
	}
	return out
}

func reverseBytes(in []byte) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}
