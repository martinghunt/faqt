package genomedl

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/martinghunt/faqt/internal/closeutil"
)

const (
	defaultDatasetsDownloadURL = "https://api.ncbi.nlm.nih.gov/datasets/v2/genome/accession/%s/download?include_annotation_type=GENOME_FASTA&include_annotation_type=GENOME_GFF&include_annotation_type=GENOME_GBFF"
	defaultDatasetsFastaURL    = "https://api.ncbi.nlm.nih.gov/datasets/v2/genome/accession/%s/download?include_annotation_type=GENOME_FASTA"
	defaultDatasetsGFF3URL     = "https://api.ncbi.nlm.nih.gov/datasets/v2/genome/accession/%s/download?include_annotation_type=GENOME_FASTA&include_annotation_type=GENOME_GFF"
	defaultDatasetsGenBankURL  = "https://api.ncbi.nlm.nih.gov/datasets/v2/genome/accession/%s/download?include_annotation_type=GENOME_GBFF"
	defaultENAEmblURL          = "https://www.ebi.ac.uk/ena/browser/api/embl/%s?download=true"
	defaultSviewerFastaURL     = "https://www.ncbi.nlm.nih.gov/sviewer/viewer.fcgi?id=%s&db=nuccore&report=fasta&retmode=text"
	defaultSviewerGFF3URL      = "https://www.ncbi.nlm.nih.gov/sviewer/viewer.fcgi?id=%s&db=nuccore&report=gff3&retmode=text"
	defaultHTTPTimeout         = 120 * time.Second
)

// GenomeFormat controls which biological file format a genome download writes.
type GenomeFormat string

const (
	GenomeFormatAuto    GenomeFormat = "auto"
	GenomeFormatFASTA   GenomeFormat = "fasta"
	GenomeFormatGFF3    GenomeFormat = "gff3"
	GenomeFormatGenBank GenomeFormat = "genbank"
	GenomeFormatEMBL    GenomeFormat = "embl"
)

// ParseGenomeFormat parses a genome download format name.
func ParseGenomeFormat(value string) (GenomeFormat, error) {
	format := GenomeFormat(strings.ToLower(strings.TrimSpace(value)))
	switch format {
	case "", GenomeFormatAuto:
		return GenomeFormatAuto, nil
	case GenomeFormatFASTA, GenomeFormatGFF3, GenomeFormatGenBank, GenomeFormatEMBL:
		return format, nil
	case "gb", "gbk", "gbff":
		return GenomeFormatGenBank, nil
	default:
		return "", fmt.Errorf("unsupported genome format %q; allowed values: auto, fasta, gff3, genbank, gb, gbk, gbff, embl", value)
	}
}

// DownloadOptions controls genome download output.
type DownloadOptions struct {
	// Format chooses the biological format to write. The zero value behaves as
	// GenomeFormatAuto.
	Format GenomeFormat
	// Wrap controls FASTA line wrapping. The default 0 disables wrapping.
	Wrap int
	// FastaOnly downloads and writes genomic FASTA even when annotation is
	// available.
	//
	// Deprecated: use Format: GenomeFormatFASTA.
	FastaOnly bool
	// WarningWriter receives non-fatal warnings. If nil, warnings are
	// suppressed.
	WarningWriter io.Writer
}

// Downloader downloads genome sequence files from NCBI and ENA endpoints.
type Downloader struct {
	// DatasetsDownloadURL is the fmt pattern for assembly genome downloads.
	DatasetsDownloadURL string
	// ENAEmblURL is the fmt pattern for ENA EMBL downloads.
	ENAEmblURL string
	// SviewerFastaURL is the fmt pattern for nuccore FASTA downloads.
	SviewerFastaURL string
	// SviewerGFF3URL is the fmt pattern for nuccore GFF3 downloads.
	SviewerGFF3URL string
	// HTTPClient is used for download requests.
	HTTPClient *http.Client
}

