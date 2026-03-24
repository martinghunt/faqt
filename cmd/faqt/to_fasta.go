package main

import (
	"bytes"

	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newToFastaCmd() *cobra.Command {
	var (
		inputPath    string
		outputPath   string
		wrap         int
		compress     string
		removeDashes bool
	)
	cmd := &cobra.Command{
		Use:   "to-fasta [input]",
		Short: "Convert supported sequence input to FASTA",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 && inputPath == "-" {
				inputPath = args[0]
			}
			return seqio.ToFASTAPathWithTransform(
				inputPath,
				outputPath,
				removeDashesTransform(removeDashes),
				seqio.WithWrap(wrap),
				seqio.WithCompression(seqio.Compression(compress)),
			)
		},
	}
	cmd.Flags().StringVarP(&inputPath, "input", "i", "-", "Input path or - for stdin")
	cmd.Flags().StringVarP(&outputPath, "output", "o", "-", "Output path or - for stdout")
	cmd.Flags().IntVar(&wrap, "wrap", 0, "FASTA wrap width; 0 disables wrapping")
	cmd.Flags().StringVar(&compress, "compress", string(seqio.CompressAuto), "Output compression: auto, none, gzip, bzip2, xz, zstd")
	cmd.Flags().BoolVar(&removeDashes, "remove-dashes", false, "Remove '-' characters from output sequences")
	return cmd
}

func removeDashesTransform(remove bool) seqio.RecordTransform {
	if !remove {
		return nil
	}
	return func(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
		copyRec := *rec
		copyRec.Seq = bytes.ReplaceAll(rec.Seq, []byte("-"), nil)
		return &copyRec, nil
	}
}
