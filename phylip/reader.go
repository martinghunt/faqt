package phylip

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r           *bufio.Reader
	recordCount int
	seqLen      int
	readCount   int
}

func NewReader(r *bufio.Reader) (*Reader, error) {
	line, err := readNonEmptyLine(r)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(line))
	if len(fields) < 2 {
		return nil, fmt.Errorf("phylip header must contain sequence count and alignment length")
	}
	recordCount, err := strconv.Atoi(fields[0])
	if err != nil || recordCount < 0 {
		return nil, fmt.Errorf("invalid phylip sequence count %q", fields[0])
	}
	seqLen, err := strconv.Atoi(fields[1])
	if err != nil || seqLen < 0 {
		return nil, fmt.Errorf("invalid phylip sequence length %q", fields[1])
	}
	return &Reader{r: r, recordCount: recordCount, seqLen: seqLen}, nil
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	if r.readCount >= r.recordCount {
		return nil, io.EOF
	}
	line, err := readNonEmptyLine(r.r)
	if err != nil {
		return nil, err
	}
	fields := strings.Fields(string(line))
	if len(fields) < 2 {
		return nil, fmt.Errorf("phylip record line must contain name and sequence")
	}
	name := fields[0]
	seq := make([]byte, 0, r.seqLen)
	for _, part := range fields[1:] {
		seq = appendSequenceChars(seq, []byte(part))
	}
	if len(seq) != r.seqLen {
		return nil, fmt.Errorf("phylip sequence %q length %d does not match header length %d", name, len(seq), r.seqLen)
	}
	r.readCount++
	return &seqrecord.SeqRecord{Name: name, Seq: seq}, nil
}

func readNonEmptyLine(r *bufio.Reader) ([]byte, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimSpace(line)
		if len(line) != 0 {
			return line, nil
		}
		if err == io.EOF {
			return nil, io.EOF
		}
	}
}

func appendSequenceChars(dst, src []byte) []byte {
	for _, b := range src {
		if b == ' ' || b == '\t' || b == '\r' || b == '\n' {
			continue
		}
		dst = append(dst, b)
	}
	return dst
}
