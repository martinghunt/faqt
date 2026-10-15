package seqio

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/fasta"
	"github.com/martinghunt/faqt/gff3"
	"github.com/martinghunt/faqt/internal/seqcheck"
	"github.com/martinghunt/faqt/seq"
)

// CheckPath streams through an input and reports diagnostics as they occur.
// By default it stops at the first error. With allErrors it continues where
// format parsers support safe recovery; structural and I/O errors remain fatal
// for that input. It returns the first error, including any reported diagnostic.
// No records are written or repaired. The count includes checked invalid records.
// Alphabet checking is opt-in with WithCheckAlphabet and examines normalized
// sequences, not raw input formatting, headers, qualities, or annotations.
func CheckPath(path string, allErrors bool, report func(error), opts ...Option) (count int, err error) {
	emit := func(diagnostic error) {
		if err == nil {
			err = diagnostic
		}
		if report != nil {
			report(diagnostic)
		}
	}
	options := newOptions(opts...)
	var alphabet *seq.Alphabet
	if options.checkAlphabet != "" {
		var alphabetErr error
		alphabet, alphabetErr = seq.NewAlphabet(options.checkAlphabet, options.checkGaps)
		if alphabetErr != nil {
			emit(alphabetErr)
			return
		}
	} else if options.checkGaps {
		emit(fmt.Errorf("allowing gaps requires an alphabet (dna, rna, or protein)"))
		return
	}
	reader, openErr := OpenPath(path, opts...)
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
	var visit func(*SeqRecord) error
	if alphabet != nil {
		// FASTA checking already reports every NUL run with source line
		// coordinates. Avoid reporting those bytes again as alphabet errors.
		source := reader
		if wrapped, ok := source.(*readerWithCloser); ok {
			source = wrapped.Reader
		}
		nulsReported := false
		switch source.(type) {
		case *fasta.Reader, *gff3.Reader:
			nulsReported = allErrors
		}
		visit = func(record *SeqRecord) error {
			var first error
			emitRun := func(start, end int) bool {
				diagnostic := fmt.Errorf("sequence: record %q, characters outside %s alphabet at sequence positions %d-%d (%d bytes; first byte %q, 0x%02X)", record.Name, options.checkAlphabet, start+1, end, end-start, record.Seq[start], record.Seq[start])
				if !allErrors {
					first = diagnostic
					return false
				}
				emit(diagnostic)
				return true
			}
			alphabet.InvalidRuns(record.Seq, func(start, end int) bool {
				if !nulsReported {
					return emitRun(start, end)
				}
				for start < end {
					if record.Seq[start] == 0 {
						start++
						continue
					}
					stop := start + 1
					for stop < end && record.Seq[stop] != 0 {
						stop++
					}
					if !emitRun(start, stop) {
						return false
					}
					start = stop
				}
				return true
			})
			return first
		}
	}
	var readErr error
	if allErrors {
		count, readErr = seqcheck.Check(reader, emit, visit)
	} else {
		count, readErr = seqcheck.ReadAll(reader, visit)
	}
	if readErr != nil {
		emit(readErr)
	}
	return
}
