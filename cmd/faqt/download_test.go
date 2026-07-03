package main

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/martinghunt/faqt/genomedl"
	"github.com/martinghunt/faqt/seqdl"
	"github.com/spf13/cobra"
)

func TestDownloadCommandExists(t *testing.T) {
	cmd := newRootCmd()
	found, _, err := cmd.Find([]string{"download"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found == nil || found.Name() != "download" {
		t.Fatalf("unexpected command = %v", found)
	}
	for _, name := range []string{
		"output",
		"wrap",
		"compress",
		"format",
		"db",
		"nucleotide",
		"source",
		"assembly",
		"api-key",
		"email",
	} {
		if found.Flags().Lookup(name) == nil {
			t.Fatalf("download command missing --%s flag", name)
		}
	}
	if commandExists(cmd, "download-genome") {
		t.Fatal("download-genome should not be a root command")
	}
	if commandExists(cmd, "download-seq") {
		t.Fatal("download-seq should not be a root command")
	}
}

func TestDownloadCommandRoutesAssemblyToGenomeDownloader(t *testing.T) {
	oldGenome := downloadGenomeWithOptions
	oldSeq := downloadSeqAccessions
	defer func() {
		downloadGenomeWithOptions = oldGenome
		downloadSeqAccessions = oldSeq
	}()

	var (
		gotAcc     string
		gotOutPath string
		gotOptions genomedl.DownloadOptions
	)
	downloadGenomeWithOptions = func(accession, outPath string, opts genomedl.DownloadOptions) (string, error) {
		gotAcc = accession
		gotOutPath = outPath
		gotOptions = opts
		return outPath, nil
	}
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		t.Fatalf("sequence downloader should not be called")
		return nil
	}

	outPath := filepath.Join(t.TempDir(), "genome.fa")
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"GCF_000191525.1", "-o", outPath, "--format", "fasta", "--wrap", "37"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotAcc != "GCF_000191525.1" {
		t.Fatalf("accession = %q, want GCF_000191525.1", gotAcc)
	}
	if gotOutPath != outPath {
		t.Fatalf("outPath = %q, want %q", gotOutPath, outPath)
	}
	if gotOptions.Format != genomedl.GenomeFormatFASTA {
		t.Fatalf("format = %q, want %q", gotOptions.Format, genomedl.GenomeFormatFASTA)
	}
	if gotOptions.Wrap != 37 {
		t.Fatalf("wrap = %d, want 37", gotOptions.Wrap)
	}
	if gotOptions.WarningWriter == nil {
		t.Fatal("WarningWriter was not set")
	}
}

func TestDownloadCommandDeprecatedFastaAliasRoutesToGenomeDownloader(t *testing.T) {
	oldGenome := downloadGenomeWithOptions
	oldSeq := downloadSeqAccessions
	defer func() {
		downloadGenomeWithOptions = oldGenome
		downloadSeqAccessions = oldSeq
	}()

	var gotOptions genomedl.DownloadOptions
	downloadGenomeWithOptions = func(accession, outPath string, opts genomedl.DownloadOptions) (string, error) {
		gotOptions = opts
		return outPath, nil
	}
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		t.Fatalf("sequence downloader should not be called")
		return nil
	}

	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"GCF_000191525.1", "--fasta"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotOptions.Format != genomedl.GenomeFormatFASTA {
		t.Fatalf("format = %q, want %q", gotOptions.Format, genomedl.GenomeFormatFASTA)
	}
}

func TestDownloadCommandRoutesGenomeFormatToDownloader(t *testing.T) {
	tests := []struct {
		name string
		arg  string
		want genomedl.GenomeFormat
	}{
		{name: "gff3", arg: "gff3", want: genomedl.GenomeFormatGFF3},
		{name: "genbank", arg: "genbank", want: genomedl.GenomeFormatGenBank},
		{name: "gbk alias", arg: "gbk", want: genomedl.GenomeFormatGenBank},
		{name: "gbff alias", arg: "gbff", want: genomedl.GenomeFormatGenBank},
		{name: "embl", arg: "embl", want: genomedl.GenomeFormatEMBL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldGenome := downloadGenomeWithOptions
			oldSeq := downloadSeqAccessions
			defer func() {
				downloadGenomeWithOptions = oldGenome
				downloadSeqAccessions = oldSeq
			}()

			var gotOptions genomedl.DownloadOptions
			downloadGenomeWithOptions = func(accession, outPath string, opts genomedl.DownloadOptions) (string, error) {
				gotOptions = opts
				return outPath, nil
			}
			downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
				t.Fatalf("sequence downloader should not be called")
				return nil
			}

			cmd := newDownloadCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs([]string{"GCF_000191525.1", "--format", tt.arg})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if gotOptions.Format != tt.want {
				t.Fatalf("format = %q, want %q", gotOptions.Format, tt.want)
			}
		})
	}
}

