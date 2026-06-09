package genomedl

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/internal/xopen"
)

func TestIsAssemblyAccession(t *testing.T) {
	if !isAssemblyAccession("GCF_000001405.40") {
		t.Fatal("expected GCF accession to be assembly")
	}
	if !isAssemblyAccession(" gca_000001405.1 ") {
		t.Fatal("expected GCA accession to be assembly")
	}
	if isAssemblyAccession("NC_000962.3") {
		t.Fatal("nuccore accession should not be assembly")
	}
}

func TestSanitizeFilename(t *testing.T) {
	got := sanitizeFilename(`NC 000001.1:alt/path\name`)
	want := "NC_000001.1_alt_path_name"
	if got != want {
		t.Fatalf("sanitizeFilename() = %q, want %q", got, want)
	}
}

func TestDownloadGenomeNuccore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "report=fasta"):
			_, _ = w.Write([]byte(">chr1\nACGT\n"))
		case strings.Contains(r.URL.RawQuery, "report=gff3"):
			_, _ = w.Write([]byte("##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.SviewerFastaURL = server.URL + "/viewer?id=%s&report=fasta"
	downloader.SviewerGFF3URL = server.URL + "/viewer?id=%s&report=gff3"

	outPath := filepath.Join(t.TempDir(), "genome.gff3")
	gotPath, err := downloader.DownloadGenome("NC_000001.1", outPath)
	if err != nil {
		t.Fatalf("DownloadGenome() error = %v", err)
	}
	if gotPath != outPath {
		t.Fatalf("got path = %q, want %q", gotPath, outPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "##gff-version 3") || !strings.Contains(text, "##FASTA") || !strings.Contains(text, ">chr1") {
		t.Fatalf("combined output = %q", text)
	}
	if strings.Contains(text, "\n\n##FASTA\n") {
		t.Fatalf("combined output contains extra blank line before ##FASTA: %q", text)
	}
}

func TestDownloadGenomeNuccoreReportsGFFDownloadErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "report=fasta"):
			_, _ = w.Write([]byte(">chr1\nACGT\n"))
		case strings.Contains(r.URL.RawQuery, "report=gff3"):
			http.Error(w, "temporary failure", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.SviewerFastaURL = server.URL + "/viewer?id=%s&report=fasta"
	downloader.SviewerGFF3URL = server.URL + "/viewer?id=%s&report=gff3"

	_, err := downloader.DownloadGenome("NC_000001.1", filepath.Join(t.TempDir(), "genome.gff3"))
	if err == nil || !strings.Contains(err.Error(), "download gff3") {
		t.Fatalf("DownloadGenome() error = %v, want GFF3 download error", err)
	}
}

func TestDatasetsDownloadURLRequestsSupportedAnnotationByDefault(t *testing.T) {
	downloader := NewDownloader()
	url := downloader.datasetsDownloadURL(false)
	for _, want := range []string{"GENOME_FASTA", "GENOME_GFF", "GENOME_GBFF"} {
		if !strings.Contains(url, want) {
			t.Fatalf("datasetsDownloadURL(false) = %q, missing %s", url, want)
		}
	}
}

func TestDatasetsDownloadURLFastaOnly(t *testing.T) {
	downloader := NewDownloader()
	url := downloader.datasetsDownloadURL(true)
	if !strings.Contains(url, "GENOME_FASTA") {
		t.Fatalf("datasetsDownloadURL(true) = %q, missing GENOME_FASTA", url)
	}
	for _, unwanted := range []string{"GENOME_GFF", "GENOME_GBFF"} {
		if strings.Contains(url, unwanted) {
			t.Fatalf("datasetsDownloadURL(true) = %q, contains %s", url, unwanted)
		}
	}
}

func TestDownloadGenomeNuccoreFastaOnlySkipsGFF3(t *testing.T) {
	var gffRequested bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "report=fasta"):
			_, _ = w.Write([]byte(">chr1\nACGT\n"))
		case strings.Contains(r.URL.RawQuery, "report=gff3"):
			gffRequested = true
			http.Error(w, "temporary failure", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.SviewerFastaURL = server.URL + "/viewer?id=%s&report=fasta"
	downloader.SviewerGFF3URL = server.URL + "/viewer?id=%s&report=gff3"

	outPath := filepath.Join(t.TempDir(), "genome.fa")
	gotPath, err := downloader.DownloadGenomeWithOptions("NC_000001.1", outPath, DownloadOptions{
		FastaOnly: true,
	})
	if err != nil {
		t.Fatalf("DownloadGenomeWithOptions() error = %v", err)
	}
	if gffRequested {
		t.Fatal("FASTA-only download should not request GFF3")
	}
	if gotPath != outPath {
		t.Fatalf("path = %q, want %q", gotPath, outPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != ">chr1\nACGT\n" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestDownloadGenomeNuccoreFallsBackToFASTAWhenGFF3Missing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.RawQuery, "report=fasta"):
			_, _ = w.Write([]byte(">chr1\nACGT\n"))
		case strings.Contains(r.URL.RawQuery, "report=gff3"):
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.SviewerFastaURL = server.URL + "/viewer?id=%s&report=fasta"
	downloader.SviewerGFF3URL = server.URL + "/viewer?id=%s&report=gff3"

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "genome.gff3")
	gotPath, err := downloader.DownloadGenome("NC_000001.1", outPath)
	if err != nil {
		t.Fatalf("DownloadGenome() error = %v", err)
	}
	if gotPath != outPath {
		t.Fatalf("path = %q, want %q", gotPath, outPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != ">chr1\nACGT\n" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestDownloadGenomeAssemblyFallsBackToFASTAOnly(t *testing.T) {
	var zipData bytes.Buffer
	zw := zip.NewWriter(&zipData)
	w, err := zw.Create("ncbi_dataset/data/GCF_1/GCF_1_genomic.fna")
	if err != nil {
		t.Fatalf("Create(zip entry) error = %v", err)
	}
	if _, err := w.Write([]byte(">chr1\nACGT\n")); err != nil {
		t.Fatalf("Write(zip entry) error = %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("Close(zip writer) error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipData.Bytes())
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.DatasetsDownloadURL = server.URL + "/download/%s"

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "genome.out")
	gotPath, err := downloader.DownloadGenome("GCF_000191525.1", outPath)
	if err != nil {
		t.Fatalf("DownloadGenome() error = %v", err)
	}
	if gotPath != outPath {
		t.Fatalf("path = %q, want %q", gotPath, outPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != ">chr1\nACGT\n" {
		t.Fatalf("output = %q", string(data))
	}
}

func TestDownloadGenomeAssemblyWritesGBFFWhenAvailable(t *testing.T) {
	var zipData bytes.Buffer
	zw := zip.NewWriter(&zipData)
	writeEntry := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(zip entry) error = %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Write(zip entry) error = %v", err)
		}
	}
	writeEntry("ncbi_dataset/data/GCF_1/GCF_1_genomic.fna", ">chr1\nACGT\n")
	writeEntry("ncbi_dataset/data/GCF_1/genomic.gbff", "LOCUS       REC1\nORIGIN\n        1 acgt\n//\n")
	if err := zw.Close(); err != nil {
		t.Fatalf("Close(zip writer) error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(zipData.Bytes())
	}))
	defer server.Close()

	downloader := NewDownloader()
	downloader.DatasetsDownloadURL = server.URL + "/download/%s"

	outPath := filepath.Join(t.TempDir(), "genome.out")
	gotPath, err := downloader.DownloadGenome("GCF_000191525.1", outPath)
	if err != nil {
		t.Fatalf("DownloadGenome() error = %v", err)
	}
	if gotPath != outPath {
		t.Fatalf("path = %q, want %q", gotPath, outPath)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "LOCUS") {
		t.Fatalf("output = %q", string(data))
	}
}

func TestDownloadURLToFileReportsBodyCloseError(t *testing.T) {
	closeErr := errors.New("close failed")
	downloader := &Downloader{
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       closeErrorBody{Reader: strings.NewReader(">chr1\nACGT\n"), err: closeErr},
					Header:     make(http.Header),
					Request:    r,
				}, nil
			}),
		},
	}

	err := downloader.downloadURLToFile("https://example.test/genome.fa", filepath.Join(t.TempDir(), "genome.fa"))
	if !errors.Is(err, closeErr) {
		t.Fatalf("downloadURLToFile() error = %v, want close error", err)
	}
}

