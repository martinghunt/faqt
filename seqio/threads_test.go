package seqio_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

// TestWithThreadsClampsToOne checks that zero and negative thread counts never
// reach the libraries, where zero means GOMAXPROCS.
func TestWithThreadsClampsToOne(t *testing.T) {
	plain := ">rec1\nACGT\n"
	for _, threads := range []int{-1, 0} {
		path := filepath.Join(t.TempDir(), "input.fa")
		if err := os.WriteFile(path, []byte(plain), 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}
		reader, err := seqio.OpenPath(path, seqio.WithThreads(threads))
		if err != nil {
			t.Fatalf("OpenPath(threads=%d) error = %v", threads, err)
		}
		rec, err := reader.Read()
		if err != nil {
			t.Fatalf("Read(threads=%d) error = %v", threads, err)
		}
		if rec.Name != "rec1" {
			t.Fatalf("record = %+v", rec)
		}
		if closer, ok := reader.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		}
	}
}
