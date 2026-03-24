package fasta

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r       *bufio.Reader
	pending []byte
	eof     bool
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	if r.eof && len(r.pending) == 0 {
		return nil, io.EOF
	}

	header := r.pending
	r.pending = nil
	for len(header) == 0 {
		line, err := r.r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) == 0 && err == io.EOF {
			r.eof = true
			return nil, io.EOF
		}
		if len(line) == 0 {
			continue
		}
		header = line
		if err == io.EOF {
			r.eof = true
		}
	}
	if header[0] != '>' {
		return nil, io.ErrUnexpectedEOF
	}
	name, desc := parseHeader(header[1:])
	seq := make([]byte, 0, 1024)
	for {
		line, err := r.r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimRight(line, "\r\n")
		if len(line) > 0 && line[0] == '>' {
			r.pending = line
			break
		}
		seq = append(seq, bytes.TrimSpace(line)...)
		if err == io.EOF {
			r.eof = true
			break
		}
	}
	return &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq}, nil
}

func parseHeader(header []byte) (string, string) {
	parts := strings.Fields(string(header))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}
