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
	header, err := r.readBufferedLine()
	if err != nil {
		return nil, err
	}
	if len(header) == 0 || header[0] != '@' {
		return nil, fmt.Errorf("fastq record must start with @")
	}
	name, desc := seqrecord.ParseHeader(header[1:])

	seq, err := r.readOwnedLine()
	if err != nil {
		return nil, err
	}
	plus, err := r.readBufferedLine()
	if err != nil {
		return nil, err
	}
	if len(plus) == 0 || plus[0] != '+' {
		return nil, fmt.Errorf("fastq separator line must start with +")
	}
	qual, err := r.readOwnedLine()
	if err != nil {
		return nil, err
	}
	rec := &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq, Qual: qual}
	if err := rec.ValidateFASTQ(); err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *Reader) readBufferedLine() ([]byte, error) {
	line, err := r.r.ReadSlice('\n')
	if err == bufio.ErrBufferFull {
		return r.readLongBufferedLine(line)
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = bytes.TrimRight(line, "\r\n")
	if err == io.EOF && len(line) == 0 {
		return nil, io.EOF
	}
	return line, nil
}

func (r *Reader) readLongBufferedLine(first []byte) ([]byte, error) {
	line := append([]byte(nil), first...)
	for {
		next, err := r.r.ReadSlice('\n')
		line = append(line, next...)
		if err == bufio.ErrBufferFull {
			continue
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if err == io.EOF && len(line) == 0 {
			return nil, io.EOF
		}
		return line, nil
	}
}

func (r *Reader) readOwnedLine() ([]byte, error) {
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
