package gff3

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/martinghunt/faqt/fasta"
	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r         *bufio.Reader
	fastaPart *fasta.Reader
	seenFASTA bool
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	if r.fastaPart != nil {
		return r.fastaPart.Read()
	}
	for {
		line, err := r.r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if bytes.Equal(line, []byte("##FASTA")) {
			r.seenFASTA = true
			r.fastaPart = fasta.NewReader(r.r)
			return r.fastaPart.Read()
		}
		if err == io.EOF {
			if !r.seenFASTA {
				return nil, fmt.Errorf("gff3 input does not contain ##FASTA sequence section")
			}
			return nil, io.EOF
		}
	}
}
