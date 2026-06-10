package readdl

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/martinghunt/ichsm"
)

func TestDownloadReadsWithENA(t *testing.T) {
	fq1 := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	fq2 := gzipBytes(t, []byte("@r2\nGT\n+\n!!\n"))

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			query := r.URL.Query()
			if got := query.Get("result"); got != "read_run" {
				t.Fatalf("result = %q, want read_run", got)
			}
			if got := query.Get("query"); got != "run_accession=ERR123456" {
				t.Fatalf("query = %q", got)
			}
			if got := query.Get("fields"); got != "run_accession,fastq_ftp,fastq_md5,fastq_bytes" {
				t.Fatalf("fields = %q", got)
			}
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[` +
				`{"run_accession":"ERR123456",` +
				`"fastq_ftp":"` + host + `/ERR123456_1.fastq.gz;` + host + `/ERR123456_2.fastq.gz",` +
				`"fastq_md5":"` + md5Hex(fq1) + `;` + md5Hex(fq2) + `",` +
				`"fastq_bytes":"` + intString(len(fq1)) + `;` + intString(len(fq2)) + `"}` +
				`]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq1)
		case "/ERR123456_2.fastq.gz":
			_, _ = w.Write(fq2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir: outDir,
		Attempts:  1,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}

	wantDir := outDir
	if result.Dir != wantDir {
		t.Fatalf("dir = %q, want %q", result.Dir, wantDir)
	}
	if result.Method != MethodENA {
		t.Fatalf("method = %q, want %q", result.Method, MethodENA)
	}
	if result.MetaPath != "" {
		t.Fatalf("meta path = %q, want empty", result.MetaPath)
	}
	if len(result.Files) != 2 {
		t.Fatalf("len(files) = %d, want 2", len(result.Files))
	}
	for _, file := range result.Files {
		if _, err := os.Stat(file.Path); err != nil {
			t.Fatalf("expected downloaded file %s: %v", file.Path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(wantDir, "ERR123456_ena_meta.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata stat error = %v, want not exist", err)
	}
}

func TestDownloadReadsReportsProgress(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz","fastq_md5":"` + md5Hex(fq) + `","fastq_bytes":"` + intString(len(fq)) + `"}]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	var progress bytes.Buffer
	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	_, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:                outDir,
		Attempts:                 1,
		DownloadProgressInterval: time.Nanosecond,
		ProgressWriter:           &progress,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}

	got := progress.String()
	for _, want := range []string{
		"download-reads: attempt 1/1 using ena\n",
		"download-reads: querying ENA read files for ERR123456\n",
		"download-reads: found 1 FASTQ file(s)\n",
		"download-reads: downloading ERR123456_1.fastq.gz\n",
		"download-reads: validated ERR123456_1.fastq.gz\n",
		"download-reads: wrote 1 FASTQ file(s) to " + outDir + "\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("progress output missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "download-reads: downloaded ERR123456_1.fastq.gz: ") {
		t.Fatalf("progress output missing byte progress in:\n%s", got)
	}
	if !strings.Contains(got, "100.0%") {
		t.Fatalf("progress output missing completion percentage in:\n%s", got)
	}
}

func TestDownloadReadsWithENAMD5MismatchFailsAndDoesNotCreateFinalDir(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz","fastq_md5":"bad","fastq_bytes":"1"}]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	_, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir: outDir,
		Attempts:  1,
	})
	if err == nil || !strings.Contains(err.Error(), "md5 mismatch") {
		t.Fatalf("DownloadReads() error = %v, want md5 mismatch", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "ERR123456_1.fastq.gz")); !os.IsNotExist(statErr) {
		t.Fatalf("final FASTQ stat error = %v, want not exist", statErr)
	}
}

