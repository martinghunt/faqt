package genomedl

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
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

func extractGenomeFilesFromZip(zipPath, outDir string) (files []string, err error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer closeutil.CloseWithError(&err, reader)

	files = make([]string, 0, 2)
	var seenFASTA bool
	for _, file := range reader.File {
		lowerName := strings.ToLower(file.Name)
		switch {
		case strings.HasSuffix(lowerName, ".fna"),
			strings.HasSuffix(lowerName, ".fa"),
			strings.HasSuffix(lowerName, ".fasta"),
			strings.HasSuffix(lowerName, ".gff"),
			strings.HasSuffix(lowerName, ".gff3"),
			strings.HasSuffix(lowerName, ".gb"),
			strings.HasSuffix(lowerName, ".gbk"),
			strings.HasSuffix(lowerName, ".genbank"),
			strings.HasSuffix(lowerName, ".embl"):
		default:
			continue
		}
		outPath := filepath.Join(outDir, filepath.Base(file.Name))
		if err := extractZipFile(file, outPath); err != nil {
			return nil, err
		}
		if isSequenceFile(lowerName) {
			seenFASTA = true
		}
		files = append(files, outPath)
	}
	if !seenFASTA {
		return nil, fmt.Errorf("downloaded archive did not contain a genome sequence file")
	}
	return files, nil
}

func extractZipFile(file *zip.File, outPath string) (err error) {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, src)

	dst, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, dst)

	_, err = io.Copy(dst, src)
	return err
}

func isSequenceFile(name string) bool {
	return strings.HasSuffix(name, ".fna") ||
		strings.HasSuffix(name, ".fa") ||
		strings.HasSuffix(name, ".fasta") ||
		strings.HasSuffix(name, ".gb") ||
		strings.HasSuffix(name, ".gbk") ||
		strings.HasSuffix(name, ".genbank") ||
		strings.HasSuffix(name, ".embl")
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_", ":", "_")
	return replacer.Replace(name)
}

func writeDownloadedGenome(files []string, outPath string) error {
	if len(files) == 0 {
		return fmt.Errorf("download produced no files")
	}
	sort.Strings(files)
	if len(files) == 1 {
		return copyFile(files[0], outPath)
	}

	var fastaPath string
	var gffPath string
	for _, path := range files {
		lower := strings.ToLower(path)
		switch {
		case strings.HasSuffix(lower, ".fa"), strings.HasSuffix(lower, ".fasta"), strings.HasSuffix(lower, ".fna"):
			fastaPath = path
		case strings.HasSuffix(lower, ".gff"), strings.HasSuffix(lower, ".gff3"):
			gffPath = path
		}
	}
	if fastaPath != "" && gffPath != "" && len(files) == 2 {
		return combineGFF3AndFASTA(gffPath, fastaPath, outPath)
	}
	return fmt.Errorf("download produced multiple files that cannot be combined into one output: %v", files)
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, in)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, out)

	_, err = io.Copy(out, in)
	return err
}

func combineGFF3AndFASTA(gffPath, fastaPath, outPath string) (err error) {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, out)

	gffBytes, err := copyFileTrimTrailingNewlines(out, gffPath)
	if err != nil {
		return err
	}
	if gffBytes > 0 {
		if _, err := out.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(out, "##FASTA\n"); err != nil {
		return err
	}

	fastaBytes, lastByte, err := copyFileTrackingLastByte(out, fastaPath)
	if err != nil {
		return err
	}
	if fastaBytes > 0 && lastByte != '\n' {
		if _, err := out.Write([]byte{'\n'}); err != nil {
			return err
		}
	}
	return nil
}

func copyFileTrimTrailingNewlines(w io.Writer, path string) (written int64, err error) {
	in, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer closeutil.CloseWithError(&err, in)

	info, err := in.Stat()
	if err != nil {
		return 0, err
	}
	trimmedSize, err := trimmedFileSize(in, info.Size())
	if err != nil {
		return 0, err
	}
	if _, err := in.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if trimmedSize == 0 {
		return 0, nil
	}
	return io.CopyN(w, in, trimmedSize)
}

func trimmedFileSize(file *os.File, size int64) (int64, error) {
	buf := make([]byte, 32*1024)
	for size > 0 {
		readSize := int64(len(buf))
		if size < readSize {
			readSize = size
		}
		offset := size - readSize
		n, err := file.ReadAt(buf[:readSize], offset)
		if err != nil && err != io.EOF {
			return 0, err
		}
		i := n - 1
		for i >= 0 && (buf[i] == '\n' || buf[i] == '\r') {
			i--
		}
		size = offset + int64(i+1)
		if i >= 0 {
			break
		}
	}
	return size, nil
}

func copyFileTrackingLastByte(w io.Writer, path string) (written int64, last byte, err error) {
	in, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer closeutil.CloseWithError(&err, in)

	tracker := &lastByteWriter{w: w}
	n, err := io.Copy(tracker, in)
	return n, tracker.last, err
}

type lastByteWriter struct {
	w    io.Writer
	last byte
}

func (w *lastByteWriter) Write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if n > 0 {
		w.last = p[n-1]
	}
	return n, err
}
