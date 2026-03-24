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
	htssam "github.com/biogo/hts/sam"
	dsnetbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
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
			r, err := WrapReader(bytes.NewReader(tc.data(t)))
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

			w, closer, err := WrapWriter(fh, tc.compression)
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

			r, err := Open(path)
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
	if _, _, err := WrapWriter(io.Discard, "bogus"); err == nil {
		t.Fatal("WrapWriter() error = nil, want unsupported compression error")
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
	mc := newMultiCloser(nil, first, second)
	if err := mc.Close(); err != io.EOF {
		t.Fatalf("Close() error = %v, want EOF", err)
	}
	if !first.closed || !second.closed {
		t.Fatalf("closed flags = (%v, %v), want both true", first.closed, second.closed)
	}
}
