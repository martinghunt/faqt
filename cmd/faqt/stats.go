package main

import (
	"fmt"
	"os"

	"github.com/martinghunt/faqt/stats"
	"github.com/spf13/cobra"
)

func newStatsCmd() *cobra.Command {
	var (
		minimumLength int
		greppy        bool
		tabDelimited  bool
		tabNoHeader   bool
		combineInputs bool
		perSequence   bool
		input         inputOptions
	)
	cmd := &cobra.Command{
		Use:   "stats [files...]",
		Short: "Report assembly-style sequence statistics",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := statsFormat(greppy, tabDelimited, tabNoHeader)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				args = []string{"-"}
			}
			if perSequence {
				if combineInputs {
					return fmt.Errorf("--combine-inputs cannot be used with --per-sequence, which reports one row per sequence")
				}
				format, err = perSequenceFormat(format)
				if err != nil {
					return err
				}
				return stats.WritePerSequence(os.Stdout, args, minimumLength, format, input.seqioOptions()...)
			}
			results, err := stats.FromPaths(args, minimumLength, combineInputs, input.seqioOptions()...)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(os.Stdout, stats.RenderMany(results, format))
			return err
		},
	}
	cmd.Flags().IntVarP(&minimumLength, "length", "l", 1, "Minimum length cutoff for each sequence")
	cmd.Flags().BoolVarP(&greppy, "greppy", "s", false, "Print grep friendly output")
	cmd.Flags().BoolVarP(&tabDelimited, "tab", "t", false, "Print tab-delimited output")
	cmd.Flags().BoolVarP(&tabNoHeader, "tab-no-header", "u", false, "Print tab-delimited output with no header line")
	cmd.Flags().BoolVar(&combineInputs, "combine-inputs", false, "Combine all records from all inputs into one statistics result")
	cmd.Flags().BoolVarP(&perSequence, "per-sequence", "p", false, "Report one tab-delimited row per sequence (file, name, length, N_count, Gaps, GC) instead of one result per input")
	addInputFlags(cmd, &input)
	return cmd
}

func statsFormat(greppy, tabDelimited, tabNoHeader bool) (stats.Format, error) {
	selected := 0
	if greppy {
		selected++
	}
	if tabDelimited {
		selected++
	}
	if tabNoHeader {
		selected++
	}
	if selected > 1 {
		return 0, fmt.Errorf("choose at most one of -s, -t, or -u")
	}
	switch {
	case greppy:
		return stats.FormatGreppy, nil
	case tabDelimited:
		return stats.FormatTab, nil
	case tabNoHeader:
		return stats.FormatTabNoHeader, nil
	default:
		return stats.FormatHuman, nil
	}
}

// perSequenceFormat maps the shared output flags onto the formats a
// per-sequence table can be printed in. The human layout is a per-input
// paragraph and greppy keys on the input name, so neither says anything
// useful about a single sequence; the plain default becomes the tab table.
func perSequenceFormat(format stats.Format) (stats.Format, error) {
	switch format {
	case stats.FormatHuman, stats.FormatTab:
		return stats.FormatTab, nil
	case stats.FormatTabNoHeader:
		return stats.FormatTabNoHeader, nil
	default:
		return 0, fmt.Errorf("--per-sequence has no grep friendly output; use -t or -u")
	}
}
