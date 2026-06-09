package genomedl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/martinghunt/faqt/internal/closeutil"
	"github.com/martinghunt/faqt/internal/xopen"
)

func writeDownloadedGenome(files []string, outPath string) error {
	_, err := writeDownloadedGenomeWithOptions(files, outPath, DownloadOptions{})
	return err
}

func writeDownloadedGenomeWithOptions(files []string, outPath string, opts DownloadOptions) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("download produced no files")
	}
	sort.Strings(files)
	if opts.FastaOnly {
		return writeFASTAOutput(files, outPath, opts)
	}
	return writeBestDownloadedOutput(files, outPath, opts)
}

func writeFASTAOutput(files []string, outPath string, opts DownloadOptions) (string, error) {
	fastaPath, err := singleMatchingFile(files, "FASTA", isFASTAFile)
	if err != nil {
		return "", err
	}
	if fastaPath == "" {
		return "", fmt.Errorf("download produced no FASTA file")
	}
	if err := copyFile(fastaPath, outPath); err != nil {
		return "", err
	}
	warnOutputExtensionMismatch(outPath, outputFASTA, opts.WarningWriter)
	return outPath, nil
}

func writeBestDownloadedOutput(files []string, outPath string, opts DownloadOptions) (string, error) {
	annotationPath, annotationKind, err := selectAnnotationFile(files)
	if err != nil {
		return "", err
	}
	if annotationPath == "" {
		return writeFASTAOutput(files, outPath, opts)
	}
	if annotationKind == annotationGFF3 {
		fastaPath, err := singleMatchingFile(files, "FASTA", isFASTAFile)
		if err != nil {
			return "", err
		}
		if fastaPath != "" {
			if err := combineGFF3AndFASTA(annotationPath, fastaPath, outPath); err != nil {
				return "", err
			}
			warnOutputExtensionMismatch(outPath, outputGFF3, opts.WarningWriter)
			return outPath, nil
		}
	}
	if err := copyFile(annotationPath, outPath); err != nil {
		return "", err
	}
	warnOutputExtensionMismatch(outPath, outputFormatFromAnnotationKind(annotationKind), opts.WarningWriter)
	return outPath, nil
}

type outputFormat int

const (
	outputUnknown outputFormat = iota
	outputFASTA
	outputGFF3
	outputGenBank
	outputEMBL
)

func outputFormatFromAnnotationKind(kind annotationKind) outputFormat {
	switch kind {
	case annotationGFF3:
		return outputGFF3
	case annotationGenBank:
		return outputGenBank
	case annotationEMBL:
		return outputEMBL
	default:
		return outputUnknown
	}
}

func warnOutputExtensionMismatch(outPath string, actual outputFormat, w io.Writer) {
	if w == nil || outPath == "-" || actual == outputUnknown {
		return
	}
	pathFormat, ok := outputFormatFromPath(outPath)
	if !ok || pathFormat == actual {
		return
	}
	_, _ = fmt.Fprintf(w, "warning: writing %s content to output path with %s extension: %s\n", actual.label(), pathFormat.label(), outPath)
}

func outputFormatFromPath(path string) (outputFormat, bool) {
	base := xopen.BasePathWithoutCompression(path)
	switch strings.ToLower(filepath.Ext(base)) {
	case ".fna", ".fa", ".fasta":
		return outputFASTA, true
	case ".gff", ".gff3":
		return outputGFF3, true
	case ".gb", ".gbk", ".gbff", ".genbank":
		return outputGenBank, true
	case ".embl":
		return outputEMBL, true
	default:
		return outputUnknown, false
	}
}

func (f outputFormat) label() string {
	switch f {
	case outputFASTA:
		return "FASTA"
	case outputGFF3:
		return "GFF3"
	case outputGenBank:
		return "GenBank"
	case outputEMBL:
		return "EMBL"
	default:
		return "unknown"
	}
}

type annotationKind int

const (
	annotationNone annotationKind = iota
	annotationGFF3
	annotationGenBank
	annotationEMBL
)

func selectAnnotationFile(files []string) (string, annotationKind, error) {
	for _, kind := range []annotationKind{annotationGFF3, annotationGenBank, annotationEMBL} {
		path, err := singleMatchingFile(files, annotationKindLabel(kind), func(name string) bool {
			return annotationFileKind(name) == kind
		})
		if err != nil {
			return "", annotationNone, err
		}
		if path != "" {
			return path, kind, nil
		}
	}
	return "", annotationNone, nil
}

func annotationKindLabel(kind annotationKind) string {
	switch kind {
	case annotationGFF3:
		return "GFF3"
	case annotationGenBank:
		return "GenBank"
	case annotationEMBL:
		return "EMBL"
	default:
		return "annotation"
	}
}

func annotationFileKind(name string) annotationKind {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".gff"), strings.HasSuffix(lower, ".gff3"):
		return annotationGFF3
	case strings.HasSuffix(lower, ".gb"),
		strings.HasSuffix(lower, ".gbk"),
		strings.HasSuffix(lower, ".gbff"),
		strings.HasSuffix(lower, ".genbank"):
		return annotationGenBank
	case strings.HasSuffix(lower, ".embl"):
		return annotationEMBL
	default:
		return annotationNone
	}
}

func singleMatchingFile(files []string, label string, match func(string) bool) (string, error) {
	matches := make([]string, 0, 1)
	for _, path := range files {
		if match(strings.ToLower(path)) {
			matches = append(matches, path)
		}
	}
	if len(matches) == 0 {
		return "", nil
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("download produced multiple %s files: %v", label, matches)
	}
	return matches[0], nil
}

func isFASTAFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".fna") ||
		strings.HasSuffix(lower, ".fa") ||
		strings.HasSuffix(lower, ".fasta")
}

func copyFile(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, in)

	out, outCloser, err := createOutputWriter(dst)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, outCloser)

	_, err = io.Copy(out, in)
	return err
}

func createOutputWriter(path string) (io.Writer, io.Closer, error) {
	var (
		base   io.Writer
		closer io.Closer
	)
	if path == "-" {
		base = os.Stdout
	} else {
		fh, err := os.Create(path)
		if err != nil {
			return nil, nil, err
		}
		base = fh
		closer = fh
	}

	wrapped, wrappedCloser, err := xopen.WrapWriter(base, xopen.CompressionFromPath(path))
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return nil, nil, err
	}
	return wrapped, closeutil.MultiCloser(closer, wrappedCloser), nil
}

func combineGFF3AndFASTA(gffPath, fastaPath, outPath string) (err error) {
	out, outCloser, err := createOutputWriter(outPath)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, outCloser)

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