func TestDownloadReadsWithENAInvalidGzipFailsEvenWhenMD5Matches(t *testing.T) {
	badGzip := []byte("not gzip")
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz","fastq_md5":"` + md5Hex(badGzip) + `","fastq_bytes":"8"}]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(badGzip)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	_, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir: outDir,
		Attempts:  1,
	})
	if err == nil || !strings.Contains(err.Error(), "gzip validation failed") {
		t.Fatalf("DownloadReads() error = %v, want gzip validation failure", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "ERR123456_1.fastq.gz")); !os.IsNotExist(statErr) {
		t.Fatalf("final FASTQ stat error = %v, want not exist", statErr)
	}
}

func TestDownloadReadsWithENAStalledDownloadFails(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz","fastq_md5":"` + md5Hex(fq) + `","fastq_bytes":"` + intString(len(fq)) + `"}]`))
		case "/ERR123456_1.fastq.gz":
			w.Header().Set("Content-Type", "application/gzip")
			_, _ = w.Write(fq[:1])
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			<-r.Context().Done()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	_, err := downloader.DownloadReads(ctx, "ERR123456", DownloadOptions{
		OutputDir:            outDir,
		Attempts:             1,
		DownloadStallTimeout: 50 * time.Millisecond,
	})
	if err == nil || !strings.Contains(err.Error(), "download stalled for 50ms") {
		t.Fatalf("DownloadReads() error = %v, want stalled download", err)
	}
	if _, statErr := os.Stat(filepath.Join(outDir, "ERR123456_1.fastq.gz")); !os.IsNotExist(statErr) {
		t.Fatalf("final FASTQ stat error = %v, want not exist", statErr)
	}
}

func TestDownloadReadsWithENAOutputPrefix(t *testing.T) {
	fq1 := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	fq2 := gzipBytes(t, []byte("@r2\nGT\n+\n!!\n"))

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[` +
				`{"run_accession":"ERR123456",` +
				`"fastq_ftp":"` + host + `/ERR123456_1.fastq.gz;` + host + `/ERR123456_2.fastq.gz",` +
				`"fastq_md5":"` + md5Hex(fq1) + `;` + md5Hex(fq2) + `",` +
				`"fastq_bytes":"10;20"}` +
				`]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq1)
		case "/ERR123456_2.fastq.gz":
			_, _ = w.Write(fq2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:     outDir,
		OutputPrefix:  "sampleA",
		Attempts:      1,
		RetryDelayMin: 0,
		RetryDelayMax: 0,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}

	wantDir := outDir
	for _, filename := range []string{"sampleA_1.fastq.gz", "sampleA_2.fastq.gz"} {
		if _, err := os.Stat(filepath.Join(wantDir, filename)); err != nil {
			t.Fatalf("expected prefixed file %s: %v", filename, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "ERR123456")); !os.IsNotExist(err) {
		t.Fatalf("run directory stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "ERR123456_1.fastq.gz")); !os.IsNotExist(err) {
		t.Fatalf("original filename stat error = %v, want not exist", err)
	}
	if result.Dir != outDir {
		t.Fatalf("result dir = %q, want %q", result.Dir, outDir)
	}
	if got := result.Files[0].Filename; got != "sampleA_1.fastq.gz" {
		t.Fatalf("result filename = %q, want sampleA_1.fastq.gz", got)
	}
	if result.Files[0].Path != filepath.Join(outDir, "sampleA_1.fastq.gz") {
		t.Fatalf("result file path = %q", result.Files[0].Path)
	}
	if result.MetaPath != "" {
		t.Fatalf("result meta path = %q, want empty", result.MetaPath)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sampleA_ena_meta.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata stat error = %v, want not exist", err)
	}
}

func TestDownloadReadsWritesENAMetadataWhenRequestedWithPrefix(t *testing.T) {
	fq1 := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	fq2 := gzipBytes(t, []byte("@r2\nGT\n+\n!!\n"))

	var searchRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			searchRequests.Add(1)
			fields := r.URL.Query().Get("fields")
			if fields != "ALL" {
				t.Fatalf("fields = %q, want ALL", fields)
			}
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[` +
				`{"run_accession":"ERR123456",` +
				`"library_layout":"PAIRED",` +
				`"center_name":"test-center",` +
				`"fastq_ftp":"` + host + `/ERR123456_1.fastq.gz;` + host + `/ERR123456_2.fastq.gz",` +
				`"fastq_md5":"` + md5Hex(fq1) + `;` + md5Hex(fq2) + `",` +
				`"fastq_bytes":"10;20"}` +
				`]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq1)
		case "/ERR123456_2.fastq.gz":
			_, _ = w.Write(fq2)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:     outDir,
		OutputPrefix:  "sampleA",
		WriteMetadata: true,
		Attempts:      1,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}
	if got := searchRequests.Load(); got != 1 {
		t.Fatalf("search requests = %d, want 1", got)
	}
	if result.MetaPath != filepath.Join(outDir, "sampleA_ena_meta.json") {
		t.Fatalf("result meta path = %q", result.MetaPath)
	}
	meta, err := os.ReadFile(result.MetaPath)
	if err != nil {
		t.Fatalf("ReadFile(meta) error = %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(meta, &record); err != nil {
		t.Fatalf("Unmarshal(meta) error = %v", err)
	}
	if record["run_accession"] != "ERR123456" || record["library_layout"] != "PAIRED" || record["source"] != string(ichsm.SearchSourceENA) {
		t.Fatalf("metadata = %s", string(meta))
	}
	if _, ok := record["filename"]; ok {
		t.Fatalf("metadata includes faqt manifest field: %s", string(meta))
	}
	if _, ok := record["ERR123456"]; ok {
		t.Fatalf("metadata still has ichsm outer accession dictionary: %s", string(meta))
	}
}

