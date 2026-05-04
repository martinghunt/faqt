package seqio

import (
	"fmt"
	"io"
	"strings"
)

type InterleaveOptions struct {
	Suffix1 string
	Suffix2 string
}

func Interleave(reader1, reader2 Reader, writer WriteCloser, opts InterleaveOptions) error {
	for {
		rec1, err := reader1.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		rec2, err := reader2.Read()
		if err == io.EOF {
			return fmt.Errorf("error getting mate for sequence %q", rec1.Name)
		}
		if err != nil {
			return err
		}

		if err := writeInterleavedPair(writer, rec1, rec2, opts); err != nil {
			return err
		}
	}

	rec2, err := reader2.Read()
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("error getting mate for sequence %q", rec2.Name)
}

func InterleavePath(inputPath1, inputPath2, outputPath string, interleaveOpts InterleaveOptions, opts ...Option) error {
	reader1, err := OpenPath(inputPath1)
	if err != nil {
		return err
	}
	if closer, ok := reader1.(io.Closer); ok {
		defer closer.Close()
	}

	reader2, err := OpenPath(inputPath2)
	if err != nil {
		return err
	}
	if closer, ok := reader2.(io.Closer); ok {
		defer closer.Close()
	}

	first1, err := reader1.Read()
	if err == io.EOF {
		first2, err := reader2.Read()
		if err == io.EOF {
			writer, err := CreatePath(outputPath, FASTA, opts...)
			if err != nil {
				return err
			}
			return writer.Close()
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("error getting mate for sequence %q", first2.Name)
	}
	if err != nil {
		return err
	}

	first2, err := reader2.Read()
	if err == io.EOF {
		return fmt.Errorf("error getting mate for sequence %q", first1.Name)
	}
	if err != nil {
		return err
	}

	format, err := interleavedOutputFormat(first1, first2)
	if err != nil {
		return err
	}
	writer, err := CreatePath(outputPath, format, opts...)
	if err != nil {
		return err
	}
	defer writer.Close()

	if err := writeInterleavedPair(writer, first1, first2, interleaveOpts); err != nil {
		return err
	}
	return Interleave(reader1, reader2, writer, interleaveOpts)
}

func writeInterleavedPair(writer WriteCloser, rec1, rec2 *SeqRecord, opts InterleaveOptions) error {
	out1 := withSuffix(rec1, opts.Suffix1)
	out2 := withSuffix(rec2, opts.Suffix2)
	if err := writer.Write(out1); err != nil {
		return err
	}
	return writer.Write(out2)
}

func withSuffix(rec *SeqRecord, suffix string) *SeqRecord {
	if suffix == "" || strings.HasSuffix(rec.Name, suffix) {
		return rec
	}
	out := *rec
	out.Name += suffix
	return &out
}

func interleavedOutputFormat(rec1, rec2 *SeqRecord) (Format, error) {
	hasQual1 := rec1.Qual != nil
	hasQual2 := rec2.Qual != nil
	switch {
	case hasQual1 && hasQual2:
		return FASTQ, nil
	case !hasQual1 && !hasQual2:
		return FASTA, nil
	default:
		return FormatUnknown, fmt.Errorf("cannot interleave FASTA and FASTQ records in one output")
	}
}
