package xopen

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	htsbam "github.com/biogo/hts/bam"
	"github.com/biogo/hts/bgzf"
	htssam "github.com/biogo/hts/sam"
	dsnetbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/ulikunitz/xz"
)

func TestWrapReader(t *testing.T) {
	tests := []struct {
		name string
		data func(t *testing.T) []byte
		want string
	}{
		{
			name: "plain",
			data: func(t *testing.T) []byte { return []byte("plain text") },
			want: "plain text",
		},
		{
			name: "gzip",
			data: func(t *testing.T) []byte {
				var buf bytes.Buffer
				w := gzip.NewWriter(&buf)
				if _, err := w.Write([]byte("gzip text")); err != nil {
					t.Fatalf("gzip Write() error = %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("gzip Close() error = %v", err)
				}
				return buf.Bytes()
			},
			want: "gzip text",
		},
		{
			name: "bzip2",
			data: func(t *testing.T) []byte {
				var buf bytes.Buffer
				w, err := dsnetbzip2.NewWriter(&buf, nil)
				if err != nil {
					t.Fatalf("bzip2 NewWriter() error = %v", err)
				}
				if _, err := w.Write([]byte("bzip2 text")); err != nil {
					t.Fatalf("bzip2 Write() error = %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("bzip2 Close() error = %v", err)
				}
				return buf.Bytes()
			},
			want: "bzip2 text",
		},
		{
			name: "xz",
			data: func(t *testing.T) []byte {
				var buf bytes.Buffer
				w, err := xz.NewWriter(&buf)
				if err != nil {
					t.Fatalf("xz NewWriter() error = %v", err)
				}
				if _, err := w.Write([]byte("xz text")); err != nil {
					t.Fatalf("xz Write() error = %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("xz Close() error = %v", err)
				}
				return buf.Bytes()
			},
			want: "xz text",
		},
		{
			name: "zstd",
			data: func(t *testing.T) []byte {
				var buf bytes.Buffer
				w, err := zstd.NewWriter(&buf)
				if err != nil {
					t.Fatalf("zstd NewWriter() error = %v", err)
				}
				if _, err := w.Write([]byte("zstd text")); err != nil {
					t.Fatalf("zstd Write() error = %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("zstd Close() error = %v", err)
				}
				return buf.Bytes()
			},
			want: "zstd text",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := WrapReader(bytes.NewReader(tc.data(t)), 1)
			if err != nil {
				t.Fatalf("WrapReader() error = %v", err)
			}
			defer r.Close()

			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("ReadAll() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWrapWriterAndOpen(t *testing.T) {
	tests := []struct {
		name        string
		compression string
		path        string
	}{
		{name: "gzip", compression: "gzip", path: "out.fa.gz"},
		{name: "bzip2", compression: "bzip2", path: "out.fa.bz2"},
		{name: "xz", compression: "xz", path: "out.fa.xz"},
		{name: "zstd", compression: "zstd", path: "out.fa.zst"},
		{name: "none", compression: "none", path: "out.fa"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), tc.path)
			fh, err := os.Create(path)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}

			w, closer, err := WrapWriter(fh, tc.compression, 1)
			if err != nil {
				t.Fatalf("WrapWriter() error = %v", err)
			}
			if _, err := io.WriteString(w, "wrapped text"); err != nil {
				t.Fatalf("WriteString() error = %v", err)
			}
			if closer != nil {
				if err := closer.Close(); err != nil {
					t.Fatalf("closer.Close() error = %v", err)
				}
			}
			if err := fh.Close(); err != nil {
				t.Fatalf("fh.Close() error = %v", err)
			}

			r, err := Open(path, 1)
			if err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			defer r.Close()

			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if string(got) != "wrapped text" {
				t.Fatalf("ReadAll() = %q, want wrapped text", got)
			}
		})
	}
}

func TestWrapWriterUnsupportedCompression(t *testing.T) {
	if _, _, err := WrapWriter(io.Discard, "bogus", 1); err == nil {
		t.Fatal("WrapWriter() error = nil, want unsupported compression error", 1)
	}
}

func TestIsBGZF(t *testing.T) {
	if ok, err := IsBGZF(bufio.NewReader(strings.NewReader("plain text"))); err != nil || ok {
		t.Fatalf("IsBGZF(plain) = (%v, %v), want (false, nil)", ok, err)
	}

	var buf bytes.Buffer
	ref, err := htssam.NewReference("ref", "", "", 100, nil, nil)
	if err != nil {
		t.Fatalf("NewReference() error = %v", err)
	}
	header, err := htssam.NewHeader(nil, []*htssam.Reference{ref})
	if err != nil {
		t.Fatalf("NewHeader() error = %v", err)
	}
	w, err := htsbam.NewWriter(&buf, header, 0)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	rec, err := htssam.NewRecord("read1", ref, nil, 0, -1, 0, 0, nil, []byte("ACGT"), []byte{30, 31, 32, 33}, nil)
	if err != nil {
		t.Fatalf("NewRecord() error = %v", err)
	}
	if err := w.Write(rec); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	ok, err := IsBGZF(bufio.NewReader(bytes.NewReader(buf.Bytes())))
	if err != nil {
		t.Fatalf("IsBGZF() error = %v", err)
	}
	if !ok {
		t.Fatal("IsBGZF() = false, want true for BAM data")
	}
}

type closerStub struct {
	closed bool
	err    error
}

func (c *closerStub) Close() error {
	c.closed = true
	return c.err
}

func TestMultiCloserCloseOrderAndError(t *testing.T) {
	first := &closerStub{err: io.EOF}
	second := &closerStub{}
	mc := closeutil.MultiCloser(nil, first, second)
	if err := mc.Close(); err != io.EOF {
		t.Fatalf("Close() error = %v, want EOF", err)
	}
	if !first.closed || !second.closed {
		t.Fatalf("closed flags = (%v, %v), want both true", first.closed, second.closed)
	}
}

// TestIsBAMDistinguishesBGZFContents covers the decision that BGZF alone
// cannot make: the same container carries BAM and bgzipped text.
func TestIsBAMDistinguishesBGZFContents(t *testing.T) {
	bgzipped := func(payload []byte) []byte {
		var buf bytes.Buffer
		w := bgzf.NewWriter(&buf, 1)
		if _, err := w.Write(payload); err != nil {
			t.Fatalf("bgzf Write() error = %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("bgzf Close() error = %v", err)
		}
		return buf.Bytes()
	}

	tests := []struct {
		name     string
		data     []byte
		wantBGZF bool
		wantBAM  bool
	}{
		{
			name:     "bgzipped fasta",
			data:     bgzipped([]byte(">rec1\nACGT\n")),
			wantBGZF: true,
			wantBAM:  false,
		},
		{
			name:     "bgzipped fastq",
			data:     bgzipped([]byte("@rec1\nACGT\n+\n!!!!\n")),
			wantBGZF: true,
			wantBAM:  false,
		},
		{
			name:     "bam",
			data:     bgzipped(append([]byte("BAM\x01"), make([]byte, 64)...)),
			wantBGZF: true,
			wantBAM:  true,
		},
		{
			name:     "plain gzip fasta",
			data:     gzipBytes(t, []byte(">rec1\nACGT\n")),
			wantBGZF: false,
			wantBAM:  false,
		},
		{
			name:     "uncompressed fasta",
			data:     []byte(">rec1\nACGT\n"),
			wantBGZF: false,
			wantBAM:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			br := bufio.NewReaderSize(bytes.NewReader(tc.data), 256<<10)
			gotBGZF, err := IsBGZF(br)
			if err != nil {
				t.Fatalf("IsBGZF() error = %v", err)
			}
			if gotBGZF != tc.wantBGZF {
				t.Fatalf("IsBGZF() = %v, want %v", gotBGZF, tc.wantBGZF)
			}
			gotBAM, err := IsBAM(br)
			if err != nil {
				t.Fatalf("IsBAM() error = %v", err)
			}
			if gotBAM != tc.wantBAM {
				t.Fatalf("IsBAM() = %v, want %v", gotBAM, tc.wantBAM)
			}

			// Neither check may consume the stream.
			rest, err := io.ReadAll(br)
			if err != nil {
				t.Fatalf("ReadAll() error = %v", err)
			}
			if !bytes.Equal(rest, tc.data) {
				t.Fatalf("detection consumed %d of %d bytes", len(tc.data)-len(rest), len(tc.data))
			}
		})
	}
}

func gzipBytes(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return buf.Bytes()
}
