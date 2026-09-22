package seqio_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/biogo/hts/bgzf"
	dsnetbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/martinghunt/faqt/seqio"
	"github.com/ulikunitz/xz"
)

// This file holds the read and write matrix over every input format and every
// compression faqt accepts. Fixtures are compressed here rather than
// round-tripped through faqt's own writer, because faqt reads containers it
// cannot write: BGZF, the container bgzip and samtools produce, reached
// release 0.9.1 unreadable for every text format precisely because no
// round-trip test could produce it.

type wantRecord struct {
	name string
	desc string
	seq  string
	qual string
}

// formatFixture is one input format, as bytes plus the records faqt should
// find in them.
type formatFixture struct {
	format string
	plain  string
	want   []wantRecord
}

func textFormatFixtures() []formatFixture {
	return []formatFixture{
		{
			format: "fasta",
			plain:  ">rec1 first record\nACGT\n>rec2\nNNNN\n",
			want: []wantRecord{
				{name: "rec1", desc: "first record", seq: "ACGT"},
				{name: "rec2", seq: "NNNN"},
			},
		},
		{
			format: "fasta-wrapped",
			plain:  ">rec1 first record\nAC\nGT\n>rec2\nNN\nNN\n",
			want: []wantRecord{
				{name: "rec1", desc: "first record", seq: "ACGT"},
				{name: "rec2", seq: "NNNN"},
			},
		},
		{
			format: "fastq",
			plain:  "@rec1 first record\nACGT\n+\n!!!!\n@rec2\nNNNN\n+\n####\n",
			want: []wantRecord{
				{name: "rec1", desc: "first record", seq: "ACGT", qual: "!!!!"},
				{name: "rec2", seq: "NNNN", qual: "####"},
			},
		},
		{
			format: "sam",
			plain: "@HD\tVN:1.6\n" +
				"@SQ\tSN:ref\tLN:100\n" +
				"rec1\t0\tref\t1\t60\t4M\t*\t0\t0\tACGT\t!!!!\n" +
				"rec2\t16\tref\t1\t60\t4M\t*\t0\t0\tAAAC\t$###\n",
			want: []wantRecord{
				{name: "rec1", seq: "ACGT", qual: "!!!!"},
				// Reverse strand, so reported in original read orientation.
				{name: "rec2", seq: "GTTT", qual: "###$"},
			},
		},
		{
			format: "phylip",
			plain:  "2 4\nseq1 ACGT\nseq2 NNNN\n",
			want: []wantRecord{
				{name: "seq1", seq: "ACGT"},
				{name: "seq2", seq: "NNNN"},
			},
		},
		{
			format: "clustal",
			plain:  "CLUSTAL W (1.83) multiple sequence alignment\n\nseq1    ACGT\nseq2    NN-N\n        **\n",
			want: []wantRecord{
				{name: "seq1", seq: "ACGT"},
				{name: "seq2", seq: "NN-N"},
			},
		},
		{
			format: "genbank",
			plain: "LOCUS       REC1\nDEFINITION  first record\nORIGIN\n        1 acgt\n//\n" +
				"LOCUS       REC2\nDEFINITION  second record\nORIGIN\n        1 nnnn\n//\n",
			want: []wantRecord{
				{name: "REC1", desc: "first record", seq: "acgt"},
				{name: "REC2", desc: "second record", seq: "nnnn"},
			},
		},
		{
			format: "embl",
			plain: "ID   REC1;\nDE   first record\nSQ   Sequence 4 BP;\n     acgt\n//\n" +
				"ID   REC2;\nDE   second record\nSQ   Sequence 4 BP;\n     nnnn\n//\n",
			want: []wantRecord{
				{name: "REC1", desc: "first record", seq: "acgt"},
				{name: "REC2", desc: "second record", seq: "nnnn"},
			},
		},
		{
			format: "gff3",
			plain: "##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n##FASTA\n" +
				">chr1 first record\nACGT\n>chr2\nNNNN\n",
			want: []wantRecord{
				{name: "chr1", desc: "first record", seq: "ACGT"},
				{name: "chr2", seq: "NNNN"},
			},
		},
	}
}

// compressor is one accepted container. ext is the suffix a user would see,
// which faqt must ignore in favour of the magic bytes.
type compressor struct {
	name     string
	ext      string
	compress func(t *testing.T, plain []byte) []byte
}