func TestDownloadReadsWritesENAMetadataWhenRequestedWithoutPrefix(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))

	var searchRequests atomic.Int32
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			searchRequests.Add(1)
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq)
			return
		default:
			http.NotFound(w, r)
			return
		}
		fields := r.URL.Query().Get("fields")
		if fields != "ALL" {
			t.Fatalf("fields = %q, want ALL", fields)
		}
		host := strings.TrimPrefix(server.URL, "https://")
		_, _ = w.Write([]byte(`[` +
			`{"run_accession":"ERR123456",` +
			`"library_layout":"SINGLE",` +
			`"center_name":"test-center",` +
			`"fastq_ftp":"` + host + `/ERR123456_1.fastq.gz",` +
			`"fastq_md5":"` + md5Hex(fq) + `",` +
			`"fastq_bytes":"` + intString(len(fq)) + `"}` +
			`]`))
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:     outDir,
		WriteMetadata: true,
		Attempts:      1,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}
	if got := searchRequests.Load(); got != 1 {
		t.Fatalf("search requests = %d, want 1", got)
	}

	wantMeta := filepath.Join(outDir, "ERR123456_ena_meta.json")
	if result.MetaPath != wantMeta {
		t.Fatalf("result meta path = %q, want %q", result.MetaPath, wantMeta)
	}
	meta, err := os.ReadFile(result.MetaPath)
	if err != nil {
		t.Fatalf("ReadFile(meta) error = %v", err)
	}
	var record map[string]any
	if err := json.Unmarshal(meta, &record); err != nil {
		t.Fatalf("Unmarshal(meta) error = %v", err)
	}
	if record["run_accession"] != "ERR123456" || record["library_layout"] != "SINGLE" || record["source"] != string(ichsm.SearchSourceENA) {
		t.Fatalf("metadata = %s", string(meta))
	}
}

