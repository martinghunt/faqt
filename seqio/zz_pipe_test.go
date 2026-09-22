package seqio_test

import (
	"io"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func BenchmarkFastaToGz(b *testing.B) {
	const in = "/private/tmp/claude-503/-Users-mh46-git-faqt/498f59c7-aa8d-4159-8d55-7e68318a9cc4/scratchpad/probe.fa"
	for i := 0; i < b.N; i++ {
		r, err := seqio.OpenPath(in)
		if err != nil {
			b.Fatal(err)
		}
		w, err := seqio.CreatePath(b.TempDir()+"/out.fa.gz", seqio.FASTA)
		if err != nil {
			b.Fatal(err)
		}
		for {
			rec, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatal(err)
			}
			if err := w.Write(rec); err != nil {
				b.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			b.Fatal(err)
		}
		if c, ok := r.(io.Closer); ok {
			c.Close()
		}
	}
}
