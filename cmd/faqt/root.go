package main

import (
	"github.com/martinghunt/faqt/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "faqt",
		Short:   "Sequence-only toolkit for reading and converting sequence files",
		Version: displayVersion(buildinfo.Version),
	}
	cmd.SetVersionTemplate("{{.Name}} {{.Version}}\n")
	cmd.AddCommand(newInterleaveCmd())
	cmd.AddCommand(newToFastaCmd())
	cmd.AddCommand(newToPerfectReadsCmd())
	cmd.AddCommand(newMakeRandomContigsCmd())
	cmd.AddCommand(newStatsCmd())
	cmd.AddCommand(newDownloadCmd())
	cmd.AddCommand(newDownloadReadsCmd())
	return cmd
}

func displayVersion(raw string) string {
	if len(raw) > 1 && (raw[0] == 'v' || raw[0] == 'V') && raw[1] >= '0' && raw[1] <= '9' {
		return raw[1:]
	}
	return raw
}
