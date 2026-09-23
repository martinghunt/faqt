package readdl

import (
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/martinghunt/ichsm"
)

const (
	DefaultAttempts                 = 3
	DefaultRetryDelayMin            = 5 * time.Second
	DefaultRetryDelayMax            = 20 * time.Second
	DefaultDownloadStallTimeout     = 5 * time.Minute
	DefaultDownloadProgressInterval = 30 * time.Second
	DefaultSrachaThreads            = 1
	DefaultSrachaConnections        = 1
	srachaSplit3                    = "split-3"
	srachaSplitFiles                = "split-files"
	srachaSplitSpot                 = "split-spot"
)

var errFilesAlreadyExist = errors.New("files already exist")
var errDownloadStalled = errors.New("download stalled")

type Method string

const (
	MethodENA    Method = "ena"
	MethodSRACHA Method = "sracha"
)

type DownloadOptions struct {
	OutputDir                string
	OutputPrefix             string
	WriteMetadata            bool
	MetadataSingleObject     bool
	Methods                  []Method
	Attempts                 int
	SrachaPath               string
	SrachaThreads            int
	SrachaConnections        int
	Protocol                 string
	RetryDelayMin            time.Duration
	RetryDelayMax            time.Duration
	DownloadStallTimeout     time.Duration
	DownloadProgressInterval time.Duration
	ProgressWriter           io.Writer
}

type DownloadedFile struct {
	Filename string
	Path     string
	URL      string
	MD5      string
	Bytes    string
}

type Result struct {
	RunAccession string
	Dir          string
	Method       Method
	MetaPath     string
	Files        []DownloadedFile
}

type MergeOptions struct {
	OutputDir     string
	OutputPrefix  string
	KeepOriginals bool
}

// DownloadRunsOptions controls a group of read downloads and their optional merge.
type DownloadRunsOptions struct {
	Download      DownloadOptions
	Merge         bool
	KeepOriginals bool
}

// DownloadRunsResult contains the individual run results and any merged files.
type DownloadRunsResult struct {
	Runs   []Result
	Merged []DownloadedFile
}

type Downloader struct {
	ENAClient  *ichsm.Client
	HTTPClient *http.Client
	Sleep      func(context.Context, time.Duration) error
}

type readManifest struct {
	Files    []ichsm.ReadFile
	Metadata ichsm.Record
}

func NewDownloader() *Downloader {
	return &Downloader{
		ENAClient:  ichsm.NewClient(),
		HTTPClient: &http.Client{},
	}
}

func DownloadReads(ctx context.Context, runAccession string, opts DownloadOptions) (Result, error) {
	return NewDownloader().DownloadReads(ctx, runAccession, opts)
}

// DownloadRuns downloads a group of runs and optionally merges like read files.
func DownloadRuns(ctx context.Context, runAccessions []string, opts DownloadRunsOptions) (DownloadRunsResult, error) {
	return NewDownloader().DownloadRuns(ctx, runAccessions, opts)
}

func (d *Downloader) DownloadRuns(ctx context.Context, runAccessions []string, opts DownloadRunsOptions) (DownloadRunsResult, error) {
	return downloadRuns(ctx, runAccessions, opts, d.DownloadReads, MergeResults)
}

func downloadRuns(
	ctx context.Context,
	runAccessions []string,
	opts DownloadRunsOptions,
	download func(context.Context, string, DownloadOptions) (Result, error),
	merge func(context.Context, []Result, MergeOptions) ([]DownloadedFile, error),
) (DownloadRunsResult, error) {
	if ctx == nil {
		return DownloadRunsResult{}, fmt.Errorf("nil context")
	}
	if len(runAccessions) == 0 {
		return DownloadRunsResult{}, fmt.Errorf("no run accessions")
	}
	downloadOpts, err := normalizeDownloadRunsOptions(opts.Download)
	if err != nil {
		return DownloadRunsResult{}, err
	}
	runs := make([]string, len(runAccessions))
	for index, accession := range runAccessions {
		run, err := cleanRunAccession(accession)
		if err != nil {
			return DownloadRunsResult{}, err
		}
		runs[index] = run
	}
	result := DownloadRunsResult{Runs: make([]Result, 0, len(runs))}
	for _, run := range runs {
		runOpts := downloadOpts
		if runOpts.OutputPrefix != "" && len(runs) > 1 {
			runOpts.OutputPrefix += "_" + run
		}
		runResult, err := download(ctx, run, runOpts)
		if err != nil {
			return DownloadRunsResult{}, err
		}
		result.Runs = append(result.Runs, runResult)
	}
	if opts.Merge && len(result.Runs) > 1 {
		result.Merged, err = merge(ctx, result.Runs, MergeOptions{
			OutputDir:     downloadOpts.OutputDir,
			OutputPrefix:  downloadOpts.OutputPrefix,
			KeepOriginals: opts.KeepOriginals,
		})
		if err != nil {
			return DownloadRunsResult{}, err
		}
	}
	return result, nil
}

