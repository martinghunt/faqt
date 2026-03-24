package fastq

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

func WriteRecord(w io.Writer, rec seqrecord.SeqRecord) error {
	if err := rec.ValidateFASTQ(); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "@"+rec.Header()+"\n"); err != nil {
		return err
	}
	if _, err := w.Write(rec.Seq); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n+\n"); err != nil {
		return err
	}
	if _, err := w.Write(rec.Qual); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\n")
	return err
}

func MustFormat(rec seqrecord.SeqRecord) string {
	var out []byte
	buf := bytesBuffer{b: out}
	if err := WriteRecord(&buf, rec); err != nil {
		panic(fmt.Sprintf("format fastq: %v", err))
	}
	return string(buf.b)
}

type bytesBuffer struct {
	b []byte
}

func (b *bytesBuffer) Write(p []byte) (int, error) {
	b.b = append(b.b, p...)
	return len(p), nil
}
