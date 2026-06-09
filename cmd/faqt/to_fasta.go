package main

import (
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newToFastaCmd() *cobra.Command {
	var (
		inputPath    string
		output       sequenceOutputOptions
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
				output.path,
				removeDashesTransform(removeDashes),
				output.seqioOptions()...,
			)
		},
	}
	cmd.Flags().StringVarP(&inputPath, "input", "i", "-", "Input path or - for stdin")
	addSequenceOutputFlags(cmd, &output)
	cmd.Flags().BoolVar(&removeDashes, "remove-dashes", false, "Remove '-' characters from output sequences")
	return cmd
}

func removeDashesTransform(remove bool) seqio.RecordTransform {
	if !remove {
		return nil
	}
	return seqio.RemoveDashes
}
