// Package seqcheck supports streaming diagnostics without attempting to repair input.
package seqcheck

import (
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader interface {
	Read() (*seqrecord.SeqRecord, error)
}

// Check uses a format's recovery support when available. Returned errors are
// fatal for this input; recoverable errors are sent to report.
func Check(r Reader, report func(error), visit func(*seqrecord.SeqRecord) error) (int, error) {
	if checker, ok := r.(interface {
		CheckRecords(func(error), func(*seqrecord.SeqRecord) error) (int, error)
	}); ok {
		return checker.CheckRecords(report, visit)
	}
	return ReadAll(r, visit)
}

func ReadAll(r Reader, visit func(*seqrecord.SeqRecord) error) (int, error) {
	count := 0
	for {
		record, err := r.Read()
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return count, err
		}
		count++
		if visit != nil {
			if err := visit(record); err != nil {
				return count, err
			}
		}
	}
}
