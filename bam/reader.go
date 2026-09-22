package bam

import (
	"io"

	htsbam "github.com/biogo/hts/bam"
	"github.com/martinghunt/faqt/internal/htsseq"
	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r *htsbam.Reader
}

// NewReader reads sequences from BAM on the calling goroutine.
func NewReader(r io.Reader) (*Reader, error) {
	return NewReaderWithThreads(r, 1)
}

// NewReaderWithThreads reads sequences from BAM, using threads worker
// goroutines to inflate BGZF blocks. One keeps the work on the calling
// goroutine; zero or less is clamped to one, because biogo reads zero as
// "use GOMAXPROCS" and faqt does not take cores it was not given.
func NewReaderWithThreads(r io.Reader, threads int) (*Reader, error) {
	if threads < 1 {
		threads = 1
	}
	br, err := htsbam.NewReader(r, threads)
	if err != nil {
		return nil, err
	}
	return &Reader{r: br}, nil
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	rec, err := r.r.Read()
	if err != nil {
		return nil, err
	}
	return htsseq.FromSAMRecord(rec), nil
}

func (r *Reader) Close() error {
	return r.r.Close()
}