func readCompressors() []compressor {
	return []compressor{
		{
			name: "none",
			ext:  "",
			compress: func(_ *testing.T, plain []byte) []byte {
				return plain
			},
		},
		{
			name:     "gzip",
			ext:      ".gz",
			compress: compressGzip,
		},
		{
			// The container bgzip and samtools write. It is gzip with a
			// BC extra field, and it also carries BAM, so reading it
			// means looking inside rather than trusting the container.
			name:     "bgzf",
			ext:      ".gz",
			compress: compressBGZF,
		},
		{
			name:     "bzip2",
			ext:      ".bz2",
			compress: compressBzip2,
		},
		{
			name:     "xz",
			ext:      ".xz",
			compress: compressXZ,
		},
		{
			name:     "zstd",
			ext:      ".zst",
			compress: compressZstd,
		},
	}
}

func compressGzip(t *testing.T, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return buf.Bytes()
}

func compressBGZF(t *testing.T, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := bgzf.NewWriter(&buf, 1)
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("bgzf Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bgzf Close() error = %v", err)
	}
	return buf.Bytes()
}

func compressBzip2(t *testing.T, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := dsnetbzip2.NewWriter(&buf, nil)
	if err != nil {
		t.Fatalf("bzip2 NewWriter() error = %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("bzip2 Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("bzip2 Close() error = %v", err)
	}
	return buf.Bytes()
}

func compressXZ(t *testing.T, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := xz.NewWriter(&buf)
	if err != nil {
		t.Fatalf("xz NewWriter() error = %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("xz Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("xz Close() error = %v", err)
	}
	return buf.Bytes()
}

func compressZstd(t *testing.T, plain []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd NewWriter() error = %v", err)
	}
	if _, err := w.Write(plain); err != nil {
		t.Fatalf("zstd Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zstd Close() error = %v", err)
	}
	return buf.Bytes()
}

func readAllRecords(t *testing.T, reader seqio.Reader) []wantRecord {
	t.Helper()
	var got []wantRecord
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return got
		}
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		got = append(got, wantRecord{
			name: rec.Name,
			desc: rec.Description,
			seq:  string(rec.Seq),
			qual: string(rec.Qual),
		})
	}
}

func checkRecords(t *testing.T, got, want []wantRecord) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("read %d records, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestReadEveryFormatAndCompression is the read half of the matrix: every
// input format, in every accepted container, opened by path.
func TestReadEveryFormatAndCompression(t *testing.T) {
	for _, fixture := range textFormatFixtures() {
		for _, comp := range readCompressors() {
			t.Run(fixture.format+"/"+comp.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "input"+comp.ext)
				data := comp.compress(t, []byte(fixture.plain))
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}

				reader, err := seqio.OpenPath(path)
				if err != nil {
					t.Fatalf("OpenPath() error = %v", err)
				}
				got := readAllRecords(t, reader)
				if closer, ok := reader.(io.Closer); ok {
					if err := closer.Close(); err != nil {
						t.Fatalf("Close() error = %v", err)
					}
				}
				checkRecords(t, got, fixture.want)
			})
		}
	}
}

// TestReadEveryFormatAndCompressionFromStdin repeats the matrix over stdin,
// where faqt cannot seek and cannot see a filename.
func TestReadEveryFormatAndCompressionFromStdin(t *testing.T) {
	for _, fixture := range textFormatFixtures() {
		for _, comp := range readCompressors() {
			t.Run(fixture.format+"/"+comp.name, func(t *testing.T) {
				data := comp.compress(t, []byte(fixture.plain))
				got := readAllRecordsFromStdin(t, data)
				checkRecords(t, got, fixture.want)
			})
		}
	}
}

func readAllRecordsFromStdin(t *testing.T, data []byte) []wantRecord {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	in, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = in.Close() }()

	oldStdin := os.Stdin
	os.Stdin = in
	defer func() { os.Stdin = oldStdin }()

	reader, err := seqio.OpenPath("-")
	if err != nil {
		t.Fatalf("OpenPath(-) error = %v", err)
	}
	got := readAllRecords(t, reader)
	if closer, ok := reader.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}
	return got
}

// TestReadEveryFormatAndCompressionWithThreads repeats the matrix with more
// than one thread allowed, which takes the parallel BGZF path.
func TestReadEveryFormatAndCompressionWithThreads(t *testing.T) {
	for _, fixture := range textFormatFixtures() {
		for _, comp := range readCompressors() {
			t.Run(fixture.format+"/"+comp.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "input"+comp.ext)
				data := comp.compress(t, []byte(fixture.plain))
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}

				reader, err := seqio.OpenPath(path, seqio.WithThreads(4))
				if err != nil {
					t.Fatalf("OpenPath() error = %v", err)
				}
				got := readAllRecords(t, reader)
				if closer, ok := reader.(io.Closer); ok {
					if err := closer.Close(); err != nil {
						t.Fatalf("Close() error = %v", err)
					}
				}
				checkRecords(t, got, fixture.want)
			})
		}
	}
}