func TestDownloadReadsAllowsExistingOutputDirectory(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz","fastq_md5":"` + md5Hex(fq) + `","fastq_bytes":"` + intString(len(fq)) + `"}]`))
		case "/ERR123456_1.fastq.gz":
			_, _ = w.Write(fq)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "existing.txt"), []byte("keep\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir: outDir,
		Attempts:  1,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}
	wantFASTQ := filepath.Join(outDir, "ERR123456_1.fastq.gz")
	if result.Files[0].Path != wantFASTQ {
		t.Fatalf("result file path = %q, want %q", result.Files[0].Path, wantFASTQ)
	}
	if _, err := os.Stat(wantFASTQ); err != nil {
		t.Fatalf("expected FASTQ in existing output directory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "existing.txt")); err != nil {
		t.Fatalf("existing file was not preserved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "ERR123456")); !os.IsNotExist(err) {
		t.Fatalf("run directory stat error = %v, want not exist", err)
	}
}

func TestDownloadReadsFailsBeforeRetryWhenOutputFilesExist(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	var downloadRequests atomic.Int32

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			host := strings.TrimPrefix(server.URL, "https://")
			_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz","fastq_md5":"` + md5Hex(fq) + `","fastq_bytes":"` + intString(len(fq)) + `"}]`))
		case "/ERR123456_1.fastq.gz":
			downloadRequests.Add(1)
			_, _ = w.Write(fq)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, "ERR123456_1.fastq.gz"), fq, 0o644); err != nil {
		t.Fatalf("WriteFile(existing FASTQ) error = %v", err)
	}

	var sleeps atomic.Int32
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
		Sleep: func(ctx context.Context, delay time.Duration) error {
			sleeps.Add(1)
			return nil
		},
	}
	_, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:     outDir,
		Attempts:      3,
		RetryDelayMin: time.Second,
		RetryDelayMax: time.Second,
	})
	if err == nil || err.Error() != "files already exist" {
		t.Fatalf("DownloadReads() error = %v, want files already exist", err)
	}
	if got := downloadRequests.Load(); got != 0 {
		t.Fatalf("download requests = %d, want 0", got)
	}
	if got := sleeps.Load(); got != 0 {
		t.Fatalf("sleeps = %d, want 0", got)
	}
}

func TestDownloadReadsWithSRACHAMissingBinary(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	_, err := DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir: t.TempDir(),
		Methods:   []Method{MethodSRACHA},
		Attempts:  1,
	})
	if err == nil || !strings.Contains(err.Error(), "sracha not found in PATH; install sracha or pass --sracha-bin") {
		t.Fatalf("DownloadReads() error = %v, want missing sracha error", err)
	}
}

func TestDownloadReadsDelaysBetweenFailedAttempts(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))

	var gotDelays []time.Duration
	downloader := &Downloader{
		Sleep: func(ctx context.Context, delay time.Duration) error {
			gotDelays = append(gotDelays, delay)
			return nil
		},
	}
	_, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:     t.TempDir(),
		Methods:       []Method{MethodSRACHA},
		Attempts:      2,
		RetryDelayMin: 7 * time.Second,
		RetryDelayMax: 7 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "sracha not found in PATH") {
		t.Fatalf("DownloadReads() error = %v, want missing sracha", err)
	}
	if !reflect.DeepEqual(gotDelays, []time.Duration{7 * time.Second}) {
		t.Fatalf("delays = %v, want [7s]", gotDelays)
	}
}

