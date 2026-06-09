package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/martinghunt/faqt/genomedl"
	"github.com/martinghunt/faqt/seqdl"
	"github.com/spf13/cobra"
)

var (
	downloadGenomeWithOptions = genomedl.DownloadGenomeWithOptions
	downloadSeqAccessions     = seqdl.DownloadAccessions
)

func newDownloadCmd() *cobra.Command {
	var (
		output   sequenceOutputOptions
		fasta    bool
		db       string
		nuc      string
		source   string
		assembly string
		apiKey   string
		email    string
	)
	cmd := &cobra.Command{
		Use:   "download ACCESSION...",
		Short: "Download genome or sequence data by accession",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if isGenomeDownload(args) {
				if err := validateGenomeDownloadArgs(args, output.compress, db, nuc, source, assembly, apiKey, email); err != nil {
					return err
				}
				_, err := downloadGenomeWithOptions(args[0], output.path, genomedl.DownloadOptions{
					FastaOnly:     fasta,
					WarningWriter: cmd.ErrOrStderr(),
				})
				return err
			}
			if err := validateSequenceDownloadArgs(args); err != nil {
				return err
			}
			if apiKey == "" {
				apiKey = os.Getenv("NCBI_API_KEY")
			}
			if email == "" {
				email = os.Getenv("NCBI_EMAIL")
			}
			return downloadSeqAccessions(args, output.path, seqdl.DownloadOptions{
				Database:      seqdl.Database(db),
				Nucleotide:    seqdl.NucleotideMode(nuc),
				Source:        seqdl.Source(source),
				Assembly:      assembly,
				APIKey:        apiKey,
				Email:         email,
				WriterOptions: output.seqioOptions(),
			})
		},
	}
	addSequenceOutputFlags(cmd, &output)
	cmd.Flags().BoolVar(&fasta, "fasta", false, "For genome accessions, write genomic FASTA only")
	cmd.Flags().StringVar(&db, "db", string(seqdl.DatabaseAuto), "NCBI sequence database: auto, protein, nuccore, nucleotide, sequences")
	cmd.Flags().StringVar(&nuc, "nucleotide", string(seqdl.NucleotideNone), "Download nucleotide CDS linked from protein accessions: first or all")
	cmd.Flags().StringVar(&source, "source", string(seqdl.SourceRefSeq), "Nucleotide CDS source: refseq, insdc, all")
	cmd.Flags().StringVar(&assembly, "assembly", "", "Nucleotide CDS assembly accession filter")
	cmd.Flags().StringVar(&apiKey, "api-key", "", "NCBI API key; defaults to NCBI_API_KEY")
	cmd.Flags().StringVar(&email, "email", "", "Email sent to NCBI; defaults to NCBI_EMAIL")
	cmd.Flags().Lookup("nucleotide").NoOptDefVal = string(seqdl.NucleotideFirst)
	return cmd
}

func isGenomeDownload(accessions []string) bool {
	return len(accessions) > 0 && isGenomeAssemblyAccession(accessions[0])
}

func validateGenomeDownloadArgs(accessions []string, compress, db, nuc, source, assembly, apiKey, email string) error {
	if len(accessions) != 1 {
		return fmt.Errorf("genome downloads accept exactly one accession")
	}
	if compress != "auto" {
		return fmt.Errorf("--compress is not supported for genome downloads; use an output compression suffix")
	}
	if db != string(seqdl.DatabaseAuto) {
		return fmt.Errorf("--db is only valid for sequence downloads")
	}
	if nuc != string(seqdl.NucleotideNone) {
		return fmt.Errorf("--nucleotide is only valid for sequence downloads")
	}
	if source != string(seqdl.SourceRefSeq) {
		return fmt.Errorf("--source is only valid with --nucleotide")
	}
	if assembly != "" {
		return fmt.Errorf("--assembly is only valid with --nucleotide")
	}
	if apiKey != "" || email != "" {
		return fmt.Errorf("--api-key and --email are only valid for sequence downloads")
	}
	return nil
}

func validateSequenceDownloadArgs(accessions []string) error {
	for _, accession := range accessions {
		if isGenomeAssemblyAccession(accession) {
			return fmt.Errorf("mixed genome assembly and sequence accessions are not supported")
		}
	}
	return nil
}

func isGenomeAssemblyAccession(accession string) bool {
	upper := strings.ToUpper(strings.TrimSpace(accession))
	return strings.HasPrefix(upper, "GCF_") || strings.HasPrefix(upper, "GCA_")
}
