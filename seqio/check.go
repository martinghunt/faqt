package seqio

import (
	"io"

	"github.com/martinghunt/faqt/internal/seqcheck"
)

// CheckPath streams through an input and reports diagnostics as they occur.
// By default it stops at the first error. With allErrors it continues where
// format parsers support safe recovery; structural and I/O errors remain fatal
// for that input. It returns the first error, including any reported diagnostic.
// No records are written or repaired. The count includes checked invalid records.
func CheckPath(path string, allErrors bool, report func(error), opts ...Option) (count int, err error) {
	reader, openErr := OpenPath(path, opts...)
	emit := func(diagnostic error) {
		if err == nil {
			err = diagnostic
		}
		if report != nil {
			report(diagnostic)
		}
	}
	if openErr != nil {
		emit(openErr)
		return
	}
	if closer, ok := reader.(io.Closer); ok {
		defer func() {
			if closeErr := closer.Close(); closeErr != nil && (allErrors || err == nil) {
				emit(closeErr)
			}
		}()
	}
	var readErr error
	if allErrors {
		count, readErr = seqcheck.Check(reader, emit)
	} else {
		count, readErr = seqcheck.ReadAll(reader)
	}
	if readErr != nil {
		emit(readErr)
	}
	return
}