func TestDownloadReadsWithSRACHAPath(t *testing.T) {
	fq1 := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	fq2 := gzipBytes(t, []byte("@r2\nGT\n+\n!!\n"))
	srcDir := t.TempDir()
	fq1Src := filepath.Join(srcDir, "fq1.gz")
	fq2Src := filepath.Join(srcDir, "fq2.gz")
	if err := os.WriteFile(fq1Src, fq1, 0o644); err != nil {
		t.Fatalf("WriteFile(fq1) error = %v", err)
	}
	if err := os.WriteFile(fq2Src, fq2, 0o644); err != nil {
		t.Fatalf("WriteFile(fq2) error = %v", err)
	}
	t.Setenv("FQ1_SRC", fq1Src)
	t.Setenv("FQ2_SRC", fq2Src)

	srachaPath := filepath.Join(t.TempDir(), "sracha")
	script := "#!/bin/sh\nfor last do :; done\ncp \"$FQ1_SRC\" \"${last}_1.fastq.gz\" || exit 1\ncp \"$FQ2_SRC\" \"${last}_2.fastq.gz\" || exit 1\n"
	if err := os.WriteFile(srachaPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(sracha) error = %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		host := strings.TrimPrefix(server.URL, "https://")
		_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz;` + host + `/ERR123456_2.fastq.gz","fastq_md5":"abc;def","fastq_bytes":"10;20"}]`))
	}))
	defer server.Close()

	outDir := t.TempDir()
	var progress bytes.Buffer
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:      outDir,
		Methods:        []Method{MethodSRACHA},
		Attempts:       1,
		SrachaPath:     srachaPath,
		ProgressWriter: &progress,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}
	if result.Method != MethodSRACHA {
		t.Fatalf("method = %q, want %q", result.Method, MethodSRACHA)
	}
	for _, filename := range []string{"ERR123456_1.fastq.gz", "ERR123456_2.fastq.gz"} {
		if _, err := os.Stat(filepath.Join(outDir, filename)); err != nil {
			t.Fatalf("expected %s: %v", filename, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "ERR123456_ena_meta.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata stat error = %v, want not exist", err)
	}
	wantCommand := "download-reads: running sracha command: " + srachaPath + " get -t 1 --connections 1 --split split-files ERR123456\n"
	if !strings.Contains(progress.String(), wantCommand) {
		t.Fatalf("progress output missing command %q in:\n%s", wantCommand, progress.String())
	}
}

func TestDownloadReadsWithSRACHASingleEndRun(t *testing.T) {
	fq := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	srcDir := t.TempDir()
	fqSrc := filepath.Join(srcDir, "fq.gz")
	if err := os.WriteFile(fqSrc, fq, 0o644); err != nil {
		t.Fatalf("WriteFile(fq) error = %v", err)
	}
	t.Setenv("FQ_SRC", fqSrc)

	srachaPath := filepath.Join(t.TempDir(), "sracha")
	script := "#!/bin/sh\ncase \" $* \" in *\" --split split-spot \"*) ;; *) echo \"wrong split: $*\" >&2; exit 2;; esac\nfor last do :; done\ncp \"$FQ_SRC\" \"${last}.fastq.gz\" || exit 1\n"
	if err := os.WriteFile(srachaPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(sracha) error = %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		host := strings.TrimPrefix(server.URL, "https://")
		_, _ = w.Write([]byte(`[{"run_accession":"DRR013337","fastq_ftp":"` + host + `/DRR013337_subreads.fastq.gz","fastq_md5":"abc","fastq_bytes":"10"}]`))
	}))
	defer server.Close()

	outDir := t.TempDir()
	var progress bytes.Buffer
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "DRR013337", DownloadOptions{
		OutputDir:      outDir,
		Methods:        []Method{MethodSRACHA},
		Attempts:       1,
		SrachaPath:     srachaPath,
		ProgressWriter: &progress,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}
	if result.Method != MethodSRACHA {
		t.Fatalf("method = %q, want %q", result.Method, MethodSRACHA)
	}
	wantFASTQ := filepath.Join(outDir, "DRR013337_subreads.fastq.gz")
	if result.Files[0].Path != wantFASTQ {
		t.Fatalf("result file path = %q, want %q", result.Files[0].Path, wantFASTQ)
	}
	if _, err := os.Stat(wantFASTQ); err != nil {
		t.Fatalf("expected single-end FASTQ: %v", err)
	}
	wantCommand := "download-reads: running sracha command: " + srachaPath + " get -t 1 --connections 1 --split split-spot DRR013337\n"
	if !strings.Contains(progress.String(), wantCommand) {
		t.Fatalf("progress output missing command %q in:\n%s", wantCommand, progress.String())
	}
}

