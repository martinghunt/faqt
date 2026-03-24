package fasta

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

func WriteRecord(w io.Writer, rec seqrecord.SeqRecord, wrap int) error {
	_, err := rec.WriteFASTATo(w, wrap)
	return err
}

func MustFormat(rec seqrecord.SeqRecord, wrap int) string {
	defer func() {
		if recovered := recover(); recovered != nil {
			panic(fmt.Sprintf("format fasta: %v", recovered))
		}
	}()
	return rec.FASTAString(wrap)
}
