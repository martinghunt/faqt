package seqio

import (
	"io"

	"github.com/martinghunt/faqt/internal/closeutil"
)

type RecordTransform func(*SeqRecord) (*SeqRecord, error)

func Identity(rec *SeqRecord) (*SeqRecord, error) {
	return rec, nil
}

func RemoveDashes(rec *SeqRecord) (*SeqRecord, error) {
	if err := rec.ValidateFASTQ(); err != nil {
		return nil, err
	}
	copyRec := *rec
	copyRec.Seq = make([]byte, 0, len(rec.Seq))
	if rec.Qual != nil {
		copyRec.Qual = make([]byte, 0, len(rec.Qual))
	}
	for i, base := range rec.Seq {
		if base == '-' {
			continue
		}
		copyRec.Seq = append(copyRec.Seq, base)
		if rec.Qual != nil {
			copyRec.Qual = append(copyRec.Qual, rec.Qual[i])
		}
	}
	return &copyRec, nil
}

func Process(reader Reader, writer WriteCloser, transform RecordTransform) error {
	if transform == nil {
		transform = Identity
	}
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		out, err := transform(rec)
		if err != nil {
			return err
		}
		if out == nil {
			continue
		}
		if err := writer.Write(out); err != nil {
			return err
		}
	}
}

func TransformPath(inputPath, outputPath string, format Format, transform RecordTransform, opts ...Option) (err error) {
	reader, err := OpenPath(inputPath)
	if err != nil {
		return err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}

	writer, err := CreatePath(outputPath, format, opts...)
	if err != nil {
		return err
	}
	defer closeutil.CloseWithError(&err, writer)

	return Process(reader, writer, transform)
}

func ToFASTAPath(inputPath, outputPath string, opts ...Option) error {
	return TransformPath(inputPath, outputPath, FASTA, nil, opts...)
}

func ToFASTAPathWithTransform(inputPath, outputPath string, transform RecordTransform, opts ...Option) error {
	return TransformPath(inputPath, outputPath, FASTA, transform, opts...)
}
