package seqio

import (
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/closeutil"
)

// ReadAll reads every record from reader into memory.
func ReadAll(reader Reader) ([]*SeqRecord, error) {
	var records []*SeqRecord
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		records = append(records, cloneSeqRecord(rec))
	}
}

// ReadAllPath opens path, detects its format and compression, and reads every
// record into memory.
func ReadAllPath(path string) (records []*SeqRecord, err error) {
	reader, err := OpenPath(path)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}
	return ReadAll(reader)
}

// ReadAllByName reads every record from reader into a map keyed by record name.
// Duplicate record names return an error.
func ReadAllByName(reader Reader) (map[string]*SeqRecord, error) {
	records := make(map[string]*SeqRecord)
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return nil, err
		}
		if _, ok := records[rec.Name]; ok {
			return nil, fmt.Errorf("duplicate record name %q", rec.Name)
		}
		records[rec.Name] = cloneSeqRecord(rec)
	}
}

// ReadAllByNamePath opens path, detects its format and compression, and reads
// every record into a map keyed by record name. Duplicate record names return
// an error.
func ReadAllByNamePath(path string) (records map[string]*SeqRecord, err error) {
	reader, err := OpenPath(path)
	if err != nil {
		return nil, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closeutil.CloseWithError(&err, closer)
	}
	return ReadAllByName(reader)
}

func cloneSeqRecord(rec *SeqRecord) *SeqRecord {
	if rec == nil {
		return nil
	}
	copyRec := *rec
	copyRec.Seq = append([]byte(nil), rec.Seq...)
	copyRec.Qual = append([]byte(nil), rec.Qual...)
	return &copyRec
}