// MergeResults concatenates the FASTQ files from multiple run download results.
// Files are grouped by the read they hold rather than by their position in each
// run's file list, so read 1 is only ever concatenated onto read 1, and within
// each output the runs are concatenated in the order given. Every run must hold
// the same numbered reads; unpaired reads are the exception, since ENA lists an
// unpaired file for only some runs of a sample, and those merge into their own
// output. The outputs use opts.OutputPrefix (or "merged" when it is empty) plus
// the read number, preserving the FASTQ extension: <prefix>_1, <prefix>_2 and,
// for unpaired reads, <prefix>. Source FASTQs are removed after a successful
// merge unless opts.KeepOriginals is true.
func MergeResults(ctx context.Context, results []Result, opts MergeOptions) ([]DownloadedFile, error) {
	if len(results) < 2 {
		return nil, fmt.Errorf("merging reads requires at least two run results")
	}
	root := strings.TrimSpace(opts.OutputDir)
	if root == "" {
		root = "."
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	prefix := strings.TrimSpace(opts.OutputPrefix)
	if prefix == "" {
		prefix = "merged"
	}
	if strings.ContainsAny(prefix, `/\\`) {
		return nil, fmt.Errorf("output prefix must not contain path separators")
	}

	groups, err := groupFilesByRole(results)
	if err != nil {
		return nil, err
	}

	merged := make([]DownloadedFile, len(groups))
	for index, group := range groups {
		ext := readFileExtension(group.files[0].Filename)
		if ext == "" {
			return nil, fmt.Errorf("cannot determine FASTQ extension for %s", group.files[0].Filename)
		}
		for _, file := range group.files[1:] {
			if got := readFileExtension(file.Filename); got != ext {
				return nil, fmt.Errorf("cannot merge FASTQ files with different extensions")
			}
		}
		filename := prefix + group.role + ext
		merged[index] = DownloadedFile{Filename: filename, Path: filepath.Join(root, filename)}
	}
	if err := ensureMergedOutputsDoNotExist(merged); err != nil {
		return nil, err
	}
	metadataTemporaryPath, metadataPath, err := prepareMergedMetadata(results, root, prefix)
	if err != nil {
		return nil, err
	}
	if metadataTemporaryPath != "" {
		defer func() { _ = os.Remove(metadataTemporaryPath) }()
	}

	temporaryPaths := make([]string, len(merged))
	for index := range merged {
		tmp, err := os.CreateTemp(root, "."+merged[index].Filename+"-merge-*")
		if err != nil {
			removeFiles(temporaryPaths)
			return nil, err
		}
		temporaryPaths[index] = tmp.Name()
		for _, file := range groups[index].files {
			if err := ctx.Err(); err != nil {
				_ = tmp.Close()
				removeFiles(temporaryPaths)
				return nil, err
			}
			in, err := os.Open(file.Path)
			if err != nil {
				_ = tmp.Close()
				removeFiles(temporaryPaths)
				return nil, err
			}
			_, copyErr := io.Copy(tmp, in)
			closeErr := in.Close()
			if copyErr != nil || closeErr != nil {
				_ = tmp.Close()
				removeFiles(temporaryPaths)
				if copyErr != nil {
					return nil, copyErr
				}
				return nil, closeErr
			}
		}
		if err := tmp.Close(); err != nil {
			removeFiles(temporaryPaths)
			return nil, err
		}
	}
	moves := make([]fileMove, 0, len(temporaryPaths)+1)
	for index, temporaryPath := range temporaryPaths {
		moves = append(moves, fileMove{source: temporaryPath, target: merged[index].Path})
	}
	if metadataTemporaryPath != "" {
		moves = append(moves, fileMove{source: metadataTemporaryPath, target: metadataPath})
	}
	if err := publishFileMoves(moves); err != nil {
		removeFiles(temporaryPaths)
		return nil, err
	}
	if !opts.KeepOriginals {
		if err := removeResultFiles(results); err != nil {
			return nil, err
		}
	}
	return merged, nil
}

func ParseMethods(value string) ([]Method, error) {
	parts := strings.Split(value, ",")
	methods := make([]Method, 0, len(parts))
	for _, part := range parts {
		method := Method(strings.ToLower(strings.TrimSpace(part)))
		if method == "" {
			return nil, fmt.Errorf("download methods must be a comma-separated list of: %s", strings.Join(allowedMethodStrings(), ","))
		}
		switch method {
		case MethodENA, MethodSRACHA:
			methods = append(methods, method)
		default:
			return nil, fmt.Errorf("unknown download method %q; allowed methods: %s", part, strings.Join(allowedMethodStrings(), ","))
		}
	}
	return methods, nil
}

func (d *Downloader) DownloadReads(ctx context.Context, runAccession string, opts DownloadOptions) (Result, error) {
	if ctx == nil {
		return Result{}, fmt.Errorf("nil context")
	}
	run, err := cleanRunAccession(runAccession)
	if err != nil {
		return Result{}, err
	}
	opts, err = normalizeDownloadOptions(opts)
	if err != nil {
		return Result{}, err
	}
	root := strings.TrimSpace(opts.OutputDir)
	if root == "" {
		root = "."
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Result{}, err
	}

	var lastErr error
	for attempt := 1; attempt <= opts.Attempts; attempt++ {
		for methodIndex, method := range opts.Methods {
			progressf(opts.ProgressWriter, "attempt %d/%d using %s", attempt, opts.Attempts, method)
			result, err := d.downloadAttempt(ctx, run, root, opts, method)
			if err == nil {
				progressf(opts.ProgressWriter, "wrote %d FASTQ file(s) to %s", len(result.Files), result.Dir)
				return result, nil
			}
			if errors.Is(err, errFilesAlreadyExist) {
				return Result{}, errFilesAlreadyExist
			}
			progressf(opts.ProgressWriter, "%s attempt failed: %v", method, err)
			lastErr = err
			if attempt < opts.Attempts || methodIndex < len(opts.Methods)-1 {
				delay := randomRetryDelay(opts.RetryDelayMin, opts.RetryDelayMax)
				if delay > 0 {
					progressf(opts.ProgressWriter, "waiting %s before next attempt", delay)
					if err := d.sleep(ctx, delay); err != nil {
						return Result{}, err
					}
				}
			}
		}
	}
	return Result{}, fmt.Errorf("%d rounds of download attempts failed for methods: %s: %w", opts.Attempts, methodList(opts.Methods), lastErr)
}

func (d *Downloader) downloadAttempt(ctx context.Context, run, root string, opts DownloadOptions, method Method) (Result, error) {
	finalDir := root
	tmpDir, err := os.MkdirTemp(root, "."+run+"-download-*")
	if err != nil {
		return Result{}, err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	var result Result
	switch method {
	case MethodENA:
		result, err = d.downloadWithENA(ctx, run, tmpDir, finalDir, opts)
	case MethodSRACHA:
		result, err = d.downloadWithSRACHA(ctx, run, tmpDir, finalDir, opts)
	default:
		err = fmt.Errorf("unknown download method %q", method)
	}
	if err != nil {
		return Result{}, err
	}
	if info, err := os.Stat(finalDir); err == nil && !info.IsDir() {
		return Result{}, fmt.Errorf("output path exists and is not a directory: %s", finalDir)
	} else if err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	if err := os.MkdirAll(finalDir, 0o755); err != nil {
		return Result{}, err
	}
	if err := moveResultFiles(result, finalDir); err != nil {
		return Result{}, err
	}
	if err := os.RemoveAll(tmpDir); err != nil {
		return Result{}, err
	}
	success = true
	return resultInFinalDir(result, finalDir), nil
}

func (d *Downloader) downloadWithENA(ctx context.Context, run, outDir, finalDir string, opts DownloadOptions) (Result, error) {
	manifest, err := d.readManifest(ctx, run, outDir, opts)
	if err != nil {
		return Result{}, err
	}
	files, err := applyOutputPrefix(manifest.Files, outDir, opts.OutputPrefix)
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, fmt.Errorf("no FASTQ files found for %s", run)
	}
	if err := ensureNoExistingOutputs(files, finalDir, run, opts.OutputPrefix, opts.WriteMetadata); err != nil {
		return Result{}, err
	}
	var metaPath string
	if opts.WriteMetadata {
		progressf(opts.ProgressWriter, "writing ENA metadata to %s", filepath.Join(outDir, metadataFilename(run, opts.OutputPrefix)))
		metaPath, err = writeENAMetadata(outDir, run, opts.OutputPrefix, manifest.Metadata, opts.MetadataSingleObject)
		if err != nil {
			return Result{}, err
		}
	}

	for _, file := range files {
		if file.MD5 == "" {
			return Result{}, fmt.Errorf("missing MD5 checksum for %s", file.URL)
		}
		progressf(opts.ProgressWriter, "downloading %s", file.Filename)
		downloadProgress := newDownloadProgressReporter(opts.ProgressWriter, file.Filename, parseByteCount(file.Bytes), opts.DownloadProgressInterval)
		got, err := d.downloadURLToFileAndValidate(ctx, file.URL, file.OutputPath, opts.DownloadStallTimeout, downloadProgress)
		if err != nil {
			return Result{}, err
		}
		if got != strings.ToLower(file.MD5) {
			return Result{}, fmt.Errorf("md5 mismatch for %s: expected %s, got %s", file.OutputPath, file.MD5, got)
		}
		progressf(opts.ProgressWriter, "validated %s", file.Filename)
	}

	return Result{
		RunAccession: run,
		Dir:          outDir,
		Method:       MethodENA,
		MetaPath:     metaPath,
		Files:        downloadedFiles(files),
	}, nil
}

func (d *Downloader) downloadWithSRACHA(ctx context.Context, run, outDir, finalDir string, opts DownloadOptions) (Result, error) {
	bin, err := resolveSracha(opts.SrachaPath)
	if err != nil {
		return Result{}, err
	}
	progressf(opts.ProgressWriter, "using sracha: %s", bin)
	manifest, err := d.readManifest(ctx, run, outDir, opts)
	if err != nil {
		return Result{}, err
	}
	srachaFiles := manifest.Files
	files, err := applyOutputPrefix(srachaFiles, outDir, opts.OutputPrefix)
	if err != nil {
		return Result{}, err
	}
	if len(files) == 0 {
		return Result{}, fmt.Errorf("no FASTQ files found for %s", run)
	}
	if err := ensureNoExistingOutputs(files, finalDir, run, opts.OutputPrefix, opts.WriteMetadata); err != nil {
		return Result{}, err
	}
	var metaPath string
	if opts.WriteMetadata {
		progressf(opts.ProgressWriter, "writing ENA metadata to %s", filepath.Join(outDir, metadataFilename(run, opts.OutputPrefix)))
		metaPath, err = writeENAMetadata(outDir, run, opts.OutputPrefix, manifest.Metadata, opts.MetadataSingleObject)
		if err != nil {
			return Result{}, err
		}
	}

	splitMode := srachaSplitMode(srachaFiles)
	args, err := srachaArgs(run, opts, splitMode)
	if err != nil {
		return Result{}, err
	}
	progressf(opts.ProgressWriter, "running sracha command: %s", commandString(bin, args))
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = outDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg != "" {
			return Result{}, fmt.Errorf("sracha failed: %w: %s", err, msg)
		}
		return Result{}, fmt.Errorf("sracha failed: %w", err)
	}

	for i, file := range files {
		srachaPath, err := existingSrachaOutputPath(srachaFiles[i], run, i, len(srachaFiles), splitMode)
		if err != nil {
			return Result{}, err
		}
		progressf(opts.ProgressWriter, "validating %s", filepath.Base(srachaPath))
		if err := validateGzip(srachaPath); err != nil {
			return Result{}, fmt.Errorf("gzip validation failed for %s: %w", srachaPath, err)
		}
		if srachaPath != file.OutputPath {
			if err := os.Rename(srachaPath, file.OutputPath); err != nil {
				return Result{}, err
			}
		}
	}

	return Result{
		RunAccession: run,
		Dir:          outDir,
		Method:       MethodSRACHA,
		MetaPath:     metaPath,
		Files:        downloadedFiles(files),
	}, nil
}

