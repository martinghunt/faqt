package genomedl

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/martinghunt/faqt/internal/closeutil"
)

func extractGenomeFilesFromZip(zipPath, outDir string) (files []string, err error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, err
	}
	defer closeutil.CloseWithError(&err, reader)

	files = make([]string, 0, 2)
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

func extractZipFile(file *zip.File, outPath string) (err error) {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, src)

	dst, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, dst)

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
