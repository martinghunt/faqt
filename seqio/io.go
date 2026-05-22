package seqio

import (
	"bufio"
	"fmt"
	"io"
	"os"

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

type Reader interface {
	Read() (*SeqRecord, error)
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

func OpenReader(r io.Reader) (Reader, error) {
	rawCloser, _ := r.(io.Closer)
	return openBufferedReader(bufio.NewReader(r), rawCloser, false)
}

func OpenPath(path string) (Reader, error) {
	src, err := openPathSource(path)
	if err != nil {
		return nil, err
	}
	return openBufferedReader(bufio.NewReader(src), src, true)
}

func openPathSource(path string) (io.ReadCloser, error) {
	if path == "-" {
		return noCloseReadCloser{Reader: os.Stdin}, nil
	}
	return os.Open(path)
}

func openBufferedReader(raw *bufio.Reader, sourceCloser io.Closer, closeSourceOnError bool) (Reader, error) {
	isBGZF, err := xopen.IsBGZF(raw)
	if err != nil {
		closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
		return nil, err
	}
	if isBGZF {
		br, err := bam.NewReader(raw)
		if err != nil {
			closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
			return nil, err
		}
		return &readerWithCloser{Reader: br, closer: closeutil.MultiCloser(sourceCloser, br)}, nil
	}
	rc, err := xopen.WrapReader(raw)
	if err != nil {
		closeReaderSetupError(sourceCloser, nil, closeSourceOnError)
		return nil, err
	}
	br := bufio.NewReaderSize(rc, sniff.PeekSize)
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
	closer io.Closer
	wrap   int
}

func NewFASTAWriter(w io.Writer, opts ...Option) *Writer {
	return NewWriter(w, FASTA, opts...)
}

func OpenFASTAWriter(w io.Writer, opts ...Option) (*Writer, error) {
	return OpenWriter(w, FASTA, opts...)
}

func CreateFASTAPath(path string, opts ...Option) (*Writer, error) {
	return CreatePath(path, FASTA, opts...)
}

func NewFASTQWriter(w io.Writer, opts ...Option) *Writer {
	return NewWriter(w, FASTQ, opts...)
}

func OpenFASTQWriter(w io.Writer, opts ...Option) (*Writer, error) {
	return OpenWriter(w, FASTQ, opts...)
}

func CreateFASTQPath(path string, opts ...Option) (*Writer, error) {
	return CreatePath(path, FASTQ, opts...)
}

func NewWriter(w io.Writer, format Format, opts ...Option) *Writer {
	options := newOptions(opts...)
	return &Writer{w: w, format: format, wrap: options.wrap}
}

func OpenWriter(w io.Writer, format Format, opts ...Option) (*Writer, error) {
	options := newOptions(opts...)
	wrapped, closer, err := xopen.WrapWriter(w, string(options.compression))
	if err != nil {
		return nil, err
	}
	return &Writer{w: wrapped, closer: closer, format: format, wrap: options.wrap}, nil
}

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
	wrapped, wcloser, err := xopen.WrapWriter(base, string(options.compression))
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, err
	}
	return &Writer{
		w:      wrapped,
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

func (w *Writer) Close() error {
	if w.closer != nil {
		return w.closer.Close()
	}
	return nil
}
