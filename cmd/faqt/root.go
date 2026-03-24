package main

import (
	"github.com/martinghunt/faqt/internal/buildinfo"
	"github.com/spf13/cobra"
)

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "faqt",
		Short:   "Sequence-only toolkit for reading and converting sequence files",
		Version: buildinfo.Version,
	}
	cmd.SetVersionTemplate("{{.Version}}\n")
	cmd.AddCommand(newToFastaCmd())
	cmd.AddCommand(newToPerfectReadsCmd())
	cmd.AddCommand(newStatsCmd())
	cmd.AddCommand(newDownloadGenomeCmd())
	return cmd
}
