package main

import (
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newInterleaveCmd() *cobra.Command {
	var (
		output  sequenceOutputOptions
		suffix1 string
		suffix2 string
	)
	cmd := &cobra.Command{
		Use:   "interleave [options] INPUT_1 INPUT_2",
		Short: "Interleave two sequence files",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return seqio.InterleavePath(
				args[0],
				args[1],
				output.path,
				seqio.InterleaveOptions{Suffix1: suffix1, Suffix2: suffix2},
				output.seqioOptions()...,
			)
		},
	}
	addSequenceOutputFlags(cmd, &output)
	cmd.Flags().StringVar(&suffix1, "suffix1", "", "Suffix to add to names from INPUT_1 if missing")
	cmd.Flags().StringVar(&suffix2, "suffix2", "", "Suffix to add to names from INPUT_2 if missing")
	return cmd
}