func TestExtractGenomeFilesFromZipPreservesOriginalFiles(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	zw := zip.NewWriter(out)
	writeEntry := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("Create(zip entry) error = %v", err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("Write(zip entry) error = %v", err)
		}
	}
	writeEntry("ncbi_dataset/data/GCF_1/genome.gbk", "LOCUS       REC1\nORIGIN\n        1 acgt\n//\n")
	writeEntry("ncbi_dataset/data/GCF_1/annot.gff3", "##gff-version 3\n")
	if err := zw.Close(); err != nil {
		t.Fatalf("Close(zip writer) error = %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("Close(zip file) error = %v", err)
	}

	files, err := extractGenomeFilesFromZip(zipPath, tmpDir)
	if err != nil {
		t.Fatalf("extractGenomeFilesFromZip() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %v", files)
	}
	if filepath.Ext(files[0]) != ".gbk" && filepath.Ext(files[1]) != ".gbk" {
		t.Fatalf("expected preserved .gbk file, got %v", files)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

type closeErrorBody struct {
	*strings.Reader
	err error
}

func (b closeErrorBody) Close() error {
	return b.err
}

func TestWriteDownloadedGenomeSingleFilePreservesOriginalType(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "genome.gbk")
	dst := filepath.Join(tmpDir, "out.gbk")
	if err := os.WriteFile(src, []byte("LOCUS       REC1\nORIGIN\n        1 acgt\n//\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(src) error = %v", err)
	}
	if err := writeDownloadedGenome([]string{src}, dst); err != nil {
		t.Fatalf("writeDownloadedGenome() error = %v", err)
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile(dst) error = %v", err)
	}
	if !strings.Contains(string(data), "LOCUS") {
		t.Fatalf("output = %q", string(data))
	}
}

func TestWriteDownloadedGenomeUsesEMBLAnnotationOverFASTA(t *testing.T) {
	tmpDir := t.TempDir()
	fastaPath := filepath.Join(tmpDir, "genome.fa")
	emblPath := filepath.Join(tmpDir, "genome.embl")
	outPath := filepath.Join(tmpDir, "out.embl")
	if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasta) error = %v", err)
	}
	if err := os.WriteFile(emblPath, []byte("ID   REC1\nSQ   Sequence 4 BP;\n     acgt\n//\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(embl) error = %v", err)
	}

	if err := writeDownloadedGenome([]string{fastaPath, emblPath}, outPath); err != nil {
		t.Fatalf("writeDownloadedGenome() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(out) error = %v", err)
	}
	if !strings.Contains(string(data), "ID   REC1") {
		t.Fatalf("output = %q", string(data))
	}
}

