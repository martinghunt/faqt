package main

import (
	"github.com/martinghunt/faqt/genomedl"
	"github.com/spf13/cobra"
)

func newDownloadGenomeCmd() *cobra.Command {
	var (
		outputPath string
		fastaOnly  bool
	)
	cmd := &cobra.Command{
		Use:   "download-genome ACCESSION",
		Short: "Download a genome and save annotation or FASTA output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := genomedl.DownloadGenomeWithOptions(args[0], outputPath, genomedl.DownloadOptions{
				FastaOnly:     fastaOnly,
				WarningWriter: cmd.ErrOrStderr(),
			})
			return err
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	cmd.Flags().BoolVar(&fastaOnly, "fasta", false, "Download and write genomic FASTA only")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}
