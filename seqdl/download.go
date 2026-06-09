package seqdl

import (
	"bufio"
	"encoding/csv"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/martinghunt/faqt/seqio"
)

const (
	defaultEFetchURL   = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"
	defaultHTTPTimeout = 120 * time.Second
	defaultToolName    = "faqt"
	maxEFetchIDs       = 200
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
	// nucleotide for recognized accession styles, and otherwise uses sequences.
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
	if db == DatabaseProtein && hasWGSProjectAccession(ids) {
		return fmt.Errorf("WGS/TSA/TLS master accessions are nucleotide records")
	}
	writer, err := seqio.CreateFASTAPath(outPath, opts.WriterOptions...)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, writer)

	written, err := d.downloadSequenceFASTARecords(ids, db, writer, opts)
	if err != nil {
		return err
	}
	if written == 0 {
		return fmt.Errorf("download produced no FASTA records")
	}
	return nil
}

func (d *Downloader) downloadSequenceFASTARecords(accessions []string, db Database, writer *seqio.Writer, opts DownloadOptions) (int, error) {
	written := 0
	direct := make([]string, 0, len(accessions))
	flushDirect := func() error {
		if len(direct) == 0 {
			return nil
		}
		n, err := d.downloadDirectFASTARecords(direct, db, writer, opts)
		if err != nil {
			return err
		}
		written += n
		direct = direct[:0]
		return nil
	}

	for _, accession := range accessions {
		if !isWGSProjectAccession(accession) {
			direct = append(direct, accession)
			continue
		}
		if err := flushDirect(); err != nil {
			return written, err
		}
		n, err := d.downloadWGSProjectFASTA(accession, writer, opts)
		if err != nil {
			return written, err
		}
		written += n
	}
	if err := flushDirect(); err != nil {
		return written, err
	}
	return written, nil
}

func (d *Downloader) downloadDirectFASTARecords(accessions []string, db Database, writer *seqio.Writer, opts DownloadOptions) (int, error) {
	written := 0
	for start := 0; start < len(accessions); start += maxEFetchIDs {
		end := start + maxEFetchIDs
		if end > len(accessions) {
			end = len(accessions)
		}
		req, err := d.newEFetchRequest(accessions[start:end], db, opts)
		if err != nil {
			return written, err
		}
		n, err := d.downloadFASTARequest(req, writer, "downloaded data is not supported sequence content")
		if err != nil {
			return written, err
		}
		written += n
	}
	return written, nil
}

func (d *Downloader) downloadFASTARequest(req *http.Request, writer *seqio.Writer, contentErrPrefix string) (written int, err error) {
	resp, err := d.httpClient().Do(req)
	if err != nil {
		return 0, err
	}
	defer closeutil.CloseWithError(&err, resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return 0, os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download failed: %s", resp.Status)
	}

	reader, err := seqio.OpenReader(bufio.NewReader(resp.Body))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", contentErrPrefix, err)
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}

	for {
		rec, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return written, readErr
		}
		if err := writer.Write(rec); err != nil {
			return written, err
		}
		written++
	}
	return written, nil
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

func (d *Downloader) downloadWGSProjectFASTA(accession string, writer *seqio.Writer, opts DownloadOptions) (int, error) {
	ranges, err := d.wgsComponentRanges(accession, opts)
	if err != nil {
		return 0, err
	}
	if len(ranges) == 0 {
		return 0, fmt.Errorf("WGS/TSA/TLS master %s has no component accession ranges", accession)
	}

	written := 0
	for _, r := range ranges {
		n, err := d.downloadWGSComponentRange(r, writer, opts)
		if err != nil {
			return written, err
		}
		written += n
	}
	if written == 0 {
		return 0, fmt.Errorf("download produced no WGS/TSA/TLS component FASTA records for %s", accession)
	}
	return written, nil
}

