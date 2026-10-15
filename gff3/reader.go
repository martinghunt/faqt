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
	r            *bufio.Reader
	fastaPart    *fasta.Reader
	seenFASTA    bool
	seenSequence bool
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	if err := r.startFASTA(); err != nil {
		return nil, err
	}
	return r.readFASTARecord()
}

// Check checks the embedded FASTA section, continuing past NUL errors.
// Diagnostic line numbers are relative to that section.
func (r *Reader) Check(report func(error)) (int, error) {
	if err := r.startFASTA(); err != nil {
		return 0, err
	}
	count, err := r.fastaPart.Check(report)
	if count == 0 && err == nil && !r.seenSequence {
		return 0, fmt.Errorf("gff3 input does not contain sequence records in ##FASTA section")
	}
	if count > 0 {
		r.seenSequence = true
	}
	return count, err
}

func (r *Reader) startFASTA() error {
	if r.fastaPart != nil {
		return nil
	}
	for {
		line, err := r.r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = bytes.TrimRight(line, "\r\n")
		if bytes.Equal(line, []byte("##FASTA")) {
			r.seenFASTA = true
			r.fastaPart = fasta.NewReader(r.r)
			return nil
		}
		if err == io.EOF {
			if !r.seenFASTA {
				return fmt.Errorf("gff3 input does not contain ##FASTA sequence section")
			}
			return io.EOF
		}
	}
}

func (r *Reader) readFASTARecord() (*seqrecord.SeqRecord, error) {
	rec, err := r.fastaPart.Read()
	if err == io.EOF && !r.seenSequence {
		return nil, fmt.Errorf("gff3 input does not contain sequence records in ##FASTA section")
	}
	if err != nil {
		return nil, err
	}
	r.seenSequence = true
	return rec, nil
}
