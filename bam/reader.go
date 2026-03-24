package bam

import (
	"io"

	htsbam "github.com/biogo/hts/bam"
	"github.com/martinghunt/faqt/internal/htsseq"
	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r *htsbam.Reader
}

func NewReader(r io.Reader) (*Reader, error) {
	br, err := htsbam.NewReader(r, 0)
	if err != nil {
		return nil, err
	}
	return &Reader{r: br}, nil
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	rec, err := r.r.Read()
	if err != nil {
		return nil, err
	}
	return htsseq.FromSAMRecord(rec), nil
}

func (r *Reader) Close() error {
	return r.r.Close()
}