func (d *Downloader) wgsComponentRanges(accession string, opts DownloadOptions) (ranges []accessionRange, err error) {
	req, err := d.newWGSProjectRequest(accession, opts)
	if err != nil {
		return nil, err
	}
	resp, err := d.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer closeutil.CloseWithError(&err, resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, os.ErrNotExist
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download WGS/TSA/TLS master failed: %s", resp.Status)
	}

	info, err := parseWGSProjectInfo(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(info.Ranges) > 0 {
		return info.Ranges, nil
	}
	return wgsRangesFromLocusLength(info.Locus, info.Length)
}

func (d *Downloader) downloadWGSComponentRange(r accessionRange, writer *seqio.Writer, opts DownloadOptions) (int, error) {
	if r.First == r.Last {
		return d.downloadDirectFASTARecords([]string{r.First}, DatabaseNuccore, writer, opts)
	}
	first, lastNumber, err := parseAccessionRangeSerial(r)
	if err != nil {
		return 0, err
	}
	if first.Number > lastNumber {
		return 0, fmt.Errorf("component accession range %s-%s is descending", r.First, r.Last)
	}

	written := 0
	batch := make([]string, 0, maxEFetchIDs)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		n, err := d.downloadDirectFASTARecords(batch, DatabaseNuccore, writer, opts)
		if err != nil {
			return err
		}
		written += n
		batch = batch[:0]
		return nil
	}

	for n := first.Number; n <= lastNumber; n++ {
		batch = append(batch, formatAccessionSerial(first, n))
		if len(batch) == cap(batch) {
			if err := flush(); err != nil {
				return written, err
			}
		}
	}
	if err := flush(); err != nil {
		return written, err
	}
	return written, nil
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

func (d *Downloader) newWGSProjectRequest(accession string, opts DownloadOptions) (*http.Request, error) {
	values := url.Values{}
	values.Set("db", string(DatabaseNuccore))
	values.Set("id", accession)
	values.Set("rettype", "gbc")
	values.Set("retmode", "xml")
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
	core := accessionCore(upper)
	switch {
	case hasAnyPrefix(upper, "WP_", "NP_", "XP_", "YP_", "AP_", "ZP_"):
		return DatabaseProtein
	case hasAnyPrefix(upper, "NC_", "NG_", "NM_", "NR_", "NT_", "NW_", "NZ_", "XM_", "XR_", "AC_", "CM_", "CP_"):
		return DatabaseNuccore
	case isINSDCProteinAccession(core):
		return DatabaseProtein
	case isWGSProjectAccession(core), isINSDCNucleotideAccession(core), isWGSLikeNucleotideAccession(core):
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

func hasWGSProjectAccession(accessions []string) bool {
	for _, accession := range accessions {
		if isWGSProjectAccession(accession) {
			return true
		}
	}
	return false
}

func isWGSProjectAccession(accession string) bool {
	core := accessionCore(strings.ToUpper(strings.TrimSpace(accession)))
	return hasLettersThenZeroes(core, 4, 8) ||
		hasLettersThenZeroes(core, 6, 9) ||
		hasLettersThenDigits(core, 4, 2) ||
		hasLettersThenDigits(core, 6, 2)
}

func isWGSLikeNucleotideAccession(core string) bool {
	return hasLettersThenDigitsAtLeast(core, 4, 8) ||
		hasLettersThenDigitsAtLeast(core, 6, 9)
}

func isINSDCNucleotideAccession(core string) bool {
	return hasLettersThenDigits(core, 1, 5) ||
		hasLettersThenDigits(core, 2, 6) ||
		hasLettersThenDigits(core, 2, 8)
}

func isINSDCProteinAccession(core string) bool {
	return hasLettersThenDigits(core, 3, 5) ||
		hasLettersThenDigits(core, 3, 7)
}

func accessionCore(accession string) string {
	if dot := strings.LastIndexByte(accession, '.'); dot > 0 && allDigits(accession[dot+1:]) {
		return accession[:dot]
	}
	return accession
}

func hasLettersThenZeroes(s string, letters, zeroes int) bool {
	if len(s) != letters+zeroes {
		return false
	}
	for i := 0; i < letters; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	for i := letters; i < len(s); i++ {
		if s[i] != '0' {
			return false
		}
	}
	return true
}

func hasLettersThenDigits(s string, letters, digits int) bool {
	return len(s) == letters+digits && hasLettersThenDigitsAtLeast(s, letters, digits)
}

func hasLettersThenDigitsAtLeast(s string, letters, digits int) bool {
	if len(s) < letters+digits {
		return false
	}
	for i := 0; i < letters; i++ {
		if s[i] < 'A' || s[i] > 'Z' {
			return false
		}
	}
	for i := letters; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

type wgsProjectInfo struct {
	Locus  string
	Length int64
	Ranges []accessionRange
}

type accessionRange struct {
	First string
	Last  string
}

type accessionSerial struct {
	Prefix  string
	Number  int64
	Width   int
	Version string
}

func parseWGSProjectInfo(r io.Reader) (wgsProjectInfo, error) {
	decoder := xml.NewDecoder(r)
	var info wgsProjectInfo
	for {
		tok, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return wgsProjectInfo{}, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "INSDSeq_locus":
			var locus string
			if err := decoder.DecodeElement(&locus, &start); err != nil {
				return wgsProjectInfo{}, err
			}
			info.Locus = strings.TrimSpace(locus)
		case "INSDSeq_length":
			var length string
			if err := decoder.DecodeElement(&length, &start); err != nil {
				return wgsProjectInfo{}, err
			}
			if strings.TrimSpace(length) != "" {
				n, err := strconv.ParseInt(strings.TrimSpace(length), 10, 64)
				if err != nil {
					return wgsProjectInfo{}, fmt.Errorf("invalid WGS/TSA/TLS master length %q", length)
				}
				info.Length = n
			}
		case "INSDAltSeqItem":
			var item struct {
				First string `xml:"INSDAltSeqItem_first-accn"`
				Last  string `xml:"INSDAltSeqItem_last-accn"`
			}
			if err := decoder.DecodeElement(&item, &start); err != nil {
				return wgsProjectInfo{}, err
			}
			first := strings.TrimSpace(item.First)
			last := strings.TrimSpace(item.Last)
			if first != "" && last != "" {
				info.Ranges = append(info.Ranges, accessionRange{First: first, Last: last})
			}
		}
	}
	if info.Locus == "" && len(info.Ranges) == 0 {
		return wgsProjectInfo{}, fmt.Errorf("download produced no WGS/TSA/TLS master record")
	}
	return info, nil
}

func wgsRangesFromLocusLength(locus string, length int64) ([]accessionRange, error) {
	locus = strings.TrimSpace(locus)
	if locus == "" || length <= 0 {
		return nil, nil
	}
	i := len(locus)
	for i > 0 && locus[i-1] == '0' {
		i--
	}
	if i == len(locus) {
		return nil, fmt.Errorf("cannot infer WGS/TSA/TLS component range from locus %q", locus)
	}
	width := len(locus) - i
	prefix := locus[:i]
	return []accessionRange{
		{
			First: prefix + fmt.Sprintf("%0*d", width, 1),
			Last:  prefix + fmt.Sprintf("%0*d", width, length),
		},
	}, nil
}

func parseAccessionRangeSerial(r accessionRange) (accessionSerial, int64, error) {
	firstCore, firstVersion := splitAccessionVersion(r.First)
	lastCore, lastVersion := splitAccessionVersion(r.Last)
	if firstVersion != lastVersion {
		return accessionSerial{}, 0, fmt.Errorf("cannot expand component accession range %s-%s", r.First, r.Last)
	}

	prefix := commonPrefix(firstCore, lastCore)
	firstDigits := firstCore[len(prefix):]
	lastDigits := lastCore[len(prefix):]
	if firstDigits == "" && lastDigits == "" {
		prefix, firstDigits = splitTrailingDigits(firstCore)
		lastDigits = firstDigits
	}
	if firstDigits == "" || lastDigits == "" || len(firstDigits) != len(lastDigits) || !allDigits(firstDigits) || !allDigits(lastDigits) {
		return accessionSerial{}, 0, fmt.Errorf("cannot expand component accession range %s-%s", r.First, r.Last)
	}

	firstNumber, err := strconv.ParseInt(firstDigits, 10, 64)
	if err != nil {
		return accessionSerial{}, 0, fmt.Errorf("invalid accession numeric suffix %q", firstDigits)
	}
	lastNumber, err := strconv.ParseInt(lastDigits, 10, 64)
	if err != nil {
		return accessionSerial{}, 0, fmt.Errorf("invalid accession numeric suffix %q", lastDigits)
	}
	return accessionSerial{
		Prefix:  prefix,
		Number:  firstNumber,
		Width:   len(firstDigits),
		Version: firstVersion,
	}, lastNumber, nil
}

func splitAccessionVersion(accession string) (string, string) {
	if dot := strings.LastIndexByte(accession, '.'); dot > 0 && allDigits(accession[dot+1:]) {
		return accession[:dot], accession[dot:]
	}
	return accession, ""
}

func splitTrailingDigits(s string) (string, string) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[:i], s[i:]
}

func commonPrefix(a, b string) string {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func formatAccessionSerial(serial accessionSerial, n int64) string {
	return serial.Prefix + fmt.Sprintf("%0*d", serial.Width, n) + serial.Version
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
