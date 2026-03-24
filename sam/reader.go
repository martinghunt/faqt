package sam

import (
	"io"

	htssam "github.com/biogo/hts/sam"
	"github.com/martinghunt/faqt/internal/htsseq"
	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r *htssam.Reader
}

func NewReader(r io.Reader) (*Reader, error) {
	sr, err := htssam.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &Reader{r: sr}, nil
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	rec, err := r.r.Read()
	if err != nil {
		return nil, err
	}
	return htsseq.FromSAMRecord(rec), nil
}
