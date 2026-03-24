package embl

import (
	"bufio"
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
		name string
		desc string
		seq  []byte
		inSQ bool
		seen bool
	)
	for {
		line, err := r.r.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		trimmed := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(trimmed, "ID"):
			if !seen {
				fields := strings.Fields(trimmed)
				if len(fields) > 1 {
					name = strings.TrimSuffix(fields[1], ";")
				}
				seen = true
			}
		case strings.HasPrefix(trimmed, "DE"):
			if desc == "" {
				desc = strings.TrimSpace(strings.TrimPrefix(trimmed, "DE"))
			}
		case strings.HasPrefix(trimmed, "SQ"):
			inSQ = true
		case trimmed == "//":
			if !seen {
				if err == io.EOF {
					return nil, io.EOF
				}
				continue
			}
			return &seqrecord.SeqRecord{Name: name, Description: desc, Seq: seq}, nil
		default:
			if inSQ {
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
