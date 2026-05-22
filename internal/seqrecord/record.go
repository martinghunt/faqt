package seqrecord

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// SeqRecord is the minimal normalized sequence representation used throughout
// the library.
type SeqRecord struct {
	Name        string
	Description string
	Seq         []byte
	Qual        []byte
}

func ParseHeader(header []byte) (string, string) {
	for _, b := range header {
		if b >= 0x80 {
			return parseUnicodeHeader(header)
		}
	}
	return parseASCIIHeader(header)
}

func parseUnicodeHeader(header []byte) (string, string) {
	parts := strings.Fields(string(header))
	if len(parts) == 0 {
		return "", ""
	}
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.Join(parts[1:], " ")
}

func parseASCIIHeader(header []byte) (string, string) {
	i := 0
	for i < len(header) && isHeaderSpace(header[i]) {
		i++
	}
	if i == len(header) {
		return "", ""
	}

	nameStart := i
	for i < len(header) && !isHeaderSpace(header[i]) {
		i++
	}
	name := string(header[nameStart:i])

	for i < len(header) && isHeaderSpace(header[i]) {
		i++
	}
	if i == len(header) {
		return name, ""
	}

	descStart := i
	descEnd := len(header)
	for descEnd > descStart && isHeaderSpace(header[descEnd-1]) {
		descEnd--
	}
	return name, normalizeHeaderDescription(header[descStart:descEnd])
}

func normalizeHeaderDescription(in []byte) string {
	needsNormalize := false
	prevSpace := false
	for _, b := range in {
		if isHeaderSpace(b) {
			if b != ' ' || prevSpace {
				needsNormalize = true
				break
			}
			prevSpace = true
			continue
		}
		prevSpace = false
	}
	if !needsNormalize {
		return string(in)
	}

	out := make([]byte, 0, len(in))
	prevSpace = false
	for _, b := range in {
		if isHeaderSpace(b) {
			if !prevSpace {
				out = append(out, ' ')
				prevSpace = true
			}
			continue
		}
		out = append(out, b)
		prevSpace = false
	}
	return string(out)
}

func isHeaderSpace(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func (r SeqRecord) String() string {
	var buf bytes.Buffer
	_, _ = r.WriteTo(&buf)
	return buf.String()
}

func (r SeqRecord) WriteTo(w io.Writer) (int64, error) {
	if r.Qual != nil {
		return r.writeFASTQTo(w)
	}
	return r.WriteFASTATo(w, 0)
}

func (r SeqRecord) Header() string {
	if r.Description == "" {
		return r.Name
	}
	if r.Name == "" {
		return r.Description
	}
	return r.Name + " " + r.Description
}

func (r SeqRecord) ValidateFASTQ() error {
	if r.Qual == nil {
		return nil
	}
	if len(r.Seq) != len(r.Qual) {
		return fmt.Errorf("fastq record %q has sequence length %d and quality length %d", r.Name, len(r.Seq), len(r.Qual))
	}
	return nil
}

func (r SeqRecord) FASTAString(wrap int) string {
	var buf bytes.Buffer
	_, _ = r.WriteFASTATo(&buf, wrap)
	return buf.String()
}

func (r SeqRecord) WriteFASTATo(w io.Writer, wrap int) (int64, error) {
	total := int64(0)
	n, err := io.WriteString(w, ">"+r.Header()+"\n")
	total += int64(n)
	if err != nil {
		return total, err
	}

	if wrap <= 0 {
		n, err = w.Write(r.Seq)
		total += int64(n)
		if err != nil {
			return total, err
		}
		n, err = io.WriteString(w, "\n")
		total += int64(n)
		return total, err
	}

	for start := 0; start < len(r.Seq); start += wrap {
		end := start + wrap
		if end > len(r.Seq) {
			end = len(r.Seq)
		}
		n, err = w.Write(r.Seq[start:end])
		total += int64(n)
		if err != nil {
			return total, err
		}
		n, err = io.WriteString(w, "\n")
		total += int64(n)
		if err != nil {
			return total, err
		}
	}
	if len(r.Seq) == 0 {
		n, err = io.WriteString(w, "\n")
		total += int64(n)
	}
	return total, err
}

func (r SeqRecord) writeFASTQTo(w io.Writer) (int64, error) {
	if err := r.ValidateFASTQ(); err != nil {
		return 0, err
	}
	total := int64(0)
	n, err := io.WriteString(w, "@"+r.Header()+"\n")
	total += int64(n)
	if err != nil {
		return total, err
	}
	n, err = w.Write(r.Seq)
	total += int64(n)
	if err != nil {
		return total, err
	}
	n, err = io.WriteString(w, "\n+\n")
	total += int64(n)
	if err != nil {
		return total, err
	}
	n, err = w.Write(r.Qual)
	total += int64(n)
	if err != nil {
		return total, err
	}
	n, err = io.WriteString(w, "\n")
	total += int64(n)
	return total, err
}
