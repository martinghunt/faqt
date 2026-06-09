package main

import (
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

type sequenceOutputOptions struct {
	path     string
	wrap     int
	compress string
}

func addSequenceOutputFlags(cmd *cobra.Command, opts *sequenceOutputOptions) {
	cmd.Flags().StringVarP(&opts.path, "output", "o", "-", "Output path or - for stdout")
	cmd.Flags().IntVar(&opts.wrap, "wrap", 0, "FASTA wrap width; default 0 disables wrapping")
	cmd.Flags().StringVar(&opts.compress, "compress", string(seqio.CompressAuto), "Output compression: auto, none, gzip, bzip2, xz, zstd")
}

func (opts sequenceOutputOptions) seqioOptions() []seqio.Option {
	return []seqio.Option{
		seqio.WithWrap(opts.wrap),
		seqio.WithCompression(seqio.Compression(opts.compress)),
	}
}