func TestDownloadReadsWithSRACHAOutputPrefix(t *testing.T) {
	fq1 := gzipBytes(t, []byte("@r1\nAC\n+\n!!\n"))
	fq2 := gzipBytes(t, []byte("@r2\nGT\n+\n!!\n"))
	srcDir := t.TempDir()
	fq1Src := filepath.Join(srcDir, "fq1.gz")
	fq2Src := filepath.Join(srcDir, "fq2.gz")
	if err := os.WriteFile(fq1Src, fq1, 0o644); err != nil {
		t.Fatalf("WriteFile(fq1) error = %v", err)
	}
	if err := os.WriteFile(fq2Src, fq2, 0o644); err != nil {
		t.Fatalf("WriteFile(fq2) error = %v", err)
	}
	t.Setenv("FQ1_SRC", fq1Src)
	t.Setenv("FQ2_SRC", fq2Src)

	srachaPath := filepath.Join(t.TempDir(), "sracha")
	script := "#!/bin/sh\nfor last do :; done\ncp \"$FQ1_SRC\" \"${last}_1.fastq.gz\" || exit 1\ncp \"$FQ2_SRC\" \"${last}_2.fastq.gz\" || exit 1\n"
	if err := os.WriteFile(srachaPath, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(sracha) error = %v", err)
	}

	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			http.NotFound(w, r)
			return
		}
		host := strings.TrimPrefix(server.URL, "https://")
		_, _ = w.Write([]byte(`[{"run_accession":"ERR123456","fastq_ftp":"` + host + `/ERR123456_1.fastq.gz;` + host + `/ERR123456_2.fastq.gz","fastq_md5":"abc;def","fastq_bytes":"10;20"}]`))
	}))
	defer server.Close()

	outDir := t.TempDir()
	downloader := &Downloader{
		ENAClient:  &ichsm.Client{BaseURL: server.URL + "/", HTTPClient: server.Client()},
		HTTPClient: server.Client(),
	}
	result, err := downloader.DownloadReads(context.Background(), "ERR123456", DownloadOptions{
		OutputDir:    outDir,
		OutputPrefix: "sampleA",
		Methods:      []Method{MethodSRACHA},
		Attempts:     1,
		SrachaPath:   srachaPath,
	})
	if err != nil {
		t.Fatalf("DownloadReads() error = %v", err)
	}
	wantDir := outDir
	for _, filename := range []string{"sampleA_1.fastq.gz", "sampleA_2.fastq.gz"} {
		if _, err := os.Stat(filepath.Join(wantDir, filename)); err != nil {
			t.Fatalf("expected prefixed file %s: %v", filename, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "ERR123456")); !os.IsNotExist(err) {
		t.Fatalf("run directory stat error = %v, want not exist", err)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "ERR123456_1.fastq.gz")); !os.IsNotExist(err) {
		t.Fatalf("original filename stat error = %v, want not exist", err)
	}
	if got := result.Files[1].Filename; got != "sampleA_2.fastq.gz" {
		t.Fatalf("result filename = %q, want sampleA_2.fastq.gz", got)
	}
	if result.Files[1].Path != filepath.Join(outDir, "sampleA_2.fastq.gz") {
		t.Fatalf("result file path = %q", result.Files[1].Path)
	}
	if result.MetaPath != "" {
		t.Fatalf("result meta path = %q, want empty", result.MetaPath)
	}
	if _, err := os.Stat(filepath.Join(outDir, "sampleA_ena_meta.json")); !os.IsNotExist(err) {
		t.Fatalf("metadata stat error = %v, want not exist", err)
	}
}

