package xopen

import (
	"bufio"
	"bytes"
	"compress/bzip2"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/biogo/hts/bgzf"
	dsnetbzip2 "github.com/dsnet/compress/bzip2"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/zstd"
	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/ulikunitz/xz"
)

const sniffSize = 16
const bgzfHeaderSize = 18

// gzipLevel is pinned rather than left to the library's default, so the
// compression ratio does not move when the library does. klauspost/compress
// defaults to 5 where compress/gzip defaulted to 6; level 6 keeps the output
// faqt has always produced, byte for byte, and klauspost reaches it about 12%
// faster than the standard library did.
const gzipLevel = 6

// maxBGZFBlock is the largest possible BGZF block, including its header and
// footer. The first block is enough to tell BAM from ordinary text.
const maxBGZFBlock = 65536

// bamMagic starts the uncompressed payload of every BAM file.
var bamMagic = []byte{'B', 'A', 'M', 0x01}

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

// IsBAM reports whether a BGZF stream holds BAM rather than compressed text.
// BGZF is the container samtools uses for both BAM and bgzipped FASTA/FASTQ,
// so the container alone cannot decide which reader to use. It decompresses
// the first block from peeked bytes and looks for the BAM magic number,
// leaving r unread.
func IsBAM(r *bufio.Reader) (bool, error) {
	header, err := r.Peek(bgzfHeaderSize)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return false, err
	}
	if len(header) < bgzfHeaderSize {
		return false, nil
	}
	// BSIZE holds the whole block's size minus one, so peek exactly one
	// block rather than waiting for a slow writer to fill a larger peek.
	size := (int(header[16]) | int(header[17])<<8) + 1
	if size < bgzfHeaderSize || size > maxBGZFBlock {
		return false, nil
	}
	block, err := r.Peek(size)
	if err != nil && err != io.EOF && err != bufio.ErrBufferFull {
		return false, err
	}
	if len(block) < bgzfHeaderSize {
		return false, nil
	}
	gr, err := gzip.NewReader(bytes.NewReader(block))
	if err != nil {
		// Not readable as gzip, so not BAM. Let the caller's normal
		// decompression path report any error.
		return false, nil
	}
	defer func() { _ = gr.Close() }()
	gr.Multistream(false)
	magic := make([]byte, len(bamMagic))
	if _, err := io.ReadFull(gr, magic); err != nil {
		return false, nil
	}
	return bytes.Equal(magic, bamMagic), nil
}

func Open(path string, threads int) (io.ReadCloser, error) {
	if path == "-" {
		return WrapReader(os.Stdin, threads)
	}
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	rc, err := WrapReader(fh, threads)
	if err != nil {
		_ = fh.Close()
		return nil, err
	}
	return &readCloser{Reader: rc, closer: closeutil.MultiCloser(fh, rc)}, nil
}

// WrapReader decompresses without an output size limit. This is deliberate:
// faqt's normal workload is multi-gigabyte genome/read files, and a size cap
// would break that. Callers are expected to point faqt at files/accessions
// they already trust, not arbitrary untrusted uploads.
//
// threads bounds the worker goroutines any decompressor may start. One keeps
// decompression on the calling goroutine, so faqt uses a single core unless
// asked for more.
func WrapReader(r io.Reader, threads int) (io.ReadCloser, error) {
	threads = normalizeThreads(threads)
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
		// BGZF is gzip, but its independent blocks can be inflated in
		// parallel when the caller has asked for more than one thread.
		if threads > 1 {
			isBGZF, err := IsBGZF(br)
			if err != nil {
				return nil, err
			}
			if isBGZF {
				return bgzf.NewReader(br, threads)
			}
		}
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
		zr, err := zstd.NewReader(br, zstd.WithDecoderConcurrency(threads))
		if err != nil {
			return nil, err
		}
		return zr.IOReadCloser(), nil
	default:
		return io.NopCloser(br), nil
	}
}

// WrapWriter compresses output with c. threads bounds the worker goroutines
// the compressor may start; one keeps compression on the calling goroutine.
func WrapWriter(w io.Writer, c string, threads int) (io.Writer, io.Closer, error) {
	threads = normalizeThreads(threads)
	switch c {
	case "auto", "none", "":
		return w, nil, nil
	case "gzip":
		gw, err := gzip.NewWriterLevel(w, gzipLevel)
		if err != nil {
			return nil, nil, err
		}
		// RFC 1952 section 2.3.1: a zero MTIME means no modification
		// time is available, which is what faqt wants, since output is
		// a stream and not a copy of a file. compress/gzip special
		// cased the zero time; klauspost/compress casts it, turning it
		// into a date in 2042, so ask for the epoch explicitly.
		gw.ModTime = time.Unix(0, 0)
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
		zw, err := zstd.NewWriter(w, zstd.WithEncoderConcurrency(threads))
		if err != nil {
			return nil, nil, err
		}
		return zw, zw, nil
	default:
		return nil, nil, fmt.Errorf("unsupported compression %q", c)
	}
}

// normalizeThreads keeps a missing or nonsensical thread count at one, so no
// code path can accidentally fan out across every core.
func normalizeThreads(threads int) int {
	if threads < 1 {
		return 1
	}
	return threads
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