func TestWriteDownloadedGenomeWarnsOnOutputExtensionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	fastaPath := filepath.Join(tmpDir, "genome.fa")
	emblPath := filepath.Join(tmpDir, "genome.embl")
	outPath := filepath.Join(tmpDir, "out.fa.gz")
	if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasta) error = %v", err)
	}
	if err := os.WriteFile(emblPath, []byte("ID   REC1\nSQ   Sequence 4 BP;\n     acgt\n//\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(embl) error = %v", err)
	}

	var warnings bytes.Buffer
	gotPath, err := writeDownloadedGenomeWithOptions([]string{fastaPath, emblPath}, outPath, DownloadOptions{
		WarningWriter: &warnings,
	})
	if err != nil {
		t.Fatalf("writeDownloadedGenomeWithOptions() error = %v", err)
	}
	if gotPath != outPath {
		t.Fatalf("path = %q, want %q", gotPath, outPath)
	}
	warning := warnings.String()
	if !strings.Contains(warning, "writing EMBL content") ||
		!strings.Contains(warning, "FASTA extension") ||
		!strings.Contains(warning, outPath) {
		t.Fatalf("warning = %q", warning)
	}
	data := readXOpenPath(t, outPath)
	if !strings.Contains(string(data), "ID   REC1") {
		t.Fatalf("output = %q", string(data))
	}
}

