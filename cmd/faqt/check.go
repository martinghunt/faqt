package main

import (
	"fmt"

	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newCheckCmd() *cobra.Command {
	var input inputOptions
	var allErrors bool
	cmd := &cobra.Command{
		Use:   "check [files...]",
		Short: "Check that every sequence record can be read",
		Long: "Read every record using faqt's sequence parsers, detecting format and compression from content.\n" +
			"Report parser and decompression errors, including corrupt compression trailers.\n" +
			"This checks sequence parsing, not annotations, alignments, or a specific biological alphabet.\n" +
			"Defaults to stdin and stops at the first error.\n" +
			"With --all-errors, reports recoverable errors within files and continues across files.\n" +
			"Structural and decompression errors stop checking the affected file.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if len(args) == 0 {
				args = []string{"-"}
			}
			failed := 0
			for _, path := range args {
				var writeErr error
				count, err := seqio.CheckPath(path, allErrors, func(diagnostic error) {
					if writeErr == nil {
						_, writeErr = fmt.Fprintf(cmd.OutOrStdout(), "%s\tERROR\t%v\n", path, diagnostic)
					}
				}, input.seqioOptions()...)
				if err != nil {
					failed++
				} else {
					_, writeErr = fmt.Fprintf(cmd.OutOrStdout(), "%s\tOK\t%d records\n", path, count)
				}
				if writeErr != nil {
					return writeErr
				}
				if err != nil && !allErrors {
					return fmt.Errorf("%s: %w", path, err)
				}
			}
			if failed > 0 {
				return fmt.Errorf("%d input(s) failed validation", failed)
			}
			return nil
		},
	}
	addInputFlags(cmd, &input)
	cmd.Flags().BoolVar(&allErrors, "all-errors", false, "Report recoverable errors within each input and continue checking other files")
	return cmd
}