func TestDownloadCommandRoutesSequenceToSeqDownloader(t *testing.T) {
	oldGenome := downloadGenomeWithOptions
	oldSeq := downloadSeqAccessions
	defer func() {
		downloadGenomeWithOptions = oldGenome
		downloadSeqAccessions = oldSeq
	}()

	var (
		gotAccessions []string
		gotOutPath    string
		gotOptions    seqdl.DownloadOptions
	)
	downloadGenomeWithOptions = func(accession, outPath string, opts genomedl.DownloadOptions) (string, error) {
		t.Fatalf("genome downloader should not be called")
		return "", nil
	}
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		gotAccessions = append([]string(nil), accessions...)
		gotOutPath = outPath
		gotOptions = opts
		return nil
	}

	outPath := filepath.Join(t.TempDir(), "wp.fa")
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"WP_002248791.1",
		"-o", outPath,
		"--db", "protein",
		"--nucleotide=all",
		"--source", "all",
		"--assembly", "GCF_000191525.1",
		"--api-key", "key123",
		"--email", "user@example.org",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotAccessions, []string{"WP_002248791.1"}) {
		t.Fatalf("accessions = %v, want WP_002248791.1", gotAccessions)
	}
	if gotOutPath != outPath {
		t.Fatalf("outPath = %q, want %q", gotOutPath, outPath)
	}
	if gotOptions.Database != seqdl.DatabaseProtein {
		t.Fatalf("database = %q, want %q", gotOptions.Database, seqdl.DatabaseProtein)
	}
	if gotOptions.Nucleotide != seqdl.NucleotideAll {
		t.Fatalf("nucleotide = %q, want %q", gotOptions.Nucleotide, seqdl.NucleotideAll)
	}
	if gotOptions.Source != seqdl.SourceAll {
		t.Fatalf("source = %q, want %q", gotOptions.Source, seqdl.SourceAll)
	}
	if gotOptions.Assembly != "GCF_000191525.1" {
		t.Fatalf("assembly = %q, want GCF_000191525.1", gotOptions.Assembly)
	}
	if gotOptions.APIKey != "key123" {
		t.Fatalf("api key = %q, want key123", gotOptions.APIKey)
	}
	if gotOptions.Email != "user@example.org" {
		t.Fatalf("email = %q, want user@example.org", gotOptions.Email)
	}
	if len(gotOptions.WriterOptions) == 0 {
		t.Fatal("writer options were not passed")
	}
}

func TestDownloadCommandRoutesWGSMasterToSeqDownloader(t *testing.T) {
	oldGenome := downloadGenomeWithOptions
	oldSeq := downloadSeqAccessions
	defer func() {
		downloadGenomeWithOptions = oldGenome
		downloadSeqAccessions = oldSeq
	}()

	var gotAccessions []string
	downloadGenomeWithOptions = func(accession, outPath string, opts genomedl.DownloadOptions) (string, error) {
		t.Fatalf("genome downloader should not be called")
		return "", nil
	}
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		gotAccessions = append([]string(nil), accessions...)
		return nil
	}

	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"JABRPF000000000.1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotAccessions, []string{"JABRPF000000000.1"}) {
		t.Fatalf("accessions = %v, want JABRPF000000000.1", gotAccessions)
	}
}

func TestDownloadCommandBareNucleotideMeansFirst(t *testing.T) {
	oldSeq := downloadSeqAccessions
	defer func() { downloadSeqAccessions = oldSeq }()

	var gotOptions seqdl.DownloadOptions
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		gotOptions = opts
		return nil
	}

	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"WP_002248791.1", "--nucleotide"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotOptions.Nucleotide != seqdl.NucleotideFirst {
		t.Fatalf("nucleotide = %q, want %q", gotOptions.Nucleotide, seqdl.NucleotideFirst)
	}
	if gotOptions.Source != seqdl.SourceRefSeq {
		t.Fatalf("source = %q, want %q", gotOptions.Source, seqdl.SourceRefSeq)
	}
}

