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
	for len(seq) < r.seqLen {
		line, err := readNonEmptyLine(r.r)
		if err == io.EOF {
			return nil, fmt.Errorf("unexpected end of phylip sequence %q at length %d, want %d", name, len(seq), r.seqLen)
		}
		if err != nil {
			return nil, err
		}
		seq, err = appendSequenceContinuation(seq, line)
		if err != nil {
			return nil, fmt.Errorf("phylip sequence %q: %w (interleaved PHYLIP is not supported)", name, err)
		}
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

// appendSequenceContinuation appends a wrapped-line continuation of a
// sequential PHYLIP record's sequence. Unlike appendSequenceChars, it
// rejects characters outside the sequence alphabet (letters and the common
// gap/stop/unknown symbols) instead of accepting anything. A continuation
// line is only ever real sequence data in the sequential format this reader
// supports; in an interleaved PHYLIP file, a "continuation" line is actually
// the next taxon's own name-and-sequence line, and accepting it here would
// silently splice that taxon's name and sequence into the current record.
func appendSequenceContinuation(dst, src []byte) ([]byte, error) {
	for _, b := range src {
		switch {
		case b == ' ' || b == '\t' || b == '\r' || b == '\n':
			continue
		case isSequenceLetter(b):
			dst = append(dst, b)
		default:
			return nil, fmt.Errorf("unexpected character %q on continuation line %q", b, src)
		}
	}
	return dst, nil
}

func isSequenceLetter(b byte) bool {
	switch {
	case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z':
		return true
	case b == '-' || b == '.' || b == '*' || b == '?':
		return true
	default:
		return false
	}
}
