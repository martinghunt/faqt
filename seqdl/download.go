package seqdl

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/martinghunt/faqt/seqio"
)

const (
	defaultEFetchURL   = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"
	defaultHTTPTimeout = 120 * time.Second
	defaultToolName    = "faqt"
)

// Database identifies an NCBI EFetch sequence database.
type Database string

const (
	DatabaseAuto       Database = "auto"
	DatabaseProtein    Database = "protein"
	DatabaseNuccore    Database = "nuccore"
	DatabaseNucleotide Database = "nucleotide"
	DatabaseSequences  Database = "sequences"
)

// NucleotideMode selects nucleotide CDS output for protein accessions.
type NucleotideMode string

const (
	NucleotideNone  NucleotideMode = ""
	NucleotideFirst NucleotideMode = "first"
	NucleotideAll   NucleotideMode = "all"
)

// Source selects which IPG nucleotide CDS rows are eligible.
type Source string

const (
	SourceRefSeq Source = "refseq"
	SourceINSDC  Source = "insdc"
	SourceAll    Source = "all"
)

// DownloadOptions controls sequence accession downloads.
type DownloadOptions struct {
	// Database selects the NCBI database. DatabaseAuto infers protein or
	// nucleotide for common accession prefixes, and otherwise uses sequences.
	Database Database
	// Nucleotide selects CDS nucleotide output linked from protein accessions.
	// Empty means download the accession's own FASTA sequence.
	Nucleotide NucleotideMode
	// Source filters IPG rows for nucleotide output. Empty defaults to RefSeq.
	Source Source
	// Assembly filters IPG rows to one assembly accession for nucleotide output.
	Assembly string
	// APIKey is passed to NCBI as api_key when non-empty.
	APIKey string
	// Email is passed to NCBI as email when non-empty.
	Email string
	// WriterOptions are passed to the FASTA writer.
	WriterOptions []seqio.Option
}

// Downloader downloads sequence FASTA from NCBI EFetch.
type Downloader struct {
	// EFetchURL is the base EFetch endpoint URL.
	EFetchURL string
	// Tool is sent to NCBI as the tool parameter.
	Tool string
	// HTTPClient is used for download requests.
	HTTPClient *http.Client
}

// NewDownloader returns a downloader configured with the default NCBI endpoint.
func NewDownloader() *Downloader {
	return &Downloader{
		EFetchURL:  defaultEFetchURL,
		Tool:       defaultToolName,
		HTTPClient: &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// DownloadAccessions downloads one or more sequence accessions to FASTA.
func DownloadAccessions(accessions []string, outPath string, opts DownloadOptions) error {
	return NewDownloader().DownloadAccessions(accessions, outPath, opts)
}

// DownloadAccession downloads one sequence accession to FASTA.
func DownloadAccession(accession, outPath string, opts DownloadOptions) error {
	return NewDownloader().DownloadAccessions([]string{accession}, outPath, opts)
}

// DownloadAccessions downloads one or more sequence accessions to FASTA.
func (d *Downloader) DownloadAccessions(accessions []string, outPath string, opts DownloadOptions) (err error) {
	ids, err := cleanAccessions(accessions)
	if err != nil {
		return err
	}
	if strings.TrimSpace(outPath) == "" {
		return fmt.Errorf("empty output path")
	}
	mode, err := normalizeNucleotideMode(opts.Nucleotide)
	if err != nil {
		return err
	}
	if mode != NucleotideNone {
		return d.downloadNucleotideAccessions(ids, outPath, opts, mode)
	}
	db, err := resolveDatabase(ids, opts.Database)
	if err != nil {
		return err
	}
	req, err := d.newEFetchRequest(ids, db, opts)
	if err != nil {
		return err
	}
	resp, err := d.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	reader, err := seqio.OpenReader(bufio.NewReader(resp.Body))
	if err != nil {
		return fmt.Errorf("downloaded data is not supported sequence content: %w", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}

	writer, err := seqio.CreateFASTAPath(outPath, opts.WriterOptions...)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, writer)

	wrote := false
	for {
		rec, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
		if err := writer.Write(rec); err != nil {
			return err
		}
		wrote = true
	}
	if !wrote {
		return fmt.Errorf("download produced no FASTA records")
	}
	return nil
}

func (d *Downloader) downloadNucleotideAccessions(accessions []string, outPath string, opts DownloadOptions, mode NucleotideMode) (err error) {
	source, err := normalizeSource(opts.Source)
	if err != nil {
		return err
	}
	assembly := strings.TrimSpace(opts.Assembly)

	rows := make([]ipgRow, 0, len(accessions))
	for _, accession := range accessions {
		matches, err := d.nucleotideRowsForAccession(accession, opts, source, assembly)
		if err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no nucleotide CDS rows for %s match source %s and assembly %q", accession, source, assembly)
		}
		if mode == NucleotideFirst {
			matches = matches[:1]
		}
		rows = append(rows, matches...)
	}

	writer, err := seqio.CreateFASTAPath(outPath, opts.WriterOptions...)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, writer)

	for _, row := range rows {
		if err := d.downloadNucleotideRow(row, writer, opts); err != nil {
			return err
		}
	}
	return nil
}

