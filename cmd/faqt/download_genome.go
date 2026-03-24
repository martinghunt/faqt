package main

import (
	"github.com/martinghunt/faqt/genomedl"
	"github.com/spf13/cobra"
)

func newDownloadGenomeCmd() *cobra.Command {
	var outputPath string
	cmd := &cobra.Command{
		Use:   "download-genome ACCESSION",
		Short: "Download a genome and save it as a single output file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := genomedl.DownloadGenome(args[0], outputPath)
			return err
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}
