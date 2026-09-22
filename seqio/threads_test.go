package seqio_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/martinghunt/faqt/seqio"
)

// cpuSeconds returns the CPU time this process has used so far.
func cpuSeconds(t *testing.T) float64 {
	t.Helper()
	var usage syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &usage); err != nil {
		t.Skipf("Getrusage() unavailable: %v", err)
	}
	seconds := func(tv syscall.Timeval) float64 {
		return float64(tv.Sec) + float64(tv.Usec)/1e6
	}
	return seconds(usage.Utime) + seconds(usage.Stime)
}

// readWholeFile reads every record and reports the CPU time used per second of
// wall time. One decoding goroutine gives roughly 1.0; a worker pool gives the
// number of cores it managed to use.
//
// Getrusage covers the whole process, so the garbage collector is paused for
// the measurement. Otherwise a collection triggered by another test is charged
// to this one and the reading looks parallel when it is not.
func readWholeFile(t *testing.T, path string, opts ...seqio.Option) float64 {
	t.Helper()
	runtime.GC()
	previousGC := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previousGC)

	cpuBefore := cpuSeconds(t)
	wallBefore := time.Now()

	reader, err := seqio.OpenPath(path, opts...)
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	for {
		_, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
	}
	if closer, ok := reader.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	wall := time.Since(wallBefore).Seconds()
	cpu := cpuSeconds(t) - cpuBefore
	if wall <= 0 {
		return 1
	}
	return cpu / wall
}

var (
	zstdFixtureOnce sync.Once
	zstdFixturePath string
	zstdFixtureDir  string
)

// TestMain removes the shared thread-measurement fixture, which outlives the
// test that builds it.
func TestMain(m *testing.M) {
	code := m.Run()
	if zstdFixtureDir != "" {
		_ = os.RemoveAll(zstdFixtureDir)
	}
	os.Exit(code)
}

// zstdFixture writes a zstd-compressed FASTA file big enough that
// decompression, not startup, dominates the measurement. It is built once and
// shared, so building it does not generate garbage inside a measurement.
func zstdFixture(t *testing.T) string {
	t.Helper()
	zstdFixtureOnce.Do(func() {
		zstdFixturePath = buildZstdFixture(t)
	})
	if zstdFixturePath == "" {
		t.Fatal("zstd fixture was not built")
	}
	return zstdFixturePath
}

func buildZstdFixture(t *testing.T) string {
	t.Helper()
	// Random-ish sequence so zstd has real work to do rather than
	// collapsing the whole file into a few matches.
	var seq bytes.Buffer
	bases := []byte("ACGT")
	x := uint32(12345)
	for i := 0; i < 24_000_000; i++ {
		x = x*1664525 + 1013904223
		seq.WriteByte(bases[(x>>16)&3])
	}
	sequence := seq.String()

	var plain bytes.Buffer
	for i := 0; i < 240; i++ {
		plain.WriteString(">rec\n")
		plain.WriteString(sequence[i*100000 : (i+1)*100000])
		plain.WriteString("\n")
	}

	var out bytes.Buffer
	zw, err := zstd.NewWriter(&out, zstd.WithEncoderConcurrency(1))
	if err != nil {
		t.Fatalf("zstd NewWriter() error = %v", err)
	}
	if _, err := zw.Write(plain.Bytes()); err != nil {
		t.Fatalf("zstd Write() error = %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zstd Close() error = %v", err)
	}

	dir, err := os.MkdirTemp("", "faqt-threads-*")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	// Removed by TestMain rather than t.Cleanup: the fixture outlives the
	// test that happens to build it.
	zstdFixtureDir = dir
	path := filepath.Join(dir, "input.fa.zst")
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

// TestDefaultStaysOnOneCore guards the CPU contract: unless a caller asks for
// more, faqt must use about one core. The decompression libraries default to
// GOMAXPROCS workers, and schedulers that allocate one core kill jobs that
// quietly fan out across a node.
func TestDefaultStaysOnOneCore(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 4 {
		t.Skip("needs at least 4 usable cores to tell one core from a pool")
	}
	path := zstdFixture(t)

	// Warm the page cache so the first read is not charged for file I/O.
	readWholeFile(t, path)

	used := readWholeFile(t, path)
	if used > 1.5 {
		t.Fatalf("default read used %.2f cores, want about 1: decompression is fanning out", used)
	}
}

// TestWithThreadsIsHonoured is the other half of the contract: asking for
// threads must actually spread the work, or the escape hatch is a lie.
func TestWithThreadsIsHonoured(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 4 {
		t.Skip("needs at least 4 usable cores to observe parallel decoding")
	}
	path := zstdFixture(t)
	readWholeFile(t, path, seqio.WithThreads(4))

	used := readWholeFile(t, path, seqio.WithThreads(4))
	if used < 1.2 {
		t.Fatalf("read with 4 threads used %.2f cores, want more than 1: threads are ignored", used)
	}
}

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
