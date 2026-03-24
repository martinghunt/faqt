package main

import (
	"fmt"
	"io"
	"os"

	"github.com/martinghunt/faqt/perfectreads"
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newToPerfectReadsCmd() *cobra.Command {
	var (
		inputPath  string
		outputPath string
		outputFwd  string
		outputRev  string
		meanInsert int
		insertStd  float64
		coverage   float64
		readLength int
		noN        bool
		seed       int64
	)
	cmd := &cobra.Command{
		Use:   "to-perfect-reads INPUT",
		Short: "Make perfect FASTQ reads from a reference sequence file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			inputPath = args[0]
			reader, err := seqio.OpenPath(inputPath)
			if err != nil {
				return err
			}
			if closer, ok := reader.(io.Closer); ok {
				defer closer.Close()
			}
			opts := perfectreads.Options{
				MeanInsert: meanInsert,
				InsertStd:  insertStd,
				Coverage:   coverage,
				ReadLength: readLength,
				NoN:        noN,
				Seed:       seed,
			}
			report, err := runToPerfectReads(reader, outputPath, outputFwd, outputRev, opts)
			if err != nil {
				return err
			}
			for _, name := range report.SkippedShort {
				_, _ = fmt.Fprintf(os.Stderr, "Warning, sequence %s too short. Skipping it...\n", name)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&outputPath, "out", "o", "", "Output FASTQ file for single-end reads")
	cmd.Flags().StringVarP(&outputFwd, "forward-out", "1", "", "Output FASTQ file for forward reads")
	cmd.Flags().StringVarP(&outputRev, "reverse-out", "2", "", "Output FASTQ file for reverse reads")
	cmd.Flags().IntVar(&meanInsert, "mean-insert", 0, "Mean insert size of read pairs")
	cmd.Flags().Float64Var(&insertStd, "insert-std", 0, "Standard deviation of insert size")
	cmd.Flags().Float64Var(&coverage, "coverage", 0, "Mean coverage of the reads")
	cmd.Flags().IntVar(&readLength, "read-length", 0, "Length of each read")
	cmd.Flags().BoolVar(&noN, "no-n", false, "Do not allow any N or n characters in the reads")
	cmd.Flags().Int64Var(&seed, "seed", 1, "Random seed")
	return cmd
}

func runToPerfectReads(reader seqio.Reader, outputPath, outputFwd, outputRev string, opts perfectreads.Options) (perfectreads.Report, error) {
	switch {
	case outputPath != "":
		if outputFwd != "" || outputRev != "" {
			return perfectreads.Report{}, fmt.Errorf("use either --out for single-end reads or --forward-out/--reverse-out for paired reads")
		}
		w, err := seqio.CreateFASTQPath(outputPath)
		if err != nil {
			return perfectreads.Report{}, err
		}
		defer w.Close()
		return perfectreads.GenerateSingle(reader, w, opts)
	case outputFwd != "" || outputRev != "":
		if outputFwd == "" || outputRev == "" {
			return perfectreads.Report{}, fmt.Errorf("paired reads require both --forward-out and --reverse-out")
		}
		if opts.MeanInsert <= 0 {
			return perfectreads.Report{}, fmt.Errorf("mean insert must be > 0")
		}
		fw, err := seqio.CreateFASTQPath(outputFwd)
		if err != nil {
			return perfectreads.Report{}, err
		}
		defer fw.Close()
		rw, err := seqio.CreateFASTQPath(outputRev)
		if err != nil {
			return perfectreads.Report{}, err
		}
		defer rw.Close()
		return perfectreads.GeneratePaired(reader, fw, rw, opts)
	default:
		return perfectreads.Report{}, fmt.Errorf("must provide either --out or both --forward-out and --reverse-out")
	}
}