// NewDownloader returns a downloader configured with the default NCBI endpoints.
func NewDownloader() *Downloader {
	return &Downloader{
		DatasetsDownloadURL: defaultDatasetsDownloadURL,
		ENAEmblURL:          defaultENAEmblURL,
		SviewerFastaURL:     defaultSviewerFastaURL,
		SviewerGFF3URL:      defaultSviewerGFF3URL,
		HTTPClient:          &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// DownloadGenome downloads accession into outPath using the default downloader.
func DownloadGenome(accession, outPath string) (string, error) {
	return NewDownloader().DownloadGenome(accession, outPath)
}

// DownloadGenomeWithOptions downloads accession into outPath using the default
// downloader.
func DownloadGenomeWithOptions(accession, outPath string, opts DownloadOptions) (string, error) {
	return NewDownloader().DownloadGenomeWithOptions(accession, outPath, opts)
}

// DownloadGenomeContext downloads accession into outPath using the default
// downloader and ctx for download requests.
func DownloadGenomeContext(ctx context.Context, accession, outPath string, opts DownloadOptions) (string, error) {
	return NewDownloader().DownloadGenomeContext(ctx, accession, outPath, opts)
}

// DownloadGenome downloads accession into outPath.
func (d *Downloader) DownloadGenome(accession, outPath string) (string, error) {
	return d.DownloadGenomeWithOptions(accession, outPath, DownloadOptions{})
}

// DownloadGenomeWithOptions downloads accession into outPath.
func (d *Downloader) DownloadGenomeWithOptions(accession, outPath string, opts DownloadOptions) (string, error) {
	return d.DownloadGenomeContext(context.Background(), accession, outPath, opts)
}

// DownloadGenomeContext downloads accession into outPath, using ctx for
// download requests.
func (d *Downloader) DownloadGenomeContext(ctx context.Context, accession, outPath string, opts DownloadOptions) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("nil context")
	}
	acc := strings.TrimSpace(accession)
	if acc == "" {
		return "", fmt.Errorf("empty accession")
	}
	if strings.TrimSpace(outPath) == "" {
		return "", fmt.Errorf("empty output path")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	format, err := normalizeGenomeFormat(opts)
	if err != nil {
		return "", err
	}
	opts.Format = format
	tmpDir, err := os.MkdirTemp("", "faqt-download-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	var files []string
	if format == GenomeFormatEMBL {
		files, err = d.downloadEMBLGenome(ctx, acc, tmpDir)
	} else if isAssemblyAccession(acc) {
		files, err = d.downloadAssemblyGenome(ctx, acc, tmpDir, format)
	} else {
		files, err = d.downloadNuccoreGenome(ctx, acc, tmpDir, format)
	}
	if err != nil {
		return "", err
	}
	return writeDownloadedGenomeWithOptions(files, outPath, opts)
}

func normalizeGenomeFormat(opts DownloadOptions) (GenomeFormat, error) {
	format, err := ParseGenomeFormat(string(opts.Format))
	if err != nil {
		return "", err
	}
	if opts.FastaOnly {
		if format != GenomeFormatAuto && format != GenomeFormatFASTA {
			return "", fmt.Errorf("FastaOnly cannot be used with genome format %q", format)
		}
		return GenomeFormatFASTA, nil
	}
	return format, nil
}

func isAssemblyAccession(accession string) bool {
	upper := strings.ToUpper(strings.TrimSpace(accession))
	return strings.HasPrefix(upper, "GCF_") || strings.HasPrefix(upper, "GCA_")
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(name)
}

func (d *Downloader) downloadAssemblyGenome(ctx context.Context, accession, outDir string, format GenomeFormat) ([]string, error) {
	zipPath := filepath.Join(outDir, sanitizeFilename(accession)+".zip")
	if err := d.downloadURLToFile(ctx, fmt.Sprintf(d.datasetsDownloadURL(format), accession), zipPath); err != nil {
		return nil, err
	}
	return extractGenomeFilesFromZip(zipPath, outDir)
}

func (d *Downloader) downloadNuccoreGenome(ctx context.Context, accession, outDir string, format GenomeFormat) ([]string, error) {
	base := sanitizeFilename(accession)
	fastaPath := filepath.Join(outDir, base+".fa")
	if err := d.downloadURLToFile(ctx, fmt.Sprintf(d.sviewerFastaURL(), accession), fastaPath); err != nil {
		return nil, err
	}
	files := []string{fastaPath}
	if format == GenomeFormatFASTA {
		return files, nil
	}

	gffPath := filepath.Join(outDir, base+".gff3")
	if err := d.downloadURLToFile(ctx, fmt.Sprintf(d.sviewerGFF3URL(), accession), gffPath); err == nil {
		files = append(files, gffPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("download gff3: %w", err)
	}

	return files, nil
}

func (d *Downloader) downloadEMBLGenome(ctx context.Context, accession, outDir string) ([]string, error) {
	emblPath := filepath.Join(outDir, sanitizeFilename(accession)+".embl")
	if err := d.downloadURLToFile(ctx, fmt.Sprintf(d.enaEmblURL(), accession), emblPath); err != nil {
		return nil, fmt.Errorf("download EMBL from ENA: %w", err)
	}
	if err := validateDownloadedENAEMBL(emblPath); err != nil {
		return nil, err
	}
	return []string{emblPath}, nil
}

func validateDownloadedENAEMBL(path string) (err error) {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, fh)

	buf := make([]byte, 4096)
	n, err := fh.Read(buf)
	if err != nil && err != io.EOF {
		return err
	}
	head := strings.TrimSpace(string(buf[:n]))
	if strings.HasPrefix(head, "ERROR:") {
		return fmt.Errorf("ENA EMBL download failed: %s", firstLine(head))
	}
	if !strings.HasPrefix(head, "ID") {
		return fmt.Errorf("downloaded ENA EMBL file has unexpected content")
	}
	return nil
}

func firstLine(text string) string {
	if i := strings.IndexByte(text, '\n'); i >= 0 {
		return text[:i]
	}
	return text
}

func (d *Downloader) downloadURLToFile(ctx context.Context, url, outPath string) (err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, out)

	_, err = io.Copy(out, resp.Body)
	return err
}

func (d *Downloader) datasetsDownloadURL(format GenomeFormat) string {
	if d != nil && d.DatasetsDownloadURL != "" {
		if d.DatasetsDownloadURL == defaultDatasetsDownloadURL {
			switch format {
			case GenomeFormatFASTA:
				return defaultDatasetsFastaURL
			case GenomeFormatGFF3:
				return defaultDatasetsGFF3URL
			case GenomeFormatGenBank:
				return defaultDatasetsGenBankURL
			}
		}
		return d.DatasetsDownloadURL
	}
	switch format {
	case GenomeFormatFASTA:
		return defaultDatasetsFastaURL
	case GenomeFormatGFF3:
		return defaultDatasetsGFF3URL
	case GenomeFormatGenBank:
		return defaultDatasetsGenBankURL
	default:
		return defaultDatasetsDownloadURL
	}
}

func (d *Downloader) enaEmblURL() string {
	if d != nil && d.ENAEmblURL != "" {
		return d.ENAEmblURL
	}
	return defaultENAEmblURL
}

func (d *Downloader) sviewerFastaURL() string {
	if d != nil && d.SviewerFastaURL != "" {
		return d.SviewerFastaURL
	}
	return defaultSviewerFastaURL
}

func (d *Downloader) sviewerGFF3URL() string {
	if d != nil && d.SviewerGFF3URL != "" {
		return d.SviewerGFF3URL
	}
	return defaultSviewerGFF3URL
}

func (d *Downloader) httpClient() *http.Client {
	if d != nil && d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{Timeout: defaultHTTPTimeout}
}
