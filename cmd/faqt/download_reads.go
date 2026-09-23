package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/martinghunt/faqt/readdl"
	"github.com/spf13/cobra"
)

var downloadReadRuns = readdl.DownloadRuns

func newDownloadReadsCmd() *cobra.Command {
	var (
		outputDir         string
		prefix            string
		accessionsFile    string
		enaMeta           bool
		enaMetaSingle     bool
		methods           string
		attempts          int
		srachaPath        string
		srachaThreads     int
		srachaConnections int
		delayMin          time.Duration
		delayMax          time.Duration
		stallTimeout      time.Duration
		merge             bool
		keepOriginals     bool
		verbose           bool
	)
	delayMin = readdl.DefaultRetryDelayMin
	delayMax = readdl.DefaultRetryDelayMax
	stallTimeout = readdl.DefaultDownloadStallTimeout
	srachaThreads = readdl.DefaultSrachaThreads
	srachaConnections = readdl.DefaultSrachaConnections
	cmd := &cobra.Command{
		Use:   "download-reads [RUN_ACCESSION[,RUN_ACCESSION...]]",
		Short: "Download read FASTQ files for run accessions",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			runArg := ""
			if len(args) > 0 {
				runArg = args[0]
			}
			runAccessions, err := downloadReadRunAccessions(runArg, accessionsFile, cmd.InOrStdin())
			if err != nil {
				return err
			}
			parsedMethods, err := readdl.ParseMethods(methods)
			if err != nil {
				return err
			}
			opts := readdl.DownloadRunsOptions{
				Download: readdl.DownloadOptions{
					OutputDir:            outputDir,
					OutputPrefix:         prefix,
					WriteMetadata:        enaMeta,
					MetadataSingleObject: enaMetaSingle && !merge,
					Methods:              parsedMethods,
					Attempts:             attempts,
					SrachaPath:           srachaPath,
					SrachaThreads:        srachaThreads,
					SrachaConnections:    srachaConnections,
					RetryDelayMin:        delayMin,
					RetryDelayMax:        delayMax,
					DownloadStallTimeout: stallTimeout,
				},
				Merge:         merge,
				KeepOriginals: keepOriginals,
			}
			if verbose {
				opts.Download.ProgressWriter = cmd.ErrOrStderr()
			}
			_, err = downloadReadRuns(cmd.Context(), runAccessions, opts)
			return err
		},
	}
	cmd.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "Directory where FASTQ files are written")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Output FASTQ filename prefix; default uses ENA filenames")
	cmd.Flags().StringVar(&accessionsFile, "accessions-file", "", "File containing run accessions, one per line; use - for stdin")
	cmd.Flags().BoolVar(&enaMeta, "ena-meta", false, "Write ENA read metadata JSON alongside FASTQ files")
	cmd.Flags().BoolVar(&enaMetaSingle, "ena-meta-single-object", false, "Write each non-merged single-run metadata file as a JSON object instead of a list")
	cmd.Flags().StringVar(&methods, "method", string(readdl.MethodENA), "Download method(s), comma-separated: ena, sracha")
	cmd.Flags().IntVar(&attempts, "attempts", readdl.DefaultAttempts, "Download attempts per method")
	cmd.Flags().StringVar(&srachaPath, "sracha-bin", "", "Path to sracha; defaults to searching PATH")
	cmd.Flags().IntVarP(&srachaThreads, "sracha-threads", "t", srachaThreads, "Threads to pass to sracha -t")
	cmd.Flags().IntVar(&srachaConnections, "sracha-connections", srachaConnections, "Connections to pass to sracha --connections")
	cmd.Flags().DurationVar(&delayMin, "retry-delay-min", delayMin, "Minimum delay between failed download attempts")
	cmd.Flags().DurationVar(&delayMax, "retry-delay-max", delayMax, "Maximum delay between failed download attempts")
	cmd.Flags().DurationVar(&stallTimeout, "download-stall-timeout", stallTimeout, "Abort direct ENA downloads if no bytes arrive for this duration")
	cmd.Flags().BoolVar(&merge, "merge", false, "Merge FASTQ files from multiple runs into one file per read direction")
	cmd.Flags().BoolVar(&keepOriginals, "keep-originals", false, "Keep per-run FASTQ files after merging")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Report download progress to stderr")
	return cmd
}

func downloadReadRunAccessions(runArg, accessionsFile string, stdin io.Reader) ([]string, error) {
	runArg = strings.TrimSpace(runArg)
	accessionsFile = strings.TrimSpace(accessionsFile)
	if runArg == "" && accessionsFile == "" {
		return nil, fmt.Errorf("download-reads requires a run accession or --accessions-file")
	}
	if runArg != "" && accessionsFile != "" {
		return nil, fmt.Errorf("download-reads accepts either run accessions or --accessions-file, not both")
	}
	if accessionsFile != "" {
		return readDownloadReadAccessionsFile(accessionsFile, stdin)
	}
	return splitDownloadReadAccessions(runArg)
}

func splitDownloadReadAccessions(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	accessions := make([]string, 0, len(parts))
	for _, part := range parts {
		accession := strings.TrimSpace(part)
		if accession == "" {
			return nil, fmt.Errorf("download-reads accession list contains an empty run accession")
		}
		accessions = append(accessions, accession)
	}
	return accessions, nil
}

func readDownloadReadAccessionsFile(path string, stdin io.Reader) ([]string, error) {
	var (
		r     io.Reader
		close func() error
	)
	if path == "-" {
		r = stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("open accessions file %q: %w", path, err)
		}
		r = f
		close = f.Close
	}
	if close != nil {
		defer close()
	}

	scanner := bufio.NewScanner(r)
	var accessions []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		accessions = append(accessions, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read accessions file %q: %w", path, err)
	}
	if len(accessions) == 0 {
		return nil, fmt.Errorf("download-reads accession file contains no run accessions")
	}
	return accessions, nil
}
