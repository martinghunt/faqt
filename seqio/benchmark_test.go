package seqio_test

import (
	"io"
	"os"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func BenchmarkReadFASTQFile(b *testing.B) {
	path := os.Getenv("FASTQ_BENCH_FILE")
	if path == "" {
		b.Skip("set FASTQ_BENCH_FILE to benchmark a real FASTQ input")
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader, err := seqio.OpenPath(path)
		if err != nil {
			b.Fatalf("OpenPath() error = %v", err)
		}

		var records int
		var bases int
		for {
			rec, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				b.Fatalf("Read() error = %v", err)
			}
			records++
			bases += len(rec.Seq)
		}
		if closer, ok := reader.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				b.Fatalf("Close() error = %v", err)
			}
		}
		b.ReportMetric(float64(records), "records/op")
		b.ReportMetric(float64(bases), "bases/op")
	}
}
