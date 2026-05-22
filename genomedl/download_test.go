package genomedl

import (
	"archive/zip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
