// Package agc adapts AGC archives to faqt sequence records while preserving
// sample boundaries explicitly.
package agc

import (
	"errors"
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
	goagc "github.com/martinghunt/go-agc"
)

var (
	// ErrClosed is returned when an operation uses a closed archive.
	ErrClosed = goagc.ErrClosed
	// ErrCorruptArchive marks malformed, truncated, or inconsistent AGC input.
	ErrCorruptArchive = goagc.ErrCorruptArchive
	// ErrUnsupportedVersion marks an AGC archive whose major version is not 3.
	ErrUnsupportedVersion = goagc.ErrUnsupportedVersion
	// ErrSampleNotFound marks a requested sample that is absent from the archive.
	ErrSampleNotFound = goagc.ErrSampleNotFound
	// ErrRandomAccessRequired marks AGC input supplied through a streaming source.
	ErrRandomAccessRequired = errors.New("agc input requires an uncompressed seekable file or ReaderAt")
)

// Archive is an open, read-only AGC archive.
type Archive struct {
	inner *goagc.Archive
}

// OpenPath opens and validates a local AGC archive. AGC requires random access;
// stdin and externally compressed archives are not supported.
func OpenPath(path string) (*Archive, error) {
	if path == "-" {
		return nil, fmt.Errorf("%w: stdin is not supported", ErrRandomAccessRequired)
	}
	inner, err := goagc.Open(path)
	if err != nil {
		return nil, err
	}
	return &Archive{inner: inner}, nil
}

// OpenReaderAt opens and validates an AGC archive from a random-access source
// and its exact size. The caller retains ownership of r.
func OpenReaderAt(r io.ReaderAt, size int64) (*Archive, error) {
	inner, err := goagc.OpenReaderAt(r, size)
	if err != nil {
		return nil, err
	}
	return &Archive{inner: inner}, nil
}

// Close closes the archive and any file opened by OpenPath.
func (a *Archive) Close() error {
	return a.inner.Close()
}

// Samples returns sample names in archive order. The reference sample is
// first.
func (a *Archive) Samples() ([]string, error) {
	samples, err := a.inner.Samples()
	if err != nil {
		return nil, err
	}
	names := make([]string, len(samples))
	for i, sample := range samples {
		names[i] = sample.Name
	}
	return names, nil
}

// OpenSample returns a reader that decodes one sample's contigs in archive
// order.
func (a *Archive) OpenSample(name string) (*Reader, error) {
	inner, err := a.inner.NewContigReader(goagc.Sample{Name: name})
	if err != nil {
		return nil, err
	}
	return &Reader{inner: inner}, nil
}

// OpenAll returns a reader for every sample in archive order. Contig names are
// prefixed with "sample." so identities remain distinct in the flat record
// stream. Samples and their contigs remain adjacent.
func (a *Archive) OpenAll() (*AllReader, error) {
	samples, err := a.Samples()
	if err != nil {
		return nil, err
	}
	return &AllReader{archive: a, samples: samples}, nil
}

// IterateSamples calls yield once per sample in archive order. Each supplied
// reader contains only that sample's contigs.
func (a *Archive) IterateSamples(yield func(sample string, records *Reader) error) error {
	if yield == nil {
		return fmt.Errorf("agc: nil IterateSamples callback")
	}
	samples, err := a.Samples()
	if err != nil {
		return err
	}
	for _, sample := range samples {
		r, err := a.OpenSample(sample)
		if err != nil {
			return err
		}
		if err := yield(sample, r); err != nil {
			return err
		}
	}
	return nil
}

// Reader reads one AGC sample as minimal sequence records.
type Reader struct {
	inner *goagc.ContigReader
}

// Read returns the next contig as a sequence record, or io.EOF after the
// sample's final contig.
func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	contig, err := r.inner.Read()
	if err != nil {
		return nil, err
	}
	name, description := seqrecord.ParseHeader([]byte(contig.Name))
	return &seqrecord.SeqRecord{
		Name:        name,
		Description: description,
		Seq:         contig.Sequence,
	}, nil
}

// AllReader flattens an archive into sample-adjacent sequence records.
type AllReader struct {
	archive *Archive
	samples []string
	next    int
	current *Reader
}

// Read returns the next contig with its sample name prepended, or io.EOF after
// the archive's final sample.
func (r *AllReader) Read() (*seqrecord.SeqRecord, error) {
	for {
		if r.current == nil {
			if r.next >= len(r.samples) {
				return nil, io.EOF
			}
			current, err := r.archive.OpenSample(r.samples[r.next])
			if err != nil {
				return nil, err
			}
			r.current = current
		}
		record, err := r.current.Read()
		if err == io.EOF {
			r.current = nil
			r.next++
			continue
		}
		if err != nil {
			return nil, err
		}
		record.Name = r.samples[r.next] + "." + record.Name
		return record, nil
	}
}
