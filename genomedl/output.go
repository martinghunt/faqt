package genomedl

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/martinghunt/faqt/internal/closeutil"
)

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
