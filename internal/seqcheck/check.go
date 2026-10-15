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
func Check(r Reader, report func(error)) (int, error) {
	if checker, ok := r.(interface {
		Check(func(error)) (int, error)
	}); ok {
		return checker.Check(report)
	}
	return ReadAll(r)
}

func ReadAll(r Reader) (int, error) {
	count := 0
	for {
		_, err := r.Read()
		if err == io.EOF {
			return count, nil
		}
		if err != nil {
			return count, err
		}
		count++
	}
}
