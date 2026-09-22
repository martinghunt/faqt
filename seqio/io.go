package seqio

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	seqagc "github.com/martinghunt/faqt/agc"
	"github.com/martinghunt/faqt/bam"
	"github.com/martinghunt/faqt/clustal"
	"github.com/martinghunt/faqt/embl"
	"github.com/martinghunt/faqt/fasta"
	"github.com/martinghunt/faqt/fastq"
	"github.com/martinghunt/faqt/genbank"
	"github.com/martinghunt/faqt/gff3"
	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/martinghunt/faqt/internal/sniff"
	"github.com/martinghunt/faqt/internal/xopen"
	"github.com/martinghunt/faqt/phylip"
	seqsam "github.com/martinghunt/faqt/sam"
)

const (
	// srcBufSize buffers the raw, still compressed input. It must hold one
	// whole BGZF block so BAM can be told from bgzipped text without
	// consuming the stream.
	srcBufSize = 256 << 10
	// readBufSize buffers the decompressed stream the format parsers read
	// lines from. It must be at least sniff.PeekSize.
	readBufSize = 256 << 10
	// writeBufSize batches output so records do not each cost write
	// syscalls. Unbuffered, one FASTQ record costs five writes and a
	// wrapped FASTA line costs two.
	writeBufSize = 256 << 10
)

// ErrEmptyInput is returned when an input holds no data, and so has no format
// to detect. Callers for which no records is a valid answer can use
// OpenPathAllowEmpty instead.
var ErrEmptyInput = sniff.ErrEmptyInput

type Reader interface {
	Read() (*SeqRecord, error)
}

var _ Reader = (*seqagc.Reader)(nil)
var _ Reader = (*seqagc.AllReader)(nil)

// emptyReader is a Reader over no records.
type emptyReader struct{}

func (emptyReader) Read() (*SeqRecord, error) {
	return nil, io.EOF
}

type WriteCloser interface {
	Write(*SeqRecord) error
	Close() error
}

type readerWithCloser struct {
	Reader
	closer io.Closer
}

func (r *readerWithCloser) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

func OpenReader(r io.Reader, opts ...Option) (Reader, error) {
	options := newOptions(opts...)
	rawCloser, _ := r.(io.Closer)
	return openBufferedReader(bufio.NewReaderSize(r, srcBufSize), rawCloser, false, options.threads)
}

func OpenPath(path string, opts ...Option) (Reader, error) {
	options := newOptions(opts...)
	src, err := openPathSource(path)
	if err != nil {
		return nil, err
	}
	reader, err := openBufferedReader(bufio.NewReaderSize(src, srcBufSize), src, true, options.threads)
	if err == nil || path == "-" || !errors.Is(err, sniff.ErrUnknownFormat) {
		return reader, err
	}
	archive, agcErr := seqagc.OpenPath(path)
	if agcErr != nil {
		if errors.Is(agcErr, seqagc.ErrUnsupportedVersion) {
			return nil, agcErr
		}
		return nil, err
	}
	all, agcErr := archive.OpenAll()
	if agcErr != nil {
		_ = archive.Close()
		return nil, agcErr
	}
	return &readerWithCloser{Reader: all, closer: archive}, nil
}

// OpenPathAllowEmpty is OpenPath, except that an input holding no data yields a
// reader over no records instead of ErrEmptyInput.
func OpenPathAllowEmpty(path string, opts ...Option) (Reader, error) {
	reader, err := OpenPath(path, opts...)
	if errors.Is(err, ErrEmptyInput) {
		return emptyReader{}, nil
	}
	return reader, err
}

func openPathSource(path string) (io.ReadCloser, error) {
	if path == "-" {
		return noCloseReadCloser{Reader: os.Stdin}, nil
	}
	return os.Open(path)
}

func openBufferedReader(raw *bufio.Reader, sourceCloser io.Closer, closeSourceOnError bool, threads int) (Reader, error) {
	isBGZF, err := xopen.IsBGZF(raw)
	if err != nil {
		closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
		return nil, err
	}
	if isBGZF {
		// BGZF carries both BAM and bgzipped text, so look at what is
		// inside before choosing a reader. Only BAM goes to the BAM
		// reader; bgzipped FASTA, FASTQ, SAM and the rest fall through
		// to ordinary decompression and format detection.
		isBAM, err := xopen.IsBAM(raw)
		if err != nil {
			closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
			return nil, err
		}
		if isBAM {
			br, err := bam.NewReaderWithThreads(raw, threads)
			if err != nil {
				closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
				return nil, err
			}
			return &readerWithCloser{Reader: br, closer: closeutil.MultiCloser(sourceCloser, br)}, nil
		}
	}
	rc, err := xopen.WrapReader(raw, threads)
	if err != nil {
		closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
		return nil, err
	}
	br := bufio.NewReaderSize(rc, readBufSize)
	detected, err := sniff.Format(br)
	if err != nil {
		closeReaderSetupError(sourceCloser, rc, closeSourceOnError)
		return nil, err
	}
	inner, err := newFormatReader(br, Format(detected))
	if err != nil {
		closeReaderSetupError(sourceCloser, rc, closeSourceOnError)
		return nil, err
	}
	return &readerWithCloser{Reader: inner, closer: closeutil.MultiCloser(sourceCloser, rc)}, nil
}

func closeReaderSetupError(sourceCloser, wrappedCloser io.Closer, closeSource bool) {
	if closeSource {
		_ = closeutil.MultiCloser(sourceCloser, wrappedCloser).Close()
		return
	}
	if wrappedCloser != nil {
		_ = wrappedCloser.Close()
	}
}

