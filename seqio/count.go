package seqio

import (
	"io"

	"github.com/martinghunt/faqt/internal/closeutil"
)

func CountRecords(reader Reader) (int, error) {
	count := 0
	for {
		_, err := reader.Read()
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return 0, err
		}
		count++
	}
}

func CountRecordsPath(path string) (count int, err error) {
	reader, err := OpenPath(path)
	if err != nil {
		return 0, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}
	return CountRecords(reader)
}
