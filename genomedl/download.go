package genomedl

import (
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
	defaultDatasetsDownloadURL = "https://api.ncbi.nlm.nih.gov/datasets/v2/genome/accession/%s/download?include_annotation_type=GENOME_FASTA&include_annotation_type=GENOME_GFF"
	defaultSviewerFastaURL     = "https://www.ncbi.nlm.nih.gov/sviewer/viewer.fcgi?id=%s&db=nuccore&report=fasta&retmode=text"
	defaultSviewerGFF3URL      = "https://www.ncbi.nlm.nih.gov/sviewer/viewer.fcgi?id=%s&db=nuccore&report=gff3&retmode=text"
	defaultHTTPTimeout         = 120 * time.Second
)

// Downloader downloads genome sequence files from NCBI endpoints.
type Downloader struct {
	// DatasetsDownloadURL is the fmt pattern for assembly genome downloads.
	DatasetsDownloadURL string
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
		SviewerFastaURL:     defaultSviewerFastaURL,
		SviewerGFF3URL:      defaultSviewerGFF3URL,
		HTTPClient:          &http.Client{Timeout: defaultHTTPTimeout},
	}
}

// DownloadGenome downloads accession into outPath using the default downloader.
func DownloadGenome(accession, outPath string) (string, error) {
	return NewDownloader().DownloadGenome(accession, outPath)
}

// DownloadGenome downloads accession into outPath.
func (d *Downloader) DownloadGenome(accession, outPath string) (string, error) {
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
	tmpDir, err := os.MkdirTemp("", "faqt-download-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	var files []string
	if isAssemblyAccession(acc) {
		files, err = d.downloadAssemblyGenome(acc, tmpDir)
	} else {
		files, err = d.downloadNuccoreGenome(acc, tmpDir)
	}
	if err != nil {
		return "", err
	}
	if err := writeDownloadedGenome(files, outPath); err != nil {
		return "", err
	}
	return outPath, nil
}

func isAssemblyAccession(accession string) bool {
	upper := strings.ToUpper(strings.TrimSpace(accession))
	return strings.HasPrefix(upper, "GCF_") || strings.HasPrefix(upper, "GCA_")
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(name)
}

func (d *Downloader) downloadAssemblyGenome(accession, outDir string) ([]string, error) {
	zipPath := filepath.Join(outDir, sanitizeFilename(accession)+".zip")
	if err := d.downloadURLToFile(fmt.Sprintf(d.datasetsDownloadURL(), accession), zipPath); err != nil {
		return nil, err
	}
	return extractGenomeFilesFromZip(zipPath, outDir)
}

func (d *Downloader) downloadNuccoreGenome(accession, outDir string) ([]string, error) {
	base := sanitizeFilename(accession)
	fastaPath := filepath.Join(outDir, base+".fa")
	if err := d.downloadURLToFile(fmt.Sprintf(d.sviewerFastaURL(), accession), fastaPath); err != nil {
		return nil, err
	}
	files := []string{fastaPath}

	gffPath := filepath.Join(outDir, base+".gff3")
	if err := d.downloadURLToFile(fmt.Sprintf(d.sviewerGFF3URL(), accession), gffPath); err == nil {
		files = append(files, gffPath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("download gff3: %w", err)
	}

	return files, nil
}

func (d *Downloader) downloadURLToFile(url, outPath string) (err error) {
	resp, err := d.httpClient().Get(url)
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

func (d *Downloader) datasetsDownloadURL() string {
	if d != nil && d.DatasetsDownloadURL != "" {
		return d.DatasetsDownloadURL
	}
	return defaultDatasetsDownloadURL
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