func (d *Downloader) readManifest(ctx context.Context, run, outDir string, opts DownloadOptions) (readManifest, error) {
	if !opts.WriteMetadata {
		progressf(opts.ProgressWriter, "querying ENA read files for %s", run)
		files, err := d.readFiles(ctx, run, outDir, opts.Protocol)
		if err != nil {
			return readManifest{}, err
		}
		if err := validateReadFilenames(files); err != nil {
			return readManifest{}, err
		}
		progressf(opts.ProgressWriter, "found %d FASTQ file(s)", len(files))
		return readManifest{Files: files}, nil
	}

	progressf(opts.ProgressWriter, "querying ENA metadata for %s", run)
	results, err := d.enaClient().Search(ctx, ichsm.SearchOptions{
		Accessions: []string{run},
		Fields:     []string{"ALL"},
		Level:      ichsm.AccessionTypeRun,
		Source:     ichsm.SearchSourceENA,
	})
	if err != nil {
		return readManifest{}, err
	}
	record, err := singleMetadataRecord(results, run)
	if err != nil {
		return readManifest{}, err
	}
	files, err := ichsm.ReadFilesFromSearchResults(results, ichsm.ReadFileOptions{
		Accessions: []string{run},
		Protocol:   opts.Protocol,
		OutputDir:  outDir,
	})
	if err != nil {
		return readManifest{}, err
	}
	if err := validateReadFilenames(files); err != nil {
		return readManifest{}, err
	}
	progressf(opts.ProgressWriter, "found %d FASTQ file(s)", len(files))
	return readManifest{Files: files, Metadata: record}, nil
}