func TestDownloadCommandUsesNCBIEnvironmentForSequences(t *testing.T) {
	oldSeq := downloadSeqAccessions
	defer func() { downloadSeqAccessions = oldSeq }()
	t.Setenv("NCBI_API_KEY", "envkey")
	t.Setenv("NCBI_EMAIL", "env@example.org")

	var gotOptions seqdl.DownloadOptions
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		gotOptions = opts
		return nil
	}

	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"WP_002248791.1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if gotOptions.APIKey != "envkey" {
		t.Fatalf("api key = %q, want envkey", gotOptions.APIKey)
	}
	if gotOptions.Email != "env@example.org" {
		t.Fatalf("email = %q, want env@example.org", gotOptions.Email)
	}
}

func TestDownloadCommandRejectsMixedGenomeAndSequenceAccessions(t *testing.T) {
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"WP_002248791.1", "GCF_000191525.1"})

	err := cmd.Execute()
	if err == nil || err.Error() != "mixed genome assembly and sequence accessions are not supported" {
		t.Fatalf("Execute() error = %v, want mixed accession error", err)
	}
}

func TestDownloadCommandRejectsMultipleGenomeAccessions(t *testing.T) {
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"GCF_000191525.1", "GCF_000001405.40"})

	err := cmd.Execute()
	if err == nil || err.Error() != "genome downloads accept exactly one accession" {
		t.Fatalf("Execute() error = %v, want single genome accession error", err)
	}
}

func TestDownloadCommandRejectsSequenceOnlyFlagsForGenome(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "db",
			args: []string{"GCF_000191525.1", "--db", "protein"},
			want: "--db is only valid for sequence downloads",
		},
		{
			name: "nucleotide",
			args: []string{"GCF_000191525.1", "--nucleotide"},
			want: "--nucleotide is only valid for sequence downloads",
		},
		{
			name: "source",
			args: []string{"GCF_000191525.1", "--source", "all"},
			want: "--source is only valid with --nucleotide",
		},
		{
			name: "assembly",
			args: []string{"GCF_000191525.1", "--assembly", "GCF_1"},
			want: "--assembly is only valid with --nucleotide",
		},
		{
			name: "compress",
			args: []string{"GCF_000191525.1", "--compress", "gzip"},
			want: "--compress is not supported for genome downloads; use an output compression suffix",
		},
		{
			name: "api",
			args: []string{"GCF_000191525.1", "--api-key", "key123"},
			want: "--api-key and --email are only valid for sequence downloads",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newDownloadCmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Execute() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDownloadCommandReturnsDownloadError(t *testing.T) {
	oldSeq := downloadSeqAccessions
	defer func() { downloadSeqAccessions = oldSeq }()
	wantErr := errors.New("download failed")
	downloadSeqAccessions = func(accessions []string, outPath string, opts seqdl.DownloadOptions) error {
		return wantErr
	}

	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"WP_002248791.1"})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}

func TestDownloadCommandRejectsUnknownGenomeFormat(t *testing.T) {
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"GCF_000191525.1",
		"-o", filepath.Join(t.TempDir(), "genome.fa"),
		"--format", "bad",
	})

	err := cmd.Execute()
	if err == nil || err.Error() != `unsupported genome format "bad"; allowed values: auto, fasta, gff3, genbank, gb, gbk, gbff, embl` {
		t.Fatalf("Execute() error = %v, want unsupported format error", err)
	}
}

func TestDownloadCommandRejectsAnnotationFormatForSequenceDownload(t *testing.T) {
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"WP_002248791.1", "--format", "gbk"})

	err := cmd.Execute()
	if err == nil || err.Error() != "--format genbank is only valid for genome downloads" {
		t.Fatalf("Execute() error = %v, want genome-only format error", err)
	}
}

func TestDownloadCommandRejectsConflictingFastaAliasAndFormat(t *testing.T) {
	cmd := newDownloadCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"GCF_000191525.1", "--format", "gff3", "--fasta"})

	err := cmd.Execute()
	if err == nil || err.Error() != "--fasta cannot be used with --format gff3" {
		t.Fatalf("Execute() error = %v, want conflicting format error", err)
	}
}

func commandExists(cmd interface{ Commands() []*cobra.Command }, name string) bool {
	for _, child := range cmd.Commands() {
		if child.Name() == name {
			return true
		}
	}
	return false
}
