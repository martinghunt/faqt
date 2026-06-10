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
	run, err := cleanRunAccession(runAccession)
	if err != nil {
		return Result{}, err
	}
	methods, err := normalizeMethods(opts.Methods)
	if err != nil {
		return Result{}, err
	}
	attempts, err := normalizeAttempts(opts.Attempts)
	if err != nil {
		return Result{}, err
	}
	retryDelayMin, retryDelayMax, err := normalizeRetryDelay(opts.RetryDelayMin, opts.RetryDelayMax)
	if err != nil {
		return Result{}, err
	}
	downloadStallTimeout, err := normalizeDownloadStallTimeout(opts.DownloadStallTimeout)
	if err != nil {
		return Result{}, err
	}
	opts.DownloadStallTimeout = downloadStallTimeout
	downloadProgressInterval, err := normalizeDownloadProgressInterval(opts.DownloadProgressInterval)
	if err != nil {
		return Result{}, err
	}
	opts.DownloadProgressInterval = downloadProgressInterval
	root := strings.TrimSpace(opts.OutputDir)
	if root == "" {
		root = "."
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Result{}, err
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		for methodIndex, method := range methods {
			progressf(opts.ProgressWriter, "attempt %d/%d using %s", attempt, attempts, method)
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
			if attempt < attempts || methodIndex < len(methods)-1 {
				delay := randomRetryDelay(retryDelayMin, retryDelayMax)
				if delay > 0 {
					progressf(opts.ProgressWriter, "waiting %s before next attempt", delay)
					if err := d.sleep(ctx, delay); err != nil {
						return Result{}, err
					}
				}
			}
		}
	}
	return Result{}, fmt.Errorf("%d rounds of download attempts failed for methods: %s: %w", attempts, methodList(methods), lastErr)
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
		metaPath, err = writeENAMetadata(outDir, run, opts.OutputPrefix, manifest.Metadata)
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
		metaPath, err = writeENAMetadata(outDir, run, opts.OutputPrefix, manifest.Metadata)
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
	progressf(opts.ProgressWriter, "found %d FASTQ file(s)", len(files))
	return readManifest{Files: files, Metadata: record}, nil
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
			candidates = append(candidates, filepath.Join(dir, run+readFileSuffix(file.Filename, index, count)))
		}
	case srachaSplitFiles:
		candidates = append(candidates, filepath.Join(dir, run+readFileSuffix(file.Filename, index, count)))
	}
	return uniqueStrings(candidates)
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
	ext := readFileExtension(filename)
	stem := strings.TrimSuffix(filename, ext)
	return trailingReadNumber(stem) == ""
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

	out := make([]ichsm.ReadFile, len(files))
	copy(out, files)
	seen := make(map[string]struct{}, len(out))
	for i := range out {
		filename := prefix + readFileSuffix(out[i].Filename, i, len(out))
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

func moveResultFiles(result Result, finalDir string) error {
	moves := make(map[string]string, len(result.Files)+1)
	if result.MetaPath != "" {
		moves[result.MetaPath] = filepath.Join(finalDir, filepath.Base(result.MetaPath))
	}
	for _, file := range result.Files {
		moves[file.Path] = filepath.Join(finalDir, filepath.Base(file.Path))
	}
	targets := make(map[string]struct{}, len(moves))
	for _, target := range moves {
		if _, ok := targets[target]; ok {
			return fmt.Errorf("duplicate output path: %s", target)
		}
		targets[target] = struct{}{}
		if _, err := os.Stat(target); err == nil {
			return errFilesAlreadyExist
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for source, target := range moves {
		if err := os.Rename(source, target); err != nil {
			return err
		}
	}
	return nil
}

func readFileSuffix(filename string, index int, count int) string {
	ext := readFileExtension(filename)
	stem := strings.TrimSuffix(filename, ext)
	if suffix := trailingReadNumber(stem); suffix != "" {
		return suffix + ext
	}
	if count == 1 {
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

func writeENAMetadata(outDir, run, prefix string, record ichsm.Record) (path string, err error) {
	path = filepath.Join(outDir, metadataFilename(run, prefix))
	out, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer closeutil.CloseWithError(&err, out)

	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
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
