package main

import (
	"fmt"
	"time"

	"github.com/martinghunt/faqt/readdl"
	"github.com/spf13/cobra"
)

var downloadReads = readdl.DownloadReads

func newDownloadReadsCmd() *cobra.Command {
	var (
		outputDir         string
		prefix            string
		enaMeta           bool
		methods           string
		attempts          int
		srachaPath        string
		srachaThreads     int
		srachaConnections int
		delayMin          time.Duration
		delayMax          time.Duration
		stallTimeout      time.Duration
		verbose           bool
	)
	delayMin = readdl.DefaultRetryDelayMin
	delayMax = readdl.DefaultRetryDelayMax
	stallTimeout = readdl.DefaultDownloadStallTimeout
	srachaThreads = readdl.DefaultSrachaThreads
	srachaConnections = readdl.DefaultSrachaConnections
	cmd := &cobra.Command{
		Use:   "download-reads RUN_ACCESSION",
		Short: "Download read FASTQ files for a run accession",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedMethods, err := readdl.ParseMethods(methods)
			if err != nil {
				return err
			}
			if attempts <= 0 {
				return fmt.Errorf("--attempts must be greater than zero")
			}
			if delayMin < 0 || delayMax < 0 {
				return fmt.Errorf("--retry-delay-min and --retry-delay-max must not be negative")
			}
			if delayMax < delayMin {
				return fmt.Errorf("--retry-delay-max must be greater than or equal to --retry-delay-min")
			}
			if stallTimeout < 0 {
				return fmt.Errorf("--download-stall-timeout must not be negative")
			}
			if srachaThreads <= 0 {
				return fmt.Errorf("--sracha-threads must be greater than zero")
			}
			if srachaConnections <= 0 {
				return fmt.Errorf("--sracha-connections must be greater than zero")
			}
			opts := readdl.DownloadOptions{
				OutputDir:            outputDir,
				OutputPrefix:         prefix,
				WriteMetadata:        enaMeta,
				Methods:              parsedMethods,
				Attempts:             attempts,
				SrachaPath:           srachaPath,
				SrachaThreads:        srachaThreads,
				SrachaConnections:    srachaConnections,
				RetryDelayMin:        delayMin,
				RetryDelayMax:        delayMax,
				DownloadStallTimeout: stallTimeout,
			}
			if verbose {
				opts.ProgressWriter = cmd.ErrOrStderr()
			}
			_, err = downloadReads(cmd.Context(), args[0], opts)
			return err
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Directory where FASTQ files are written")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Output FASTQ filename prefix; default uses ENA filenames")
	cmd.Flags().BoolVar(&enaMeta, "ena-meta", false, "Write ENA read metadata JSON alongside FASTQ files")
	cmd.Flags().StringVar(&methods, "method", string(readdl.MethodENA), "Download method(s), comma-separated: ena, sracha")
	cmd.Flags().IntVar(&attempts, "attempts", readdl.DefaultAttempts, "Download attempts per method")
	cmd.Flags().StringVar(&srachaPath, "sracha-bin", "", "Path to sracha; defaults to searching PATH")
	cmd.Flags().IntVarP(&srachaThreads, "sracha-threads", "t", srachaThreads, "Threads to pass to sracha -t")
	cmd.Flags().IntVar(&srachaConnections, "sracha-connections", srachaConnections, "Connections to pass to sracha --connections")
	cmd.Flags().DurationVar(&delayMin, "retry-delay-min", delayMin, "Minimum delay between failed download attempts")
	cmd.Flags().DurationVar(&delayMax, "retry-delay-max", delayMax, "Maximum delay between failed download attempts")
	cmd.Flags().DurationVar(&stallTimeout, "download-stall-timeout", stallTimeout, "Abort direct ENA downloads if no bytes arrive for this duration")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Report download progress to stderr")
	return cmd
}
