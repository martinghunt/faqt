package main

import (
	"fmt"
	"strconv"

	"github.com/martinghunt/faqt/randomcontigs"
	"github.com/martinghunt/faqt/seqio"
	"github.com/spf13/cobra"
)

func newMakeRandomContigsCmd() *cobra.Command {
	var (
		outputPath    string
		firstNumber   int
		nameByLetters bool
		prefix        string
		seed          int64
		seedSet       bool
		wrap          int
		compress      string
	)
	cmd := &cobra.Command{
		Use:   "make-random-contigs [options] CONTIGS LENGTH",
		Short: "Make contigs of random sequence",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			contigs, err := parseNonNegativeInt(args[0], "contigs")
			if err != nil {
				return err
			}
			length, err := parseNonNegativeInt(args[1], "length")
			if err != nil {
				return err
			}

			opts := randomcontigs.Options{
				Contigs:       contigs,
				Length:        length,
				NameByLetters: nameByLetters,
				Prefix:        prefix,
				FirstNumber:   firstNumber,
			}
			if seedSet {
				opts.Seed = &seed
			}
			return randomcontigs.GenerateToPath(
				outputPath,
				opts,
				seqio.WithWrap(wrap),
				seqio.WithCompression(seqio.Compression(compress)),
			)
		},
	}
	cmd.Flags().StringVarP(&outputPath, "output", "o", "-", "Output path or - for stdout")
	cmd.Flags().IntVar(&firstNumber, "first-number", 1, "If numbering the sequences, the first sequence gets this number")
	cmd.Flags().BoolVar(&nameByLetters, "name-by-letters", false, "Name the contigs A,B,C,... and restart at A after Z")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Prefix to add to start of every sequence name")
	cmd.Flags().Int64Var(&seed, "seed", 0, "Seed for random number generator")
	cmd.Flags().IntVar(&wrap, "wrap", 0, "FASTA wrap width; 0 disables wrapping")
	cmd.Flags().StringVar(&compress, "compress", string(seqio.CompressAuto), "Output compression: auto, none, gzip, bzip2, xz, zstd")
	cmd.PreRun = func(cmd *cobra.Command, args []string) {
		seedSet = cmd.Flags().Changed("seed")
	}
	return cmd
}

func parseNonNegativeInt(s, name string) (int, error) {
	value, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be >= 0", name)
	}
	return value, nil
}
