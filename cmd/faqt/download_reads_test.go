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
	for _, name := range []string{"output-dir", "prefix", "accessions-file", "ena-meta", "method", "attempts", "sracha-bin", "sracha-threads", "sracha-connections", "retry-delay-min", "retry-delay-max", "download-stall-timeout", "verbose"} {
		if found.Flags().Lookup(name) == nil {
			t.Fatalf("download-reads command missing --%s flag", name)
		}
	}
}

func TestDownloadReadsCommandRoutesToDownloader(t *testing.T) {
	old := downloadReads
	defer func() { downloadReads = old }()

	var (
		gotRun  string
		gotOpts readdl.DownloadOptions
	)
	downloadReads = func(ctx context.Context, runAccession string, opts readdl.DownloadOptions) (readdl.Result, error) {
		gotRun = runAccession
		gotOpts = opts
		return readdl.Result{}, nil
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
	if gotRun != "ERR123456" {
		t.Fatalf("run accession = %q, want ERR123456", gotRun)
	}
	if gotOpts.OutputDir != outDir {
		t.Fatalf("output dir = %q, want %q", gotOpts.OutputDir, outDir)
	}
	if gotOpts.OutputPrefix != "sampleA" {
		t.Fatalf("output prefix = %q, want sampleA", gotOpts.OutputPrefix)
	}
	if !gotOpts.WriteMetadata {
		t.Fatal("WriteMetadata = false, want true")
	}
	if !reflect.DeepEqual(gotOpts.Methods, []readdl.Method{readdl.MethodENA, readdl.MethodSRACHA}) {
		t.Fatalf("methods = %#v", gotOpts.Methods)
	}
	if gotOpts.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", gotOpts.Attempts)
	}
	if gotOpts.SrachaPath != "/usr/local/bin/sracha" {
		t.Fatalf("sracha path = %q", gotOpts.SrachaPath)
	}
	if gotOpts.SrachaThreads != 4 {
		t.Fatalf("sracha threads = %d, want 4", gotOpts.SrachaThreads)
	}
	if gotOpts.SrachaConnections != 2 {
		t.Fatalf("sracha connections = %d, want 2", gotOpts.SrachaConnections)
	}
	if gotOpts.RetryDelayMin != time.Second {
		t.Fatalf("retry delay min = %s, want 1s", gotOpts.RetryDelayMin)
	}
	if gotOpts.RetryDelayMax != 3*time.Second {
		t.Fatalf("retry delay max = %s, want 3s", gotOpts.RetryDelayMax)
	}
	if gotOpts.DownloadStallTimeout != 10*time.Minute {
		t.Fatalf("download stall timeout = %s, want 10m", gotOpts.DownloadStallTimeout)
	}
	if gotOpts.ProgressWriter == nil {
		t.Fatal("progress writer = nil, want stderr writer")
	}
}

func TestDownloadReadsCommandRoutesCommaSeparatedRunsToDownloader(t *testing.T) {
	old := downloadReads
	defer func() { downloadReads = old }()

	var gotRuns []string
	downloadReads = func(ctx context.Context, runAccession string, opts readdl.DownloadOptions) (readdl.Result, error) {
		gotRuns = append(gotRuns, runAccession)
		return readdl.Result{}, nil
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
	old := downloadReads
	defer func() { downloadReads = old }()

	var gotRuns []string
	downloadReads = func(ctx context.Context, runAccession string, opts readdl.DownloadOptions) (readdl.Result, error) {
		gotRuns = append(gotRuns, runAccession)
		return readdl.Result{}, nil
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

func TestDownloadReadsCommandUsesRunSpecificPrefixWithMultipleRuns(t *testing.T) {
	old := downloadReads
	defer func() { downloadReads = old }()

	var (
		gotRuns     []string
		gotPrefixes []string
	)
	downloadReads = func(ctx context.Context, runAccession string, opts readdl.DownloadOptions) (readdl.Result, error) {
		gotRuns = append(gotRuns, runAccession)
		gotPrefixes = append(gotPrefixes, opts.OutputPrefix)
		return readdl.Result{}, nil
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
	wantPrefixes := []string{"sampleA_ERR123456", "sampleA_ERR123457"}
	if !reflect.DeepEqual(gotPrefixes, wantPrefixes) {
		t.Fatalf("output prefixes = %#v, want %#v", gotPrefixes, wantPrefixes)
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
			want: "--sracha-threads must be greater than zero",
		},
		{
			name: "connections",
			args: []string{"ERR123456", "--sracha-connections", "0"},
			want: "--sracha-connections must be greater than zero",
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
	if err == nil || err.Error() != "--attempts must be greater than zero" {
		t.Fatalf("Execute() error = %v, want attempts error", err)
	}
}

func TestDownloadReadsCommandRejectsInvalidRetryDelayRange(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456", "--retry-delay-min", "20s", "--retry-delay-max", "5s"})

	err := cmd.Execute()
	if err == nil || err.Error() != "--retry-delay-max must be greater than or equal to --retry-delay-min" {
		t.Fatalf("Execute() error = %v, want retry delay range error", err)
	}
}

func TestDownloadReadsCommandRejectsInvalidDownloadStallTimeout(t *testing.T) {
	cmd := newDownloadReadsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"ERR123456", "--download-stall-timeout=-1s"})

	err := cmd.Execute()
	if err == nil || err.Error() != "--download-stall-timeout must not be negative" {
		t.Fatalf("Execute() error = %v, want download stall timeout error", err)
	}
}

func TestDownloadReadsCommandReturnsDownloadError(t *testing.T) {
	old := downloadReads
	defer func() { downloadReads = old }()

	wantErr := errors.New("download failed")
	downloadReads = func(ctx context.Context, runAccession string, opts readdl.DownloadOptions) (readdl.Result, error) {
		return readdl.Result{}, wantErr
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
