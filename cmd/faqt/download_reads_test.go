package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/martinghunt/faqt/readdl"
)

func TestDownloadReadsCommandExists(t *testing.T) {
	cmd := newRootCmd()
	found, _, err := cmd.Find([]string{"download-reads"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if found == nil || found.Name() != "download-reads" {
		t.Fatalf("unexpected command = %v", found)
	}
	for _, name := range []string{"output-dir", "prefix", "accessions-file", "ena-meta", "ena-meta-single-object", "method", "attempts", "sracha-bin", "sracha-threads", "sracha-connections", "retry-delay-min", "retry-delay-max", "download-stall-timeout", "merge", "keep-originals", "verbose"} {
		if found.Flags().Lookup(name) == nil {
			t.Fatalf("download-reads command missing --%s flag", name)
		}
	}
}

func TestDownloadReadsCommandRoutesToDownloader(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	var (
		gotRuns []string
		gotOpts readdl.DownloadRunsOptions
	)
	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		gotRuns = append([]string(nil), runAccessions...)
		gotOpts = opts
		return readdl.DownloadRunsResult{}, nil
	}

	outDir := filepath.Join(t.TempDir(), "reads")
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{
		"ERR123456",
		"--output-dir", outDir,
		"--prefix", "sampleA",
		"--ena-meta",
		"--ena-meta-single-object",
		"--method", "ena,sracha",
		"--attempts", "2",
		"--sracha-bin", "/usr/local/bin/sracha",
		"--sracha-threads", "4",
		"--sracha-connections", "2",
		"--retry-delay-min", "1s",
		"--retry-delay-max", "3s",
		"--download-stall-timeout", "10m",
		"--verbose",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !reflect.DeepEqual(gotRuns, []string{"ERR123456"}) {
		t.Fatalf("run accessions = %#v, want ERR123456", gotRuns)
	}
	if gotOpts.Download.OutputDir != outDir {
		t.Fatalf("output dir = %q, want %q", gotOpts.Download.OutputDir, outDir)
	}
	if gotOpts.Download.OutputPrefix != "sampleA" {
		t.Fatalf("output prefix = %q, want sampleA", gotOpts.Download.OutputPrefix)
	}
	if !gotOpts.Download.WriteMetadata {
		t.Fatal("WriteMetadata = false, want true")
	}
	if !gotOpts.Download.MetadataSingleObject {
		t.Fatal("MetadataSingleObject = false, want true")
	}
	if !reflect.DeepEqual(gotOpts.Download.Methods, []readdl.Method{readdl.MethodENA, readdl.MethodSRACHA}) {
		t.Fatalf("methods = %#v", gotOpts.Download.Methods)
	}
	if gotOpts.Download.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", gotOpts.Download.Attempts)
	}
	if gotOpts.Download.SrachaPath != "/usr/local/bin/sracha" {
		t.Fatalf("sracha path = %q", gotOpts.Download.SrachaPath)
	}
	if gotOpts.Download.SrachaThreads != 4 {
		t.Fatalf("sracha threads = %d, want 4", gotOpts.Download.SrachaThreads)
	}
	if gotOpts.Download.SrachaConnections != 2 {
		t.Fatalf("sracha connections = %d, want 2", gotOpts.Download.SrachaConnections)
	}
	if gotOpts.Download.RetryDelayMin != time.Second {
		t.Fatalf("retry delay min = %s, want 1s", gotOpts.Download.RetryDelayMin)
	}
	if gotOpts.Download.RetryDelayMax != 3*time.Second {
		t.Fatalf("retry delay max = %s, want 3s", gotOpts.Download.RetryDelayMax)
	}
	if gotOpts.Download.DownloadStallTimeout != 10*time.Minute {
		t.Fatalf("download stall timeout = %s, want 10m", gotOpts.Download.DownloadStallTimeout)
	}
	if gotOpts.Download.ProgressWriter == nil {
		t.Fatal("progress writer = nil, want stderr writer")
	}
}

func TestDownloadReadsCommandRoutesCommaSeparatedRunsToDownloader(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	var gotRuns []string
	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		gotRuns = append([]string(nil), runAccessions...)
		return readdl.DownloadRunsResult{}, nil
	}

	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456, ERR123457"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantRuns := []string{"ERR123456", "ERR123457"}
	if !reflect.DeepEqual(gotRuns, wantRuns) {
		t.Fatalf("run accessions = %#v, want %#v", gotRuns, wantRuns)
	}
}

