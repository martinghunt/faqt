package fastq

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r *bufio.Reader
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	header, err := r.readLine()
	if err != nil {
		return nil, err
	}
	if len(header) == 0 || header[0] != '@' {
		return nil, fmt.Errorf("fastq record must start with @")
	}
	seq, err := r.readLine()
	if err != nil {
		return nil, err
	}
	plus, err := r.readLine()
	if err != nil {
		return nil, err
	}
	if len(plus) == 0 || plus[0] != '+' {
		return nil, fmt.Errorf("fastq separator line must start with +")
	}
	qual, err := r.readLine()
	if err != nil {
		return nil, err
	}
	name, desc := seqrecord.ParseHeader(header[1:])
	rec := &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq, Qual: qual}
	if err := rec.ValidateFASTQ(); err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Reader) readLine() ([]byte, error) {
	line, err := r.r.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = bytes.TrimRight(line, "\r\n")
	if err == io.EOF && len(line) == 0 {
		return nil, io.EOF
	}
	return line, nil
}