func (d *Downloader) nucleotideRowsForAccession(accession string, opts DownloadOptions, source Source, assembly string) ([]ipgRow, error) {
	req, err := d.newIPGRequest(accession, opts)
	if err != nil {
		return nil, err
	}
	resp, err := d.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download IPG failed: %s", resp.Status)
	}
	rows, err := parseIPGRows(resp.Body)
	if err != nil {
		return nil, err
	}
	return filterIPGRows(rows, source, assembly), nil
}

func (d *Downloader) downloadNucleotideRow(row ipgRow, writer *seqio.Writer, opts DownloadOptions) (err error) {
	req, err := d.newNucleotideRegionRequest(row, opts)
	if err != nil {
		return err
	}
	resp, err := d.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download nucleotide CDS failed: %s", resp.Status)
	}

	reader, err := seqio.OpenReader(bufio.NewReader(resp.Body))
	if err != nil {
		return fmt.Errorf("downloaded nucleotide CDS data is not supported sequence content: %w", err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}
	wrote := false
	for {
		rec, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
		if err := writer.Write(rec); err != nil {
			return err
		}
		wrote = true
	}
	if !wrote {
		return fmt.Errorf("download produced no nucleotide CDS FASTA records for %s", row.NucleotideAccession)
	}
	return nil
}

func cleanAccessions(accessions []string) ([]string, error) {
	ids := make([]string, 0, len(accessions))
	for _, accession := range accessions {
		acc := strings.TrimSpace(accession)
		if acc == "" {
			continue
		}
		ids = append(ids, acc)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("empty accession")
	}
	return ids, nil
}

func (d *Downloader) newEFetchRequest(accessions []string, db Database, opts DownloadOptions) (*http.Request, error) {
	values := url.Values{}
	values.Set("db", string(db))
	values.Set("id", strings.Join(accessions, ","))
	values.Set("rettype", "fasta")
	values.Set("retmode", "text")
	return d.newRequest(values, opts)
}

func (d *Downloader) newIPGRequest(accession string, opts DownloadOptions) (*http.Request, error) {
	values := url.Values{}
	values.Set("db", string(DatabaseProtein))
	values.Set("id", accession)
	values.Set("rettype", "ipg")
	values.Set("retmode", "text")
	return d.newRequest(values, opts)
}

func (d *Downloader) newNucleotideRegionRequest(row ipgRow, opts DownloadOptions) (*http.Request, error) {
	strand, err := row.efetchStrand()
	if err != nil {
		return nil, err
	}
	values := url.Values{}
	values.Set("db", string(DatabaseNuccore))
	values.Set("id", row.NucleotideAccession)
	values.Set("seq_start", row.Start)
	values.Set("seq_stop", row.Stop)
	values.Set("strand", strand)
	values.Set("rettype", "fasta")
	values.Set("retmode", "text")
	return d.newRequest(values, opts)
}

func (d *Downloader) newRequest(values url.Values, opts DownloadOptions) (*http.Request, error) {
	base, err := url.Parse(d.efetchURL())
	if err != nil {
		return nil, err
	}
	query := base.Query()
	for key, vals := range values {
		for _, value := range vals {
			query.Add(key, value)
		}
	}
	if tool := d.tool(); tool != "" {
		query.Set("tool", tool)
	}
	if opts.Email != "" {
		query.Set("email", opts.Email)
	}
	if opts.APIKey != "" {
		query.Set("api_key", opts.APIKey)
	}
	base.RawQuery = query.Encode()
	return http.NewRequest(http.MethodGet, base.String(), nil)
}

func (d *Downloader) efetchURL() string {
	if d != nil && d.EFetchURL != "" {
		return d.EFetchURL
	}
	return defaultEFetchURL
}

func (d *Downloader) tool() string {
	if d != nil && d.Tool != "" {
		return d.Tool
	}
	return defaultToolName
}

func (d *Downloader) httpClient() *http.Client {
	if d != nil && d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}

