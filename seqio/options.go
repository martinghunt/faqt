package seqio

import "github.com/martinghunt/faqt/internal/xopen"

type Format string

const (
	FormatUnknown Format = ""
	FASTA         Format = "fasta"
	FASTQ         Format = "fastq"
	SAM           Format = "sam"
	BAM           Format = "bam"
	PHYLIP        Format = "phylip"
	CLUSTAL       Format = "clustal"
	GenBank       Format = "genbank"
	EMBL          Format = "embl"
	GFF3          Format = "gff3"
)

type Compression string

const (
	CompressAuto  Compression = "auto"
	CompressNone  Compression = "none"
	CompressGzip  Compression = "gzip"
	CompressBzip2 Compression = "bzip2"
	CompressXZ    Compression = "xz"
	CompressZstd  Compression = "zstd"
)

type options struct {
	wrap        int
	compression Compression
}

type Option func(*options)

func WithWrap(width int) Option {
	return func(o *options) {
		o.wrap = width
	}
}

func WithCompression(c Compression) Option {
	return func(o *options) {
		o.compression = c
	}
}

func newOptions(opts ...Option) options {
	out := options{compression: CompressAuto}
	for _, opt := range opts {
		opt(&out)
	}
	return out
}

func CompressionFromPath(path string) Compression {
	return Compression(xopen.CompressionFromPath(path))
}

func BasePathWithoutCompression(path string) string {
	return xopen.BasePathWithoutCompression(path)
}