func TestParseMethods(t *testing.T) {
	got, err := ParseMethods("ena,sracha")
	if err != nil {
		t.Fatalf("ParseMethods() error = %v", err)
	}
	if !reflect.DeepEqual(got, []Method{MethodENA, MethodSRACHA}) {
		t.Fatalf("methods = %#v", got)
	}

	_, err = ParseMethods("ena,")
	if err == nil || !strings.Contains(err.Error(), "download methods must be a comma-separated list") {
		t.Fatalf("ParseMethods(empty) error = %v", err)
	}

	_, err = ParseMethods("bad")
	if err == nil || !strings.Contains(err.Error(), `unknown download method "bad"`) {
		t.Fatalf("ParseMethods(bad) error = %v", err)
	}
}

func TestNormalizeRetryDelay(t *testing.T) {
	minDelay, maxDelay, err := normalizeRetryDelay(0, 0)
	if err != nil {
		t.Fatalf("normalizeRetryDelay(default) error = %v", err)
	}
	if minDelay != DefaultRetryDelayMin || maxDelay != DefaultRetryDelayMax {
		t.Fatalf("default retry delay = %s-%s, want %s-%s", minDelay, maxDelay, DefaultRetryDelayMin, DefaultRetryDelayMax)
	}

	_, _, err = normalizeRetryDelay(20*time.Second, 5*time.Second)
	if err == nil || err.Error() != "retry delay max must be greater than or equal to retry delay min" {
		t.Fatalf("normalizeRetryDelay(invalid) error = %v", err)
	}
}

func TestNormalizeDownloadStallTimeout(t *testing.T) {
	got, err := normalizeDownloadStallTimeout(0)
	if err != nil {
		t.Fatalf("normalizeDownloadStallTimeout(default) error = %v", err)
	}
	if got != DefaultDownloadStallTimeout {
		t.Fatalf("default stall timeout = %s, want %s", got, DefaultDownloadStallTimeout)
	}

	_, err = normalizeDownloadStallTimeout(-time.Second)
	if err == nil || err.Error() != "download stall timeout must not be negative" {
		t.Fatalf("normalizeDownloadStallTimeout(invalid) error = %v", err)
	}
}

func TestNormalizeDownloadProgressInterval(t *testing.T) {
	got, err := normalizeDownloadProgressInterval(0)
	if err != nil {
		t.Fatalf("normalizeDownloadProgressInterval(default) error = %v", err)
	}
	if got != DefaultDownloadProgressInterval {
		t.Fatalf("default progress interval = %s, want %s", got, DefaultDownloadProgressInterval)
	}

	_, err = normalizeDownloadProgressInterval(-time.Second)
	if err == nil || err.Error() != "download progress interval must not be negative" {
		t.Fatalf("normalizeDownloadProgressInterval(invalid) error = %v", err)
	}
}

