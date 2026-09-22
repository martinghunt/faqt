package main

import (
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

// threadsFlagHelp explains the default so nobody has to discover it by
// watching a job get killed.
const threadsFlagHelp = "Worker threads for compression and decompression; 1 keeps faqt on one core"

type inputOptions struct {
	threads int
}

func addInputFlags(cmd *cobra.Command, opts *inputOptions) {
	cmd.Flags().IntVar(&opts.threads, "threads", 1, threadsFlagHelp)
}

func (opts inputOptions) seqioOptions() []seqio.Option {
	return []seqio.Option{seqio.WithThreads(opts.threads)}
}