// TestWriteEveryFormatAndCompression is the write half of the matrix. faqt
// writes FASTA and FASTQ only, and has no BGZF writer, so the axes are
// deliberately narrower than for reading.
func TestWriteEveryFormatAndCompression(t *testing.T) {
	records := []*seqio.SeqRecord{
		{Name: "rec1", Description: "first record", Seq: []byte("ACGT"), Qual: []byte("!!!!")},
		{Name: "rec2", Seq: []byte("NNNN"), Qual: []byte("####")},
	}

	formats := []struct {
		format seqio.Format
		want   string
	}{
		{
			format: seqio.FASTA,
			want:   ">rec1 first record\nACGT\n>rec2\nNNNN\n",
		},
		{
			format: seqio.FASTQ,
			want:   "@rec1 first record\nACGT\n+\n!!!!\n@rec2\nNNNN\n+\n####\n",
		},
	}

	compressions := []struct {
		compression seqio.Compression
		ext         string
	}{
		{seqio.CompressNone, ""},
		{seqio.CompressGzip, ".gz"},
		{seqio.CompressBzip2, ".bz2"},
		{seqio.CompressXZ, ".xz"},
		{seqio.CompressZstd, ".zst"},
	}

	for _, f := range formats {
		for _, c := range compressions {
			t.Run(string(f.format)+"/"+string(c.compression), func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "out"+c.ext)
				w, err := seqio.CreatePath(path, f.format, seqio.WithCompression(c.compression))
				if err != nil {
					t.Fatalf("CreatePath() error = %v", err)
				}
				for _, rec := range records {
					if err := w.Write(rec); err != nil {
						t.Fatalf("Write() error = %v", err)
					}
				}
				if err := w.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}

				// Decompress independently of faqt, so the bytes on
				// disk are checked and not just faqt's own read path.
				if got := decompressFile(t, path); got != f.want {
					t.Fatalf("decompressed output = %q, want %q", got, f.want)
				}

				// And read it back through faqt, which is what a
				// user does next.
				reader, err := seqio.OpenPath(path)
				if err != nil {
					t.Fatalf("OpenPath() error = %v", err)
				}
				got := readAllRecords(t, reader)
				if closer, ok := reader.(io.Closer); ok {
					if err := closer.Close(); err != nil {
						t.Fatalf("Close() error = %v", err)
					}
				}
				if len(got) != len(records) {
					t.Fatalf("read back %d records, want %d", len(got), len(records))
				}
			})
		}
	}
}

// TestWriteEveryFormatAndCompressionWithThreads checks that asking for threads
// does not change the bytes written.
func TestWriteEveryFormatAndCompressionWithThreads(t *testing.T) {
	rec := &seqio.SeqRecord{Name: "rec1", Seq: []byte("ACGTACGTACGT")}
	for _, compression := range []seqio.Compression{
		seqio.CompressNone,
		seqio.CompressGzip,
		seqio.CompressBzip2,
		seqio.CompressXZ,
		seqio.CompressZstd,
	} {
		t.Run(string(compression), func(t *testing.T) {
			write := func(threads int) string {
				path := filepath.Join(t.TempDir(), "out")
				w, err := seqio.CreatePath(path, seqio.FASTA,
					seqio.WithCompression(compression), seqio.WithThreads(threads))
				if err != nil {
					t.Fatalf("CreatePath() error = %v", err)
				}
				if err := w.Write(rec); err != nil {
					t.Fatalf("Write() error = %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
				return decompressFile(t, path)
			}
			if one, four := write(1), write(4); one != four {
				t.Fatalf("threads changed output: %q vs %q", one, four)
			}
		})
	}
}

// decompressFile reads path with the compression libraries directly, choosing
// by magic bytes the way faqt does.
func decompressFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	switch {
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}):
		r, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("gzip NewReader() error = %v", err)
		}
		return readAllString(t, r)
	case bytes.HasPrefix(data, []byte("BZh")):
		r, err := dsnetbzip2.NewReader(bytes.NewReader(data), nil)
		if err != nil {
			t.Fatalf("bzip2 NewReader() error = %v", err)
		}
		return readAllString(t, r)
	case bytes.HasPrefix(data, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		r, err := xz.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("xz NewReader() error = %v", err)
		}
		return readAllString(t, r)
	case bytes.HasPrefix(data, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		r, err := zstd.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("zstd NewReader() error = %v", err)
		}
		defer r.Close()
		return readAllString(t, r.IOReadCloser())
	default:
		return string(data)
	}
}

func readAllString(t *testing.T, r io.Reader) string {
	t.Helper()
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return string(out)
}

