package clustal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	records []*seqrecord.SeqRecord
	index   int
}

func NewReader(r *bufio.Reader) (*Reader, error) {
	header, err := readNextNonEmptyLine(r)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(bytes.ToUpper(header), []byte("CLUSTAL")) {
		return nil, fmt.Errorf("clustal input must start with CLUSTAL header")
	}

	order := make([]string, 0, 16)
	records := make(map[string]*seqrecord.SeqRecord)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		line = bytes.TrimRight(line, "\r\n")
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			if err == io.EOF {
				break
			}
			continue
		}
		if isConsensusLine(line) {
			if err == io.EOF {
				break
			}
			continue
		}
		fields := strings.Fields(string(line))
		if len(fields) < 2 {
			return nil, fmt.Errorf("invalid clustal sequence line %q", string(line))
		}
		name := fields[0]
		chunk := []byte(fields[1])
		rec, ok := records[name]
		if !ok {
			rec = &seqrecord.SeqRecord{Name: name}
			records[name] = rec
			order = append(order, name)
		}
		rec.Seq = append(rec.Seq, chunk...)
		if err == io.EOF {
			break
		}
	}

	out := make([]*seqrecord.SeqRecord, 0, len(order))
	for _, name := range order {
		out = append(out, records[name])
	}
	return &Reader{records: out}, nil
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	if r.index >= len(r.records) {
		return nil, io.EOF
	}
	rec := r.records[r.index]
	r.index++
	return rec, nil
}

func readNextNonEmptyLine(r *bufio.Reader) ([]byte, error) {
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

func isConsensusLine(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	if line[0] != ' ' && line[0] != '\t' {
		return false
	}
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return false
	}
	for _, b := range trimmed {
		switch b {
		case '*', ':', '.', ' ':
		default:
			return false
		}
	}
	return true
}