type noCloseReadCloser struct {
	io.Reader
}

func (noCloseReadCloser) Close() error {
	return nil
}

func newFormatReader(r *bufio.Reader, format Format) (Reader, error) {
	switch format {
	case FASTA:
		return fasta.NewReader(r), nil
	case FASTQ:
		return fastq.NewReader(r), nil
	case SAM:
		return seqsam.NewReader(r)
	case PHYLIP:
		return phylip.NewReader(r)
	case CLUSTAL:
		return clustal.NewReader(r)
	case GenBank:
		return genbank.NewReader(r), nil
	case EMBL:
		return embl.NewReader(r), nil
	case GFF3:
		return gff3.NewReader(r), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

type Writer struct {
	format Format
	w      io.Writer
	buf    *bufio.Writer
	closer io.Closer
	wrap   int
}

// NewFASTAWriter writes to w as it is given, without buffering, because the
// caller owns w and need not call Close. Use OpenFASTAWriter or
// CreateFASTAPath for buffered output.
func NewFASTAWriter(w io.Writer, opts ...Option) *Writer {
	return NewWriter(w, FASTA, opts...)
}

func OpenFASTAWriter(w io.Writer, opts ...Option) (*Writer, error) {
	return OpenWriter(w, FASTA, opts...)
}

func CreateFASTAPath(path string, opts ...Option) (*Writer, error) {
	return CreatePath(path, FASTA, opts...)
}

// NewFASTQWriter writes to w as it is given, without buffering, because the
// caller owns w and need not call Close. Use OpenFASTQWriter or
// CreateFASTQPath for buffered output.
func NewFASTQWriter(w io.Writer, opts ...Option) *Writer {
	return NewWriter(w, FASTQ, opts...)
}

func OpenFASTQWriter(w io.Writer, opts ...Option) (*Writer, error) {
	return OpenWriter(w, FASTQ, opts...)
}

func CreateFASTQPath(path string, opts ...Option) (*Writer, error) {
	return CreatePath(path, FASTQ, opts...)
}

// NewWriter writes records to w unbuffered. The caller owns w, so there is no
// guarantee Close will ever be called and nothing may be left pending in a
// buffer.
func NewWriter(w io.Writer, format Format, opts ...Option) *Writer {
	options := newOptions(opts...)
	return &Writer{w: w, format: format, wrap: options.wrap}
}

// OpenWriter writes compressed records to w. The returned Writer buffers, so
// Close must be called to flush it.
func OpenWriter(w io.Writer, format Format, opts ...Option) (*Writer, error) {
	options := newOptions(opts...)
	wrapped, closer, err := xopen.WrapWriter(w, string(options.compression), options.threads)
	if err != nil {
		return nil, err
	}
	buf := bufio.NewWriterSize(wrapped, writeBufSize)
	return &Writer{w: buf, buf: buf, closer: closer, format: format, wrap: options.wrap}, nil
}

// CreatePath creates path and writes records to it, or to stdout for "-". The
// returned Writer buffers, so Close must be called to flush it.
func CreatePath(path string, format Format, opts ...Option) (*Writer, error) {
	options := newOptions(opts...)
	if options.compression == CompressAuto {
		if path == "-" {
			options.compression = CompressNone
		} else {
			options.compression = CompressionFromPath(path)
		}
	}
	var (
		base   io.Writer
		closer io.Closer
	)
	if path == "-" {
		base = os.Stdout
	} else {
		fh, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		base = fh
		closer = fh
	}
	wrapped, wcloser, err := xopen.WrapWriter(base, string(options.compression), options.threads)
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	buf := bufio.NewWriterSize(wrapped, writeBufSize)
	return &Writer{
		w:      buf,
		buf:    buf,
		closer: closeutil.MultiCloser(closer, wcloser),
		format: format,
		wrap:   options.wrap,
	}, nil
}

func (w *Writer) Write(rec *SeqRecord) error {
	if rec == nil {
		return fmt.Errorf("cannot write nil record")
	}
	switch w.format {
	case FASTA:
		return fasta.WriteRecord(w.w, *rec, w.wrap)
	case FASTQ:
		return fastq.WriteRecord(w.w, *rec)
	default:
		return fmt.Errorf("unsupported output format %q", w.format)
	}
}

// Flush writes records held in the output buffer to the underlying writer.
// Writers created by CreatePath and OpenWriter batch records, so a Write does
// not reach the file or a reader downstream until the buffer fills, Flush is
// called, or Close is called. Use it when something is waiting on the output,
// such as a process reading a pipe record by record.
//
// Flush does not close anything, and Close flushes on its own, so ordinary
// callers writing a whole file need only Close.
func (w *Writer) Flush() error {
	if w.buf != nil {
		return w.buf.Flush()
	}
	return nil
}

// Close flushes buffered records and closes any compressor and file this
// Writer owns. Output is incomplete until it returns without error, so its
// error must be propagated rather than discarded.
func (w *Writer) Close() error {
	if w.buf != nil {
		if err := w.buf.Flush(); err != nil {
			// Still close, so a failed flush does not also leak the
			// file descriptor, but report the flush error: it is the
			// one that explains the truncated output.
			if w.closer != nil {
				_ = w.closer.Close()
			}
			return err
		}
	}
	if w.closer != nil {
		return w.closer.Close()
	}
	return nil
}
