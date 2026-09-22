package seqio_test

import (
	"io"
	"os"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func BenchmarkReadFASTQFile(b *testing.B) {
	benchmarkReadFile(b, "FASTQ_BENCH_FILE")
}

// BenchmarkReadFASTAFile benchmarks a real FASTA input. Wrapped and unwrapped
// files exercise different paths in the reader, so benchmark both.
func BenchmarkReadFASTAFile(b *testing.B) {
	benchmarkReadFile(b, "FASTA_BENCH_FILE")
}

func benchmarkReadFile(b *testing.B, env string) {
	b.Helper()
	path := os.Getenv(env)
	if path == "" {
		b.Skipf("set %s to benchmark a real input", env)
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
