package main

import (
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newInterleaveCmd() *cobra.Command {
	var (
		outputPath string
		suffix1    string
		suffix2    string
		compress   string
		wrap       int
	)
	cmd := &cobra.Command{
		Use:   "interleave [options] INPUT_1 INPUT_2",
		Short: "Interleave two sequence files",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return seqio.InterleavePath(
				args[0],
				args[1],
				outputPath,
				seqio.InterleaveOptions{Suffix1: suffix1, Suffix2: suffix2},
				seqio.WithCompression(seqio.Compression(compress)),
				seqio.WithWrap(wrap),
			)
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "-", "Output path or - for stdout")
	cmd.Flags().StringVar(&suffix1, "suffix1", "", "Suffix to add to names from INPUT_1 if missing")
	cmd.Flags().StringVar(&suffix2, "suffix2", "", "Suffix to add to names from INPUT_2 if missing")
	cmd.Flags().StringVar(&compress, "compress", string(seqio.CompressAuto), "Output compression: auto, none, gzip, bzip2, xz, zstd")
	cmd.Flags().IntVar(&wrap, "wrap", 0, "FASTA wrap width; 0 disables wrapping")
	return cmd
}
