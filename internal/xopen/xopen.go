package xopen

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	dsnetbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/zstd"
	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/ulikunitz/xz"
)

const sniffSize = 16
const bgzfHeaderSize = 18

func CompressionFromPath(path string) string {
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".gz"):
		return "gzip"
	case strings.HasSuffix(strings.ToLower(path), ".bz2"):
		return "bzip2"
	case strings.HasSuffix(strings.ToLower(path), ".xz"):
		return "xz"
	case strings.HasSuffix(strings.ToLower(path), ".zst"):
		return "zstd"
	default:
		return "none"
	}
}

func BasePathWithoutCompression(path string) string {
	lower := strings.ToLower(path)
	for _, suffix := range []string{".gz", ".bz2", ".xz", ".zst"} {
		if strings.HasSuffix(lower, suffix) {
			return path[:len(path)-len(suffix)]
		}
	}
	return path
}

func IsBGZF(r *bufio.Reader) (bool, error) {
	header, err := r.Peek(bgzfHeaderSize)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return false, err
	}
	if len(header) < bgzfHeaderSize {
		return false, nil
	}
	if header[0] != 0x1f || header[1] != 0x8b || header[2] != 0x08 {
		return false, nil
	}
	if header[3]&0x04 == 0 {
		return false, nil
	}
	xlen := int(header[10]) | int(header[11])<<8
	if xlen < 6 {
		return false, nil
	}
	return header[12] == 'B' && header[13] == 'C' && header[14] == 2 && header[15] == 0, nil
}

func Open(path string) (io.ReadCloser, error) {
	if path == "-" {
		return WrapReader(os.Stdin)
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	rc, err := WrapReader(fh)
	if err != nil {
		_ = fh.Close()
		return nil, err
	}
	return &readCloser{Reader: rc, closer: closeutil.MultiCloser(fh, rc)}, nil
}

func WrapReader(r io.Reader) (io.ReadCloser, error) {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	magic, err := br.Peek(sniffSize)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return nil, err
	}
	switch {
	case bytes.HasPrefix(magic, []byte{0x1f, 0x8b}):
		gr, err := gzip.NewReader(br)
		if err != nil {
			return nil, err
		}
		return gr, nil
	case bytes.HasPrefix(magic, []byte("BZh")):
		return io.NopCloser(bzip2.NewReader(br)), nil
	case bytes.HasPrefix(magic, []byte{0xfd, '7', 'z', 'X', 'Z', 0x00}):
		xzr, err := xz.NewReader(br)
		if err != nil {
			return nil, err
		}
		return io.NopCloser(xzr), nil
	case bytes.HasPrefix(magic, []byte{0x28, 0xb5, 0x2f, 0xfd}):
		zr, err := zstd.NewReader(br)
		if err != nil {
			return nil, err
		}
		return zr.IOReadCloser(), nil
	default:
		return io.NopCloser(br), nil
	}
}

func WrapWriter(w io.Writer, c string) (io.Writer, io.Closer, error) {
	switch c {
	case "auto", "none", "":
		return w, nil, nil
	case "gzip":
		gw := gzip.NewWriter(w)
		return gw, gw, nil
	case "bzip2":
		bw, err := dsnetbzip2.NewWriter(w, nil)
		if err != nil {
			return nil, nil, err
		}
		return bw, bw, nil
	case "xz":
		xzw, err := xz.NewWriter(w)
		if err != nil {
			return nil, nil, err
		}
		return xzw, xzw, nil
	case "zstd":
		zw, err := zstd.NewWriter(w)
		if err != nil {
			return nil, nil, err
		}
		return zw, zw, nil
	default:
		return nil, nil, fmt.Errorf("unsupported compression %q", c)
	}
}

type readCloser struct {
	io.Reader
	closer io.Closer
}

func (r *readCloser) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}