func TestFormatByteCount(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{n: 512, want: "512 B"},
		{n: 1024, want: "1.0 KiB"},
		{n: 5 * 1024 * 1024, want: "5.0 MiB"},
	}
	for _, tt := range tests {
		if got := formatByteCount(tt.n); got != tt.want {
			t.Fatalf("formatByteCount(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

func TestSrachaArgs(t *testing.T) {
	got, err := srachaArgs("ERR123456", DownloadOptions{}, srachaSplitFiles)
	if err != nil {
		t.Fatalf("srachaArgs(default) error = %v", err)
	}
	want := []string{"get", "-t", "1", "--connections", "1", "--split", "split-files", "ERR123456"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default args = %#v, want %#v", got, want)
	}

	got, err = srachaArgs("ERR123456", DownloadOptions{
		SrachaThreads:     4,
		SrachaConnections: 2,
	}, srachaSplitSpot)
	if err != nil {
		t.Fatalf("srachaArgs(custom) error = %v", err)
	}
	want = []string{"get", "-t", "4", "--connections", "2", "--split", "split-spot", "ERR123456"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("custom args = %#v, want %#v", got, want)
	}

	_, err = srachaArgs("ERR123456", DownloadOptions{SrachaThreads: -1}, srachaSplitFiles)
	if err == nil || err.Error() != "sracha threads must be greater than zero" {
		t.Fatalf("srachaArgs(bad threads) error = %v", err)
	}
}

func TestSrachaSplitMode(t *testing.T) {
	tests := []struct {
		name  string
		files []ichsm.ReadFile
		want  string
	}{
		{
			name:  "single bare fastq",
			files: []ichsm.ReadFile{{Filename: "DRR013337.fastq.gz"}},
			want:  srachaSplitSpot,
		},
		{
			name:  "paired",
			files: []ichsm.ReadFile{{Filename: "ERR123456_1.fastq.gz"}, {Filename: "ERR123456_2.fastq.gz"}},
			want:  srachaSplitFiles,
		},
		{
			name:  "paired with unpaired",
			files: []ichsm.ReadFile{{Filename: "ERR123456.fastq.gz"}, {Filename: "ERR123456_1.fastq.gz"}, {Filename: "ERR123456_2.fastq.gz"}},
			want:  srachaSplit3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := srachaSplitMode(tt.files); got != tt.want {
				t.Fatalf("srachaSplitMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExistingSrachaOutputPathAcceptsSplit3UnpairedZeroFallback(t *testing.T) {
	outDir := t.TempDir()
	fallback := filepath.Join(outDir, "ERR123456_0.fastq.gz")
	if err := os.WriteFile(fallback, gzipBytes(t, []byte("@r1\nAC\n+\n!!\n")), 0o644); err != nil {
		t.Fatalf("WriteFile(fallback) error = %v", err)
	}

	got, err := existingSrachaOutputPath(ichsm.ReadFile{
		Filename:   "ERR123456.fastq.gz",
		OutputPath: filepath.Join(outDir, "ERR123456.fastq.gz"),
	}, "ERR123456", 0, 3, srachaSplit3)
	if err != nil {
		t.Fatalf("existingSrachaOutputPath() error = %v", err)
	}
	if got != fallback {
		t.Fatalf("existingSrachaOutputPath() = %q, want %q", got, fallback)
	}
}

func TestApplyOutputPrefix(t *testing.T) {
	files := []ichsm.ReadFile{
		{Filename: "ERR123456_1.fastq.gz", OutputPath: "old1"},
		{Filename: "ERR123456_2.fastq.gz", OutputPath: "old2"},
	}
	got, err := applyOutputPrefix(files, "/tmp/reads", "sampleA")
	if err != nil {
		t.Fatalf("applyOutputPrefix() error = %v", err)
	}
	if got[0].Filename != "sampleA_1.fastq.gz" || got[1].Filename != "sampleA_2.fastq.gz" {
		t.Fatalf("filenames = %q, %q", got[0].Filename, got[1].Filename)
	}
	if got[0].OutputPath != filepath.Join("/tmp/reads", "sampleA_1.fastq.gz") {
		t.Fatalf("output path = %q", got[0].OutputPath)
	}
	if files[0].Filename != "ERR123456_1.fastq.gz" {
		t.Fatalf("input files were mutated: %#v", files)
	}

	got, err = applyOutputPrefix([]ichsm.ReadFile{{Filename: "single.fastq.gz"}}, "/tmp/reads", "sampleA")
	if err != nil {
		t.Fatalf("applyOutputPrefix(single) error = %v", err)
	}
	if got[0].Filename != "sampleA.fastq.gz" {
		t.Fatalf("single filename = %q, want sampleA.fastq.gz", got[0].Filename)
	}

	_, err = applyOutputPrefix(files, "/tmp/reads", "bad/prefix")
	if err == nil || err.Error() != "output prefix must not contain path separators" {
		t.Fatalf("applyOutputPrefix(bad) error = %v", err)
	}
}

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	if _, err := gw.Write(data); err != nil {
		t.Fatalf("gzip Write() error = %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return buf.Bytes()
}

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func intString(value int) string {
	return strconv.Itoa(value)
}