// validateReadFilenames rejects FASTQ filenames from ENA metadata that could
// escape the download directory when joined into an output path. Filenames
// normally come from the last path segment of an ENA FTP URL, but faqt
// doesn't rely on that being safe on every OS (e.g. a backslash isn't a path
// separator to ichsm's Unix-style URL parsing, but is one in filepath.Join on
// Windows), so it validates independently before any path is built from it.
func validateReadFilenames(files []ichsm.ReadFile) error {
	for _, file := range files {
		if err := validateSafeFilename(file.Filename); err != nil {
			return fmt.Errorf("refusing to use FASTQ filename from ENA metadata: %w", err)
		}
	}
	return nil
}

func validateSafeFilename(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid filename %q", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("filename %q must not contain path separators", name)
	}
	return nil
}

func (d *Downloader) readFiles(ctx context.Context, run, outDir, protocol string) ([]ichsm.ReadFile, error) {
	return d.enaClient().ReadFiles(ctx, ichsm.ReadFileOptions{
		Accessions: []string{run},
		Protocol:   protocol,
		OutputDir:  outDir,
	})
}

func (d *Downloader) enaClient() *ichsm.Client {
	if d != nil && d.ENAClient != nil {
		return d.ENAClient
	}
	return ichsm.NewClient()
}