func TestWriteDownloadedGenomeDoesNotWarnForMatchingOrNeutralOutputPath(t *testing.T) {
	tests := []struct {
		name    string
		outPath string
		opts    DownloadOptions
		files   func(t *testing.T, tmpDir string) []string
	}{
		{
			name:    "matching compressed fasta",
			outPath: "out.fa.gz",
			opts:    DownloadOptions{FastaOnly: true},
			files: func(t *testing.T, tmpDir string) []string {
				t.Helper()
				fastaPath := filepath.Join(tmpDir, "genome.fa")
				if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(fasta) error = %v", err)
				}
				return []string{fastaPath}
			},
		},
		{
			name:    "neutral compressed output path",
			outPath: "out.gz",
			files: func(t *testing.T, tmpDir string) []string {
				t.Helper()
				fastaPath := filepath.Join(tmpDir, "genome.fa")
				emblPath := filepath.Join(tmpDir, "genome.embl")
				if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(fasta) error = %v", err)
				}
				if err := os.WriteFile(emblPath, []byte("ID   REC1\nSQ   Sequence 4 BP;\n     acgt\n//\n"), 0o644); err != nil {
					t.Fatalf("WriteFile(embl) error = %v", err)
				}
				return []string{fastaPath, emblPath}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			outPath := filepath.Join(tmpDir, tt.outPath)
			var warnings bytes.Buffer
			opts := tt.opts
			opts.WarningWriter = &warnings
			if _, err := writeDownloadedGenomeWithOptions(tt.files(t, tmpDir), outPath, opts); err != nil {
				t.Fatalf("writeDownloadedGenomeWithOptions() error = %v", err)
			}
			if warnings.Len() != 0 {
				t.Fatalf("warning = %q, want no warning", warnings.String())
			}
		})
	}
}

func TestWriteDownloadedGenomeCompressesCombinedOutputByExtension(t *testing.T) {
	tmpDir := t.TempDir()
	gffPath := filepath.Join(tmpDir, "annot.gff3")
	fastaPath := filepath.Join(tmpDir, "genome.fa")
	outPath := filepath.Join(tmpDir, "combined.gff3.gz")
	if err := os.WriteFile(gffPath, []byte("##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(gff) error = %v", err)
	}
	if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasta) error = %v", err)
	}

	if err := writeDownloadedGenome([]string{gffPath, fastaPath}, outPath); err != nil {
		t.Fatalf("writeDownloadedGenome() error = %v", err)
	}
	data := readXOpenPath(t, outPath)
	want := "##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n##FASTA\n>chr1\nACGT\n"
	if string(data) != want {
		t.Fatalf("output = %q, want %q", string(data), want)
	}
}

func TestWriteDownloadedGenomeFastaOnlyCompressesByExtension(t *testing.T) {
	for _, suffix := range []string{".gz", ".bz2", ".xz", ".zst"} {
		t.Run(suffix, func(t *testing.T) {
			tmpDir := t.TempDir()
			fastaPath := filepath.Join(tmpDir, "genome.fa")
			outPath := filepath.Join(tmpDir, "out.fa"+suffix)
			if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT\n"), 0o644); err != nil {
				t.Fatalf("WriteFile(fasta) error = %v", err)
			}

			gotPath, err := writeDownloadedGenomeWithOptions([]string{fastaPath}, outPath, DownloadOptions{
				FastaOnly: true,
			})
			if err != nil {
				t.Fatalf("writeDownloadedGenomeWithOptions() error = %v", err)
			}
			if gotPath != outPath {
				t.Fatalf("path = %q, want %q", gotPath, outPath)
			}
			data := readXOpenPath(t, outPath)
			if string(data) != ">chr1\nACGT\n" {
				t.Fatalf("output = %q", string(data))
			}
		})
	}
}

func readXOpenPath(t *testing.T, path string) []byte {
	t.Helper()

	in, err := xopen.Open(path)
	if err != nil {
		t.Fatalf("xopen.Open() error = %v", err)
	}
	defer func() {
		if err := in.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()
	data, err := io.ReadAll(in)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return data
}

func TestCombineGFF3AndFASTATrimsAndTerminatesOutput(t *testing.T) {
	tmpDir := t.TempDir()
	gffPath := filepath.Join(tmpDir, "annot.gff3")
	fastaPath := filepath.Join(tmpDir, "genome.fa")
	outPath := filepath.Join(tmpDir, "combined.gff3")
	if err := os.WriteFile(gffPath, []byte("##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n\n\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(gff) error = %v", err)
	}
	if err := os.WriteFile(fastaPath, []byte(">chr1\nACGT"), 0o644); err != nil {
		t.Fatalf("WriteFile(fasta) error = %v", err)
	}

	if err := combineGFF3AndFASTA(gffPath, fastaPath, outPath); err != nil {
		t.Fatalf("combineGFF3AndFASTA() error = %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile(out) error = %v", err)
	}
	want := "##gff-version 3\nchr1\tsrc\tgene\t1\t4\t.\t+\t.\tID=g1\n##FASTA\n>chr1\nACGT\n"
	if string(data) != want {
		t.Fatalf("combined output = %q, want %q", string(data), want)
	}
}
