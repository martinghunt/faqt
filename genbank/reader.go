package genbank

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/martinghunt/faqt/internal/flatseq"
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
		inDef    bool
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
			inDef = false
			seen = true
		case strings.HasPrefix(trimmed, "DEFINITION"):
			desc = flatseq.AppendDescription(desc, strings.TrimPrefix(trimmed, "DEFINITION"))
			inDef = true
		case inDef && isContinuationLine(trimmed):
			desc = flatseq.AppendDescription(desc, trimmed)
		case strings.HasPrefix(trimmed, "ORIGIN"):
			inOrigin = true
			inDef = false
		case trimmed == "//":
			if !seen {
				if err == io.EOF {
					return nil, io.EOF
				}
				continue
			}
			return &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq}, nil
		default:
			inDef = false
			if inOrigin {
				seq = append(seq, flatseq.SequenceLetters(trimmed)...)
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

func isContinuationLine(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
