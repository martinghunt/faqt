package seqrecord

import (
	"bytes"
	"fmt"
	"io"
)

// SeqRecord is the minimal normalized sequence representation used throughout
// the library.
type SeqRecord struct {
	Name        string
	Description string
	Seq         []byte
	Qual        []byte
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
