package genbank

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r *bufio.Reader
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	var (
		name     string
		desc     string
		seq      []byte
		inOrigin bool
		seen     bool
	)
	for {
		line, err := r.r.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "LOCUS"):
			if seen {
				return nil, fmt.Errorf("unexpected LOCUS before end of previous record")
			}
			fields := strings.Fields(trimmed)
			if len(fields) > 1 {
				name = fields[1]
			}
			seen = true
		case strings.HasPrefix(trimmed, "DEFINITION"):
			desc = strings.TrimSpace(strings.TrimPrefix(trimmed, "DEFINITION"))
		case strings.HasPrefix(trimmed, "ORIGIN"):
			inOrigin = true
		case trimmed == "//":
			if !seen {
				if err == io.EOF {
					return nil, io.EOF
				}
				continue
			}
			return &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq}, nil
		default:
			if inOrigin {
				seq = append(seq, sequenceFromLine(trimmed)...)
			}
		}
		if err == io.EOF {
			if !seen {
				return nil, io.EOF
			}
			return &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq}, nil
		}
	}
}

func sequenceFromLine(line string) []byte {
	line = strings.ToLower(line)
	var out []byte
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch >= 'a' && ch <= 'z' {
			out = append(out, ch)
		}
	}
	return out
}