// TestReadConcatenatedStreams covers inputs made by concatenating compressed
// files, which is how sequencing runs and lane splits often arrive.
func TestReadConcatenatedStreams(t *testing.T) {
	first := "@rec1\nACGT\n+\n!!!!\n"
	second := "@rec2\nNNNN\n+\n####\n"
	want := []wantRecord{
		{name: "rec1", seq: "ACGT", qual: "!!!!"},
		{name: "rec2", seq: "NNNN", qual: "####"},
	}

	for _, comp := range readCompressors() {
		t.Run(comp.name, func(t *testing.T) {
			joined := append(comp.compress(t, []byte(first)), comp.compress(t, []byte(second))...)
			path := filepath.Join(t.TempDir(), "joined"+comp.ext)
			if err := os.WriteFile(path, joined, 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			reader, err := seqio.OpenPath(path)
			if err != nil {
				t.Fatalf("OpenPath() error = %v", err)
			}
			got := readAllRecords(t, reader)
			if closer, ok := reader.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
			checkRecords(t, got, want)
		})
	}
}

// TestMisleadingCompressionSuffixes checks that detection uses content, not
// the filename, for every container.
func TestMisleadingCompressionSuffixes(t *testing.T) {
	plain := ">rec1\nACGT\n"
	want := []wantRecord{{name: "rec1", seq: "ACGT"}}
	for _, comp := range readCompressors() {
		t.Run(comp.name, func(t *testing.T) {
			// Every fixture is named .fa.zst no matter what it holds.
			path := filepath.Join(t.TempDir(), "misleading.fa.zst")
			if err := os.WriteFile(path, comp.compress(t, []byte(plain)), 0o600); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}
			reader, err := seqio.OpenPath(path)
			if err != nil {
				t.Fatalf("OpenPath() error = %v", err)
			}
			got := readAllRecords(t, reader)
			if closer, ok := reader.(io.Closer); ok {
				if err := closer.Close(); err != nil {
					t.Fatalf("Close() error = %v", err)
				}
			}
			checkRecords(t, got, want)
		})
	}
}

// TestCreatePathBuffersUntilClose pins the buffering contract: output is
// batched, so Close is what makes a file complete. Anything that forgets to
// propagate the error from Close would truncate output silently.
func TestCreatePathBuffersUntilClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.fa")
	w, err := seqio.CreatePath(path, seqio.FASTA)
	if err != nil {
		t.Fatalf("CreatePath() error = %v", err)
	}
	for i := 0; i < 1000; i++ {
		if err := w.Write(&seqio.SeqRecord{Name: "rec", Seq: []byte("ACGT")}); err != nil {
			t.Fatalf("Write() error = %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := bytes.Count(data, []byte(">rec\nACGT\n")); got != 1000 {
		t.Fatalf("wrote %d records, want 1000", got)
	}
}

// TestNewWriterIsUnbuffered pins the other half: a caller-supplied writer sees
// records immediately, because nothing guarantees Close is ever called.
func TestNewWriterIsUnbuffered(t *testing.T) {
	var buf bytes.Buffer
	w := seqio.NewFASTAWriter(&buf)
	if err := w.Write(&seqio.SeqRecord{Name: "rec1", Seq: []byte("ACGT")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if got := buf.String(); got != ">rec1\nACGT\n" {
		t.Fatalf("output before Close = %q, want the record", got)
	}
}

// TestWriterFlushMakesRecordsVisible covers Flush, the supported way to get a
// record out of the buffer without closing the writer.
func TestWriterFlushMakesRecordsVisible(t *testing.T) {
	var sink bytes.Buffer
	w, err := seqio.OpenWriter(&sink, seqio.FASTQ, seqio.WithCompression(seqio.CompressNone))
	if err != nil {
		t.Fatalf("OpenWriter() error = %v", err)
	}
	rec := &seqio.SeqRecord{Name: "rec1", Seq: []byte("ACGT"), Qual: []byte("!!!!")}
	if err := w.Write(rec); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if sink.Len() != 0 {
		t.Fatalf("output before Flush = %d bytes, want it still buffered", sink.Len())
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if got := sink.String(); got != "@rec1\nACGT\n+\n!!!!\n" {
		t.Fatalf("output after Flush = %q", got)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := sink.String(); got != "@rec1\nACGT\n+\n!!!!\n" {
		t.Fatalf("Close duplicated or lost output: %q", got)
	}
}

// TestWriterFlushOnUnbufferedWriter checks Flush is safe on a writer that does
// not buffer, so callers need not care which constructor was used.
func TestWriterFlushOnUnbufferedWriter(t *testing.T) {
	var sink bytes.Buffer
	w := seqio.NewFASTAWriter(&sink)
	if err := w.Write(&seqio.SeqRecord{Name: "rec1", Seq: []byte("ACGT")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}
	if got := sink.String(); got != ">rec1\nACGT\n" {
		t.Fatalf("output = %q", got)
	}
}
