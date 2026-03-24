package seqio

import (
	"path/filepath"
	"strings"
)

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
	switch {
	case strings.HasSuffix(strings.ToLower(path), ".gz"):
		return CompressGzip
	case strings.HasSuffix(strings.ToLower(path), ".bz2"):
		return CompressBzip2
	case strings.HasSuffix(strings.ToLower(path), ".xz"):
		return CompressXZ
	case strings.HasSuffix(strings.ToLower(path), ".zst"):
		return CompressZstd
	default:
		return CompressNone
	}
}

func BasePathWithoutCompression(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".gz", ".xz", ".zst":
		return strings.TrimSuffix(path, filepath.Ext(path))
	case ".bz2":
		return strings.TrimSuffix(path, ".bz2")
	default:
		return path
	}
}
