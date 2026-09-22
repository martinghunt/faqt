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
	AGC           Format = "agc"
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
	threads     int
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

// WithThreads sets the number of worker goroutines readers and writers may
// use to compress or decompress data. The default is one, which keeps faqt on
// a single core; schedulers that allocate one core kill jobs that quietly fan
// out across a whole node. Values above one are honoured only where the format
// allows it, such as BGZF blocks and zstd frames.
func WithThreads(n int) Option {
	return func(o *options) {
		if n < 1 {
			n = 1
		}
		o.threads = n
	}
}

func newOptions(opts ...Option) options {
	out := options{compression: CompressAuto, threads: 1}
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
