package seqio

import "io"

type RecordTransform func(*SeqRecord) (*SeqRecord, error)

func Identity(rec *SeqRecord) (*SeqRecord, error) {
	return rec, nil
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

func TransformPath(inputPath, outputPath string, format Format, transform RecordTransform, opts ...Option) error {
	reader, err := OpenPath(inputPath)
	if err != nil {
		return err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	writer, err := CreatePath(outputPath, format, opts...)
	if err != nil {
		return err
	}
	defer writer.Close()

	return Process(reader, writer, transform)
}

func ToFASTAPath(inputPath, outputPath string, opts ...Option) error {
	return TransformPath(inputPath, outputPath, FASTA, nil, opts...)
}

func ToFASTAPathWithTransform(inputPath, outputPath string, transform RecordTransform, opts ...Option) error {
	return TransformPath(inputPath, outputPath, FASTA, transform, opts...)
}
