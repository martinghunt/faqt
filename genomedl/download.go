package genomedl

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	datasetsDownloadURL = "https://api.ncbi.nlm.nih.gov/datasets/v2/genome/accession/%s/download?include_annotation_type=GENOME_FASTA&include_annotation_type=GENOME_GFF"
	sviewerFastaURL     = "https://www.ncbi.nlm.nih.gov/sviewer/viewer.fcgi?id=%s&db=nuccore&report=fasta&retmode=text"
	sviewerGFF3URL      = "https://www.ncbi.nlm.nih.gov/sviewer/viewer.fcgi?id=%s&db=nuccore&report=gff3&retmode=text"
	httpClient          = &http.Client{Timeout: 120 * time.Second}
)

func DownloadGenome(accession, outPath string) (string, error) {
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
		files, err = downloadAssemblyGenome(acc, tmpDir)
	} else {
		files, err = downloadNuccoreGenome(acc, tmpDir)
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

func downloadAssemblyGenome(accession, outDir string) ([]string, error) {
	zipPath := filepath.Join(outDir, sanitizeFilename(accession)+".zip")
	if err := downloadURLToFile(fmt.Sprintf(datasetsDownloadURL, accession), zipPath); err != nil {
		return nil, err
	}
	return extractGenomeFilesFromZip(zipPath, outDir)
}

func downloadNuccoreGenome(accession, outDir string) ([]string, error) {
	base := sanitizeFilename(accession)
	fastaPath := filepath.Join(outDir, base+".fa")
	if err := downloadURLToFile(fmt.Sprintf(sviewerFastaURL, accession), fastaPath); err != nil {
		return nil, err
	}
	files := []string{fastaPath}

	gffPath := filepath.Join(outDir, base+".gff3")
	if err := downloadURLToFile(fmt.Sprintf(sviewerGFF3URL, accession), gffPath); err == nil {
		files = append(files, gffPath)
	} else if os.IsNotExist(err) {
		// nothing
	}

	return files, nil
}

func downloadURLToFile(url, outPath string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
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
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractGenomeFilesFromZip(zipPath, outDir string) ([]string, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	files := make([]string, 0, 2)
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

func extractZipFile(file *zip.File, outPath string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer dst.Close()

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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func combineGFF3AndFASTA(gffPath, fastaPath, outPath string) error {
	gffData, err := os.ReadFile(gffPath)
	if err != nil {
		return err
	}
	fastaData, err := os.ReadFile(fastaPath)
	if err != nil {
		return err
	}
	gffData = trimTrailingBlankLines(gffData)
	var buf bytes.Buffer
	buf.Write(gffData)
	if len(gffData) > 0 && gffData[len(gffData)-1] != '\n' {
		buf.WriteByte('\n')
	}
	buf.WriteString("##FASTA\n")
	buf.Write(fastaData)
	if len(fastaData) > 0 && fastaData[len(fastaData)-1] != '\n' {
		buf.WriteByte('\n')
	}
	return os.WriteFile(outPath, buf.Bytes(), 0o644)
}

func trimTrailingBlankLines(data []byte) []byte {
	data = bytes.TrimRight(data, "\r\n")
	return data
}