func TestDownloadReadsCommandRoutesAccessionsFileToDownloader(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	var gotRuns []string
	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		gotRuns = append([]string(nil), runAccessions...)
		return readdl.DownloadRunsResult{}, nil
	}

	path := filepath.Join(t.TempDir(), "runs.txt")
	if err := os.WriteFile(path, []byte("ERR123456\n\n ERR123457 \n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"--accessions-file", path})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantRuns := []string{"ERR123456", "ERR123457"}
	if !reflect.DeepEqual(gotRuns, wantRuns) {
		t.Fatalf("run accessions = %#v, want %#v", gotRuns, wantRuns)
	}
}

func TestDownloadReadsCommandPassesBasePrefixToDownloader(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	var (
		gotRuns   []string
		gotPrefix string
	)
	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		gotRuns = append([]string(nil), runAccessions...)
		gotPrefix = opts.Download.OutputPrefix
		return readdl.DownloadRunsResult{}, nil
	}

	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456,ERR123457", "--prefix", "sampleA"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantRuns := []string{"ERR123456", "ERR123457"}
	if !reflect.DeepEqual(gotRuns, wantRuns) {
		t.Fatalf("run accessions = %#v, want %#v", gotRuns, wantRuns)
	}
	if gotPrefix != "sampleA" {
		t.Fatalf("output prefix = %q, want sampleA", gotPrefix)
	}
}

func TestDownloadReadsCommandIgnoresSingleObjectMetadataWhenMerging(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		if opts.Download.MetadataSingleObject {
			t.Fatal("MetadataSingleObject = true with --merge, want false")
		}
		if !opts.Merge {
			t.Fatal("Merge = false, want true")
		}
		return readdl.DownloadRunsResult{}, nil
	}

	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456,ERR123457", "--ena-meta", "--ena-meta-single-object", "--merge"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestDownloadReadsCommandPassesMergeOptions(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	var gotOpts readdl.DownloadRunsOptions
	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		gotOpts = opts
		return readdl.DownloadRunsResult{}, nil
	}

	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456,ERR123457", "--prefix", "sampleA", "--merge", "--keep-originals"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !gotOpts.Merge {
		t.Fatal("Merge = false, want true")
	}
	if !gotOpts.KeepOriginals {
		t.Fatal("KeepOriginals = false, want true")
	}
}

func TestDownloadReadsCommandRejectsEmptyRunInCommaSeparatedList(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456,"})

	err := cmd.Execute()
	if err == nil || err.Error() != "download-reads accession list contains an empty run accession" {
		t.Fatalf("Execute() error = %v, want empty accession error", err)
	}
}

func TestDownloadReadsCommandRejectsInvalidSrachaOptions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "threads",
			args: []string{"ERR123456", "--sracha-threads", "0"},
			want: "sracha threads must be greater than zero",
		},
		{
			name: "connections",
			args: []string{"ERR123456", "--sracha-connections", "0"},
			want: "sracha connections must be greater than zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newDownloadReadsCmd()
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

func TestDownloadReadsCommandRejectsUnknownMethod(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456", "--method", "bad"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown download method "bad"`) {
		t.Fatalf("Execute() error = %v, want unknown method", err)
	}
}

func TestDownloadReadsCommandRejectsNonPositiveAttempts(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456", "--attempts", "0"})

	err := cmd.Execute()
	if err == nil || err.Error() != "attempts must be greater than zero" {
		t.Fatalf("Execute() error = %v, want attempts error", err)
	}
}

func TestDownloadReadsCommandRejectsInvalidRetryDelayRange(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456", "--retry-delay-min", "20s", "--retry-delay-max", "5s"})

	err := cmd.Execute()
	if err == nil || err.Error() != "retry delay max must be greater than or equal to retry delay min" {
		t.Fatalf("Execute() error = %v, want retry delay range error", err)
	}
}

func TestDownloadReadsCommandRejectsInvalidDownloadStallTimeout(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456", "--download-stall-timeout=-1s"})

	err := cmd.Execute()
	if err == nil || err.Error() != "download stall timeout must not be negative" {
		t.Fatalf("Execute() error = %v, want download stall timeout error", err)
	}
}

func TestDownloadReadsCommandReturnsDownloadError(t *testing.T) {
	old := downloadReadRuns
	defer func() { downloadReadRuns = old }()

	wantErr := errors.New("download failed")
	downloadReadRuns = func(ctx context.Context, runAccessions []string, opts readdl.DownloadRunsOptions) (readdl.DownloadRunsResult, error) {
		return readdl.DownloadRunsResult{}, wantErr
	}

	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456"})

	err := cmd.Execute()
	if !errors.Is(err, wantErr) {
		t.Fatalf("Execute() error = %v, want %v", err, wantErr)
	}
}
