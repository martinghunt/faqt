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
	)
	cmd := &cobra.Command{
		Use:   "stats [files...]",
		Short: "Report assembly-style sequence statistics",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := statsFormat(greppy, tabDelimited, tabNoHeader)
			if err != nil {
				return err
			}
			results := make([]stats.Stats, 0, len(args))
			for _, path := range args {
				s, err := stats.FromPath(path, minimumLength)
				if err != nil {
					return err
				}
				results = append(results, s)
			}
			_, err = fmt.Fprint(os.Stdout, stats.RenderMany(results, format))
			return err
		},
	}
	cmd.Flags().IntVarP(&minimumLength, "length", "l", 1, "Minimum length cutoff for each sequence")
	cmd.Flags().BoolVarP(&greppy, "greppy", "s", false, "Print grep friendly output")
	cmd.Flags().BoolVarP(&tabDelimited, "tab", "t", false, "Print tab-delimited output")
	cmd.Flags().BoolVarP(&tabNoHeader, "tab-no-header", "u", false, "Print tab-delimited output with no header line")
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