func (d *Downloader) downloadURLToFileAndValidate(ctx context.Context, rawURL, outPath string, stallTime time.Duration, progress *downloadProgressReporter) (md5sum string, err error) {
	reqCtx := ctx
	var cancel context.CancelFunc
	var watchdog *downloadStallWatchdog
	if stallTime > 0 {
		reqCtx, cancel = context.WithCancel(ctx)
		defer cancel()
		watchdog = newDownloadStallWatchdog(stallTime, cancel)
		defer watchdog.stop()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.httpClient().Do(req)
	if err != nil {
		if stalledErr := watchdog.err(rawURL); stalledErr != nil {
			return "", stalledErr
		}
		return "", err
	}
	defer closeutil.CloseWithError(&err, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed for %s: %s", rawURL, resp.Status)
	}
	if progress != nil && progress.totalBytes == 0 && resp.ContentLength > 0 {
		progress.totalBytes = resp.ContentLength
	}

	out, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer closeutil.CloseWithError(&err, out)

	hash := md5.New()
	var body io.Reader = resp.Body
	if watchdog != nil || progress != nil {
		body = progressReader{
			Reader: resp.Body,
			progress: func(n int) {
				if watchdog != nil {
					watchdog.progress()
				}
				if progress != nil {
					progress.add(n)
				}
			},
		}
	}
	tee := io.TeeReader(body, io.MultiWriter(out, hash))
	gr, err := gzip.NewReader(tee)
	if err != nil {
		if stalledErr := watchdog.err(rawURL); stalledErr != nil {
			return "", stalledErr
		}
		return "", fmt.Errorf("gzip validation failed for %s: %w", outPath, err)
	}
	defer closeutil.CloseWithError(&err, gr)

	if _, err := io.Copy(io.Discard, gr); err != nil {
		if stalledErr := watchdog.err(rawURL); stalledErr != nil {
			return "", stalledErr
		}
		return "", fmt.Errorf("gzip validation failed for %s: %w", outPath, err)
	}
	if progress != nil {
		progress.done()
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type progressReader struct {
	io.Reader
	progress func(int)
}

func (r progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if n > 0 && r.progress != nil {
		r.progress(n)
	}
	return n, err
}

type downloadProgressReporter struct {
	writer        io.Writer
	filename      string
	totalBytes    int64
	interval      time.Duration
	started       time.Time
	lastReport    time.Time
	bytesRead     int64
	reportedBytes int64
}

func newDownloadProgressReporter(w io.Writer, filename string, totalBytes int64, interval time.Duration) *downloadProgressReporter {
	if w == nil {
		return nil
	}
	now := time.Now()
	return &downloadProgressReporter{
		writer:     w,
		filename:   filename,
		totalBytes: totalBytes,
		interval:   interval,
		started:    now,
		lastReport: now,
	}
}

func (p *downloadProgressReporter) add(n int) {
	p.bytesRead += int64(n)
	now := time.Now()
	if now.Sub(p.lastReport) < p.interval {
		return
	}
	p.report(now)
}

func (p *downloadProgressReporter) done() {
	if p.bytesRead == 0 || p.reportedBytes == p.bytesRead {
		return
	}
	p.report(time.Now())
}

func (p *downloadProgressReporter) report(now time.Time) {
	p.lastReport = now
	p.reportedBytes = p.bytesRead
	elapsed := now.Sub(p.started).Seconds()
	if elapsed <= 0 {
		elapsed = 1
	}
	rate := int64(float64(p.bytesRead) / elapsed)
	if p.totalBytes > 0 {
		percent := 100 * float64(p.bytesRead) / float64(p.totalBytes)
		progressf(p.writer, "downloaded %s: %s/%s (%.1f%%, %s/s)", p.filename, formatByteCount(p.bytesRead), formatByteCount(p.totalBytes), percent, formatByteCount(rate))
		return
	}
	progressf(p.writer, "downloaded %s: %s (%s/s)", p.filename, formatByteCount(p.bytesRead), formatByteCount(rate))
}

type downloadStallWatchdog struct {
	timeout time.Duration
	timer   *time.Timer
	stalled atomic.Bool
}

func newDownloadStallWatchdog(timeout time.Duration, cancel context.CancelFunc) *downloadStallWatchdog {
	w := &downloadStallWatchdog{timeout: timeout}
	w.timer = time.AfterFunc(timeout, func() {
		w.stalled.Store(true)
		cancel()
	})
	return w
}

func (w *downloadStallWatchdog) progress() {
	w.timer.Reset(w.timeout)
}

func (w *downloadStallWatchdog) stop() {
	w.timer.Stop()
}

func (w *downloadStallWatchdog) err(rawURL string) error {
	if w != nil && w.stalled.Load() {
		return fmt.Errorf("download stalled for %s while reading %s: %w", w.timeout, rawURL, errDownloadStalled)
	}
	return nil
}

func cleanRunAccession(accession string) (string, error) {
	fixed, typ, ok := ichsm.IdentifyAccession(accession)
	if !ok || typ != ichsm.AccessionTypeRun {
		return "", fmt.Errorf("download-reads requires a run accession")
	}
	return fixed, nil
}

func normalizeMethods(methods []Method) ([]Method, error) {
	if len(methods) == 0 {
		return []Method{MethodENA}, nil
	}
	out := make([]Method, 0, len(methods))
	for _, method := range methods {
		parsed, err := ParseMethods(string(method))
		if err != nil {
			return nil, err
		}
		out = append(out, parsed...)
	}
	return out, nil
}

func normalizeDownloadOptions(opts DownloadOptions) (DownloadOptions, error) {
	var err error
	opts.Methods, err = normalizeMethods(opts.Methods)
	if err != nil {
		return DownloadOptions{}, err
	}
	opts.Attempts, err = normalizeAttempts(opts.Attempts)
	if err != nil {
		return DownloadOptions{}, err
	}
	opts.RetryDelayMin, opts.RetryDelayMax, err = normalizeRetryDelay(opts.RetryDelayMin, opts.RetryDelayMax)
	if err != nil {
		return DownloadOptions{}, err
	}
	opts.DownloadStallTimeout, err = normalizeDownloadStallTimeout(opts.DownloadStallTimeout)
	if err != nil {
		return DownloadOptions{}, err
	}
	opts.DownloadProgressInterval, err = normalizeDownloadProgressInterval(opts.DownloadProgressInterval)
	if err != nil {
		return DownloadOptions{}, err
	}
	opts.SrachaThreads, err = normalizePositiveDefault(opts.SrachaThreads, DefaultSrachaThreads, "sracha threads")
	if err != nil {
		return DownloadOptions{}, err
	}
	opts.SrachaConnections, err = normalizePositiveDefault(opts.SrachaConnections, DefaultSrachaConnections, "sracha connections")
	if err != nil {
		return DownloadOptions{}, err
	}
	return opts, nil
}

func normalizeDownloadRunsOptions(opts DownloadOptions) (DownloadOptions, error) {
	if opts.Attempts <= 0 {
		return DownloadOptions{}, fmt.Errorf("attempts must be greater than zero")
	}
	if opts.SrachaThreads <= 0 {
		return DownloadOptions{}, fmt.Errorf("sracha threads must be greater than zero")
	}
	if opts.SrachaConnections <= 0 {
		return DownloadOptions{}, fmt.Errorf("sracha connections must be greater than zero")
	}
	return normalizeDownloadOptions(opts)
}

func normalizeAttempts(attempts int) (int, error) {
	if attempts == 0 {
		return DefaultAttempts, nil
	}
	if attempts < 0 {
		return 0, fmt.Errorf("attempts must be greater than zero")
	}
	return attempts, nil
}

func normalizeRetryDelay(minDelay, maxDelay time.Duration) (time.Duration, time.Duration, error) {
	if minDelay == 0 && maxDelay == 0 {
		return DefaultRetryDelayMin, DefaultRetryDelayMax, nil
	}
	if minDelay < 0 || maxDelay < 0 {
		return 0, 0, fmt.Errorf("retry delay must not be negative")
	}
	if maxDelay < minDelay {
		return 0, 0, fmt.Errorf("retry delay max must be greater than or equal to retry delay min")
	}
	return minDelay, maxDelay, nil
}

func normalizeDownloadStallTimeout(stallTimeout time.Duration) (time.Duration, error) {
	if stallTimeout == 0 {
		return DefaultDownloadStallTimeout, nil
	}
	if stallTimeout < 0 {
		return 0, fmt.Errorf("download stall timeout must not be negative")
	}
	return stallTimeout, nil
}

func normalizeDownloadProgressInterval(interval time.Duration) (time.Duration, error) {
	if interval == 0 {
		return DefaultDownloadProgressInterval, nil
	}
	if interval < 0 {
		return 0, fmt.Errorf("download progress interval must not be negative")
	}
	return interval, nil
}

func srachaArgs(run string, opts DownloadOptions, splitMode string) ([]string, error) {
	threads, err := normalizePositiveDefault(opts.SrachaThreads, DefaultSrachaThreads, "sracha threads")
	if err != nil {
		return nil, err
	}
	connections, err := normalizePositiveDefault(opts.SrachaConnections, DefaultSrachaConnections, "sracha connections")
	if err != nil {
		return nil, err
	}

	return []string{
		"get",
		"-t", strconv.Itoa(threads),
		"--connections", strconv.Itoa(connections),
		"--split", splitMode,
		run,
	}, nil
}

func srachaSplitMode(files []ichsm.ReadFile) string {
	if len(files) == 1 && isBareReadFilename(files[0].Filename) {
		return srachaSplitSpot
	}
	if hasBareReadFile(files) {
		return srachaSplit3
	}
	return srachaSplitFiles
}

func existingSrachaOutputPath(file ichsm.ReadFile, run string, index, count int, splitMode string) (string, error) {
	candidates := srachaOutputPathCandidates(file, run, index, count, splitMode)
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("sracha did not create expected FASTQ file: %s", strings.Join(candidates, " or "))
}

func srachaOutputPathCandidates(file ichsm.ReadFile, run string, index, count int, splitMode string) []string {
	dir := filepath.Dir(file.OutputPath)
	ext := readFileExtension(file.Filename)
	if ext == "" {
		ext = ".fastq.gz"
	}
	candidates := []string{file.OutputPath}
	switch splitMode {
	case srachaSplitSpot:
		candidates = append(candidates, filepath.Join(dir, run+ext), filepath.Join(dir, run+".fastq.gz"))
	case srachaSplit3:
		if isBareReadFilename(file.Filename) {
			candidates = append(candidates,
				filepath.Join(dir, run+ext),
				filepath.Join(dir, run+".fastq.gz"),
				filepath.Join(dir, run+"_0"+ext),
				filepath.Join(dir, run+"_0.fastq.gz"),
			)
		} else {
			candidates = append(candidates, filepath.Join(dir, run+readFileSuffix(file.Filename, index, count, true)))
		}
	case srachaSplitFiles:
		candidates = append(candidates, filepath.Join(dir, run+readFileSuffix(file.Filename, index, count, true)))
	}
	return uniqueStrings(candidates)
}

// hasNumberedReadFile reports whether any file in the set carries its own read
// number, e.g. ERR123456_1.fastq.gz. ENA exposes a bare orphan-reads file
// alongside _1 and _2 for some paired runs, and numbering that bare file by its
// position would rename it onto read 1 and collide with the real read 1, so
// positional numbering is only safe when nothing in the set is self-numbered.
func hasNumberedReadFile(files []ichsm.ReadFile) bool {
	for _, file := range files {
		if !isBareReadFilename(file.Filename) {
			return true
		}
	}
	return false
}

func hasBareReadFile(files []ichsm.ReadFile) bool {
	for _, file := range files {
		if isBareReadFilename(file.Filename) {
			return true
		}
	}
	return false
}

func isBareReadFilename(filename string) bool {
	return readFileRole(filename) == ""
}

// readFileRole returns the read a file holds, as the trailing read number in
// its name: "_1", "_2", and so on, or "" for unpaired reads, which carry no
// number.
func readFileRole(filename string) string {
	ext := readFileExtension(filename)
	return trailingReadNumber(strings.TrimSuffix(filename, ext))
}

// mergeGroup is the set of files one merged output is concatenated from, held
// in the order the runs were given.
type mergeGroup struct {
	role  string
	files []DownloadedFile
}

// groupFilesByRole collects the runs' read files into one group per read.
//
// Grouping by role rather than by position means a run contributes each file to
// the output for the read it actually holds. Position is not safe to merge on:
// nothing guarantees every run lists its files in the same order, and getting
// it wrong concatenates one run's read 1 onto another's read 2, which no later
// step would catch.
//
// Every run must hold the same numbered reads, because merging a paired run
// with a single-end one leaves the pair mismatched. Unpaired reads are the
// exception: ENA lists an unpaired file for only some runs of a sample, so that
// group takes whichever runs have one.
func groupFilesByRole(results []Result) ([]mergeGroup, error) {
	groups := make(map[string]*mergeGroup)
	var wantNumbered []string

	for index, result := range results {
		if len(result.Files) == 0 {
			return nil, fmt.Errorf("cannot merge %s: it has no FASTQ files", describeRun(result, index))
		}

		var numbered []string
		seen := make(map[string]struct{}, len(result.Files))
		for _, file := range result.Files {
			role := readFileRole(file.Filename)
			if _, duplicate := seen[role]; duplicate {
				return nil, fmt.Errorf(
					"cannot merge %s: it has more than one FASTQ file for read %s",
					describeRun(result, index), readName(role),
				)
			}
			seen[role] = struct{}{}

			group, ok := groups[role]
			if !ok {
				group = &mergeGroup{role: role}
				groups[role] = group
			}
			group.files = append(group.files, file)
			if role != "" {
				numbered = append(numbered, role)
			}
		}

		slices.Sort(numbered)
		if index == 0 {
			wantNumbered = numbered
			continue
		}
		if !slices.Equal(numbered, wantNumbered) {
			return nil, fmt.Errorf(
				"cannot merge runs holding different reads: %s has %s, %s has %s",
				describeRun(results[0], 0), describeReads(wantNumbered),
				describeRun(result, index), describeReads(numbered),
			)
		}
	}

	ordered := make([]mergeGroup, 0, len(groups))
	for _, role := range wantNumbered {
		ordered = append(ordered, *groups[role])
	}
	if unpaired, ok := groups[""]; ok {
		ordered = append(ordered, *unpaired)
	}
	return ordered, nil
}

// readName describes a role for an error message.
func readName(role string) string {
	if role == "" {
		return "unpaired"
	}
	return strings.TrimPrefix(role, "_")
}

// describeRun names a run for an error message, falling back to its position
// for a Result a library caller built without an accession.
func describeRun(result Result, index int) string {
	if result.RunAccession != "" {
		return "run " + result.RunAccession
	}
	return fmt.Sprintf("run %d", index+1)
}

// describeReads lists the numbered reads a run holds, for an error message.
func describeReads(roles []string) string {
	if len(roles) == 0 {
		return "none"
	}
	names := make([]string, len(roles))
	for index, role := range roles {
		names[index] = readName(role)
	}
	return strings.Join(names, ", ")
}

func uniqueStrings(values []string) []string {
	out := values[:0]
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizePositiveDefault(value, defaultValue int, name string) (int, error) {
	if value == 0 {
		return defaultValue, nil
	}
	if value < 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return value, nil
}

func applyOutputPrefix(files []ichsm.ReadFile, outDir, prefix string) ([]ichsm.ReadFile, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return files, nil
	}
	if strings.ContainsAny(prefix, `/\`) {
		return nil, fmt.Errorf("output prefix must not contain path separators")
	}

	numberByPosition := !hasNumberedReadFile(files)

	out := make([]ichsm.ReadFile, len(files))
	copy(out, files)
	seen := make(map[string]struct{}, len(out))
	for i := range out {
		filename := prefix + readFileSuffix(out[i].Filename, i, len(out), numberByPosition)
		if _, ok := seen[filename]; ok {
			return nil, fmt.Errorf("output prefix produced duplicate FASTQ filename: %s", filename)
		}
		seen[filename] = struct{}{}
		out[i].Filename = filename
		out[i].OutputPath = filepath.Join(outDir, filename)
	}
	return out, nil
}

func ensureNoExistingOutputs(files []ichsm.ReadFile, finalDir, run, prefix string, writeMetadata bool) error {
	targets := make(map[string]struct{}, len(files)+1)
	if writeMetadata {
		targets[filepath.Join(finalDir, metadataFilename(run, prefix))] = struct{}{}
	}
	for _, file := range files {
		targets[filepath.Join(finalDir, file.Filename)] = struct{}{}
	}
	for target := range targets {
		if _, err := os.Stat(target); err == nil {
			return errFilesAlreadyExist
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func ensureMergedOutputsDoNotExist(files []DownloadedFile) error {
	for _, file := range files {
		if _, err := os.Stat(file.Path); err == nil {
			return errFilesAlreadyExist
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func prepareMergedMetadata(results []Result, root, prefix string) (temporaryPath, path string, err error) {
	hasMetadata := false
	for _, result := range results {
		if result.MetaPath != "" {
			hasMetadata = true
			break
		}
	}
	if !hasMetadata {
		return "", "", nil
	}

	records := make([]ichsm.Record, 0, len(results))
	for _, result := range results {
		if result.MetaPath == "" {
			return "", "", fmt.Errorf("cannot merge results when only some runs have ENA metadata")
		}
		data, readErr := os.ReadFile(result.MetaPath)
		if readErr != nil {
			return "", "", readErr
		}
		var runRecords []ichsm.Record
		if unmarshalErr := json.Unmarshal(data, &runRecords); unmarshalErr != nil {
			var record ichsm.Record
			if objectErr := json.Unmarshal(data, &record); objectErr != nil {
				return "", "", fmt.Errorf("read ENA metadata %s: expected a JSON object or list: %w", result.MetaPath, unmarshalErr)
			}
			runRecords = []ichsm.Record{record}
		}
		records = append(records, runRecords...)
	}

	path = filepath.Join(root, prefix+"_ena_meta.json")
	if _, statErr := os.Stat(path); statErr == nil {
		return "", "", errFilesAlreadyExist
	} else if !os.IsNotExist(statErr) {
		return "", "", statErr
	}

	out, createErr := os.CreateTemp(root, "."+prefix+"_ena_meta-*")
	if createErr != nil {
		return "", "", createErr
	}
	temporaryPath = out.Name()
	defer func() {
		if closeErr := out.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
			temporaryPath = ""
			path = ""
		}
	}()
	if chmodErr := out.Chmod(0o644); chmodErr != nil {
		return temporaryPath, path, chmodErr
	}
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if encodeErr := encoder.Encode(records); encodeErr != nil {
		return temporaryPath, path, encodeErr
	}
	return temporaryPath, path, nil
}

func removeFiles(paths []string) {
	for _, path := range paths {
		if path != "" {
			_ = os.Remove(path)
		}
	}
}

func removeResultFiles(results []Result) error {
	for _, result := range results {
		for _, file := range result.Files {
			if err := os.Remove(file.Path); err != nil {
				return err
			}
		}
		if result.MetaPath != "" {
			if err := os.Remove(result.MetaPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func moveResultFiles(result Result, finalDir string) error {
	moves := make([]fileMove, 0, len(result.Files)+1)
	if result.MetaPath != "" {
		moves = append(moves, fileMove{
			source: result.MetaPath,
			target: filepath.Join(finalDir, filepath.Base(result.MetaPath)),
		})
	}
	for _, file := range result.Files {
		moves = append(moves, fileMove{
			source: file.Path,
			target: filepath.Join(finalDir, filepath.Base(file.Path)),
		})
	}
	return publishFileMoves(moves)
}

type fileMove struct {
	source string
	target string
}

func publishFileMoves(moves []fileMove) error {
	return publishFileMovesWith(moves, os.Rename)
}

func publishFileMovesWith(moves []fileMove, rename func(string, string) error) error {
	targets := make(map[string]struct{}, len(moves))
	for _, move := range moves {
		if _, ok := targets[move.target]; ok {
			return fmt.Errorf("duplicate output path: %s", move.target)
		}
		targets[move.target] = struct{}{}
		if _, err := os.Stat(move.target); err == nil {
			return errFilesAlreadyExist
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	for index, move := range moves {
		if err := rename(move.source, move.target); err != nil {
			return rollbackFileMoves(moves[:index], rename, err)
		}
	}
	return nil
}

func rollbackFileMoves(moves []fileMove, rename func(string, string) error, publishErr error) error {
	errs := []error{publishErr}
	for index := len(moves) - 1; index >= 0; index-- {
		move := moves[index]
		if err := rename(move.target, move.source); err != nil {
			errs = append(errs, fmt.Errorf("roll back %s: %w", move.target, err))
		}
	}
	return errors.Join(errs...)
}

// readFileSuffix returns the suffix a read file should keep once renamed. A
// file that carries its own read number keeps it; otherwise the file is
// numbered by its position in the set, but only when numberByPosition says the
// set has no self-numbered files to collide with. See hasNumberedReadFile.
func readFileSuffix(filename string, index int, count int, numberByPosition bool) string {
	ext := readFileExtension(filename)
	stem := strings.TrimSuffix(filename, ext)
	if suffix := trailingReadNumber(stem); suffix != "" {
		return suffix + ext
	}
	if count == 1 || !numberByPosition {
		return ext
	}
	return fmt.Sprintf("_%d%s", index+1, ext)
}

func readFileExtension(filename string) string {
	lower := strings.ToLower(filename)
	for _, ext := range []string{".fastq.gz", ".fq.gz", ".fastq", ".fq", ".gz"} {
		if strings.HasSuffix(lower, ext) {
			return filename[len(filename)-len(ext):]
		}
	}
	return filepath.Ext(filename)
}

func parseByteCount(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func formatByteCount(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := float64(n)
	unit := units[0]
	for _, candidate := range units {
		value /= 1024
		unit = candidate
		if value < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

func trailingReadNumber(stem string) string {
	if stem == "" {
		return ""
	}
	for i := len(stem) - 1; i >= 0; i-- {
		if stem[i] < '0' || stem[i] > '9' {
			if i == len(stem)-1 {
				return ""
			}
			if stem[i] == '_' || stem[i] == '.' {
				return "_" + stem[i+1:]
			}
			return ""
		}
	}
	return ""
}

func randomRetryDelay(minDelay, maxDelay time.Duration) time.Duration {
	if minDelay == maxDelay {
		return minDelay
	}
	return minDelay + time.Duration(rand.Int63n(int64(maxDelay-minDelay)+1))
}

func resolveSracha(path string) (string, error) {
	if strings.TrimSpace(path) != "" {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return "", fmt.Errorf("sracha binary %q was not found or is not executable", path)
		}
		return resolved, nil
	}
	resolved, err := exec.LookPath("sracha")
	if err != nil {
		return "", fmt.Errorf("sracha not found in PATH; install sracha or pass --sracha-bin")
	}
	return resolved, nil
}

func writeENAMetadata(outDir, run, prefix string, record ichsm.Record, singleObject bool) (path string, err error) {
	path = filepath.Join(outDir, metadataFilename(run, prefix))
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer closeutil.CloseWithError(&err, out)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	value := any([]ichsm.Record{record})
	if singleObject {
		value = record
	}
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return path, nil
}

func singleMetadataRecord(results []ichsm.SearchResult, run string) (ichsm.Record, error) {
	if len(results) != 1 || len(results[0].Records) != 1 {
		return nil, fmt.Errorf("expected exactly one ENA metadata record for %s", run)
	}
	return results[0].Records[0], nil
}

func metadataFilename(run, prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return run + "_ena_meta.json"
	}
	return prefix + "_ena_meta.json"
}

func validateGzip(path string) (err error) {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, in)

	gr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, gr)

	_, err = io.Copy(io.Discard, gr)
	return err
}

func downloadedFiles(files []ichsm.ReadFile) []DownloadedFile {
	out := make([]DownloadedFile, 0, len(files))
	for _, file := range files {
		out = append(out, DownloadedFile{
			Filename: file.Filename,
			Path:     file.OutputPath,
			URL:      file.URL,
			MD5:      file.MD5,
			Bytes:    file.Bytes,
		})
	}
	return out
}

func resultInFinalDir(result Result, finalDir string) Result {
	result.Dir = finalDir
	if result.MetaPath != "" {
		result.MetaPath = filepath.Join(finalDir, filepath.Base(result.MetaPath))
	}
	for i := range result.Files {
		result.Files[i].Path = filepath.Join(finalDir, filepath.Base(result.Files[i].Path))
	}
	return result
}

func methodList(methods []Method) string {
	names := make([]string, 0, len(methods))
	for _, method := range methods {
		names = append(names, string(method))
	}
	return strings.Join(names, ",")
}

func allowedMethodStrings() []string {
	return []string{string(MethodENA), string(MethodSRACHA)}
}

func progressf(w io.Writer, format string, args ...any) {
	if w == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "download-reads: "+format+"\n", args...)
}

func commandString(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(name))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if !strings.ContainsAny(value, " \t\n'\"\\$`!#&;|*?()[]{}<>") {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (d *Downloader) httpClient() *http.Client {
	if d != nil && d.HTTPClient != nil {
		return d.HTTPClient
	}
	return &http.Client{}
}

func (d *Downloader) sleep(ctx context.Context, delay time.Duration) error {
	if d != nil && d.Sleep != nil {
		return d.Sleep(ctx, delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