func resolveDatabase(accessions []string, requested Database) (Database, error) {
	switch normalizeDatabase(requested) {
	case "", DatabaseAuto:
		return inferDatabase(accessions), nil
	case DatabaseProtein:
		return DatabaseProtein, nil
	case DatabaseNuccore, DatabaseNucleotide:
		return DatabaseNuccore, nil
	case DatabaseSequences:
		return DatabaseSequences, nil
	default:
		return "", fmt.Errorf("unsupported database %q", requested)
	}
}

func normalizeNucleotideMode(mode NucleotideMode) (NucleotideMode, error) {
	switch NucleotideMode(strings.ToLower(strings.TrimSpace(string(mode)))) {
	case "", "none", "false":
		return NucleotideNone, nil
	case "first", "true":
		return NucleotideFirst, nil
	case "all":
		return NucleotideAll, nil
	default:
		return "", fmt.Errorf("unsupported nucleotide mode %q", mode)
	}
}

func normalizeSource(source Source) (Source, error) {
	switch Source(strings.ToLower(strings.TrimSpace(string(source)))) {
	case "", SourceRefSeq:
		return SourceRefSeq, nil
	case SourceINSDC:
		return SourceINSDC, nil
	case SourceAll:
		return SourceAll, nil
	default:
		return "", fmt.Errorf("unsupported source %q", source)
	}
}

func normalizeDatabase(db Database) Database {
	return Database(strings.ToLower(strings.TrimSpace(string(db))))
}

func inferDatabase(accessions []string) Database {
	var inferred Database
	for _, accession := range accessions {
		db := inferDatabaseOne(accession)
		if inferred == "" {
			inferred = db
			continue
		}
		if inferred != db {
			return DatabaseSequences
		}
	}
	if inferred == "" {
		return DatabaseSequences
	}
	return inferred
}

func inferDatabaseOne(accession string) Database {
	upper := strings.ToUpper(strings.TrimSpace(accession))
	switch {
	case hasAnyPrefix(upper, "WP_", "NP_", "XP_", "YP_", "AP_", "ZP_"):
		return DatabaseProtein
	case hasAnyPrefix(upper, "NC_", "NG_", "NM_", "NR_", "NT_", "NW_", "NZ_", "XM_", "XR_", "AC_", "CM_", "CP_"):
		return DatabaseNuccore
	default:
		return DatabaseSequences
	}
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

type ipgRow struct {
	Source              string
	NucleotideAccession string
	Start               string
	Stop                string
	Strand              string
	ProteinAccession    string
	ProteinName         string
	Organism            string
	Strain              string
	Assembly            string
}

func parseIPGRows(r io.Reader) ([]ipgRow, error) {
	reader := csv.NewReader(r)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("download produced empty IPG report")
	}
	rows := make([]ipgRow, 0, len(records)-1)
	for i, record := range records[1:] {
		if len(record) < 11 {
			return nil, fmt.Errorf("IPG row %d has %d fields, want at least 11", i+2, len(record))
		}
		row := ipgRow{
			Source:              strings.TrimSpace(record[1]),
			NucleotideAccession: strings.TrimSpace(record[2]),
			Start:               strings.TrimSpace(record[3]),
			Stop:                strings.TrimSpace(record[4]),
			Strand:              strings.TrimSpace(record[5]),
			ProteinAccession:    strings.TrimSpace(record[6]),
			ProteinName:         strings.TrimSpace(record[7]),
			Organism:            strings.TrimSpace(record[8]),
			Strain:              strings.TrimSpace(record[9]),
			Assembly:            strings.TrimSpace(record[10]),
		}
		if !row.hasNucleotideRegion() {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func filterIPGRows(rows []ipgRow, source Source, assembly string) []ipgRow {
	filtered := make([]ipgRow, 0, len(rows))
	for _, row := range rows {
		if !sourceMatches(row.Source, source) {
			continue
		}
		if assembly != "" && !strings.EqualFold(row.Assembly, assembly) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered
}

func sourceMatches(rowSource string, source Source) bool {
	switch source {
	case SourceAll:
		return true
	case SourceRefSeq:
		return strings.EqualFold(rowSource, "RefSeq")
	case SourceINSDC:
		return strings.EqualFold(rowSource, "INSDC")
	default:
		return false
	}
}

func (r ipgRow) hasNucleotideRegion() bool {
	return r.NucleotideAccession != "" &&
		!strings.EqualFold(r.NucleotideAccession, "N/A") &&
		r.Start != "" &&
		r.Stop != "" &&
		(r.Strand == "+" || r.Strand == "-")
}

func (r ipgRow) efetchStrand() (string, error) {
	switch r.Strand {
	case "+":
		return "1", nil
	case "-":
		return "2", nil
	default:
		return "", fmt.Errorf("unsupported IPG strand %q", r.Strand)
	}
}
