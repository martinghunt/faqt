package fasta

import (
	"bufio"
	"bytes"
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/seqcheck"
	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r           *bufio.Reader
	pending     []byte // owned copy of the next record's header line
	havePending bool
	eof         bool
	long        []byte // scratch for lines longer than the read buffer
	seq         []byte // sequence accumulator, reused between records
	lineNumber  int
	report      func(error) // set only while checking; normal reads remain strict
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

// Check consumes the remaining records, reporting each contiguous NUL run
// and continuing without modifying input. Other errors stop the check.
// The count includes records containing reported errors.
func (r *Reader) Check(report func(error)) (int, error) {
	r.report = report
	defer func() { r.report = nil }()
	return seqcheck.ReadAll(r)
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	if r.eof && !r.havePending {
		return nil, io.EOF
	}

	var header []byte
	if r.havePending {
		header = r.pending
		r.havePending = false
	}
	for len(header) == 0 {
		line, err := r.readLine()
		if err != nil && err != io.EOF {
			return nil, err
		}
		if len(line) == 0 && err == io.EOF {
			r.eof = true
			return nil, io.EOF
		}
		if len(line) == 0 {
			continue
		}
		header = line
		if err == io.EOF {
			r.eof = true
		}
	}
	if header[0] != '>' {
		return nil, io.ErrUnexpectedEOF
	}
	name, desc := seqrecord.ParseHeader(header[1:])
	if err := r.checkHeader(header, name); err != nil {
		return nil, err
	}

	// Accumulate into a buffer the Reader keeps, so a wrapped record costs
	// no allocation per line and the buffer stops growing once it is big
	// enough for the records in this file.
	seq := r.seq[:0]
	var run *nulRun
	for {
		line, err := r.readLine()
		if err != nil && err != io.EOF {
			_ = r.finishNULRun(name, &run)
			return nil, err
		}
		if len(line) > 0 && line[0] == '>' {
			if err := r.finishNULRun(name, &run); err != nil {
				return nil, err
			}
			r.pending = append(r.pending[:0], line...)
			r.havePending = true
			break
		}
		trimmed := bytes.TrimSpace(line)
		if err := r.checkSequenceLine(line, trimmed, name, len(seq), &run); err != nil {
			return nil, err
		}
		seq = append(seq, trimmed...)
		if err == io.EOF {
			if err := r.finishNULRun(name, &run); err != nil {
				return nil, err
			}
			r.eof = true
			break
		}
	}
	r.seq = seq

	// The caller owns the record, so hand over a copy sized to this
	// sequence rather than the accumulator itself.
	owned := make([]byte, len(seq))
	copy(owned, seq)
	return &seqrecord.SeqRecord{Name: name, Description: desc, Seq: owned}, nil
}

// readLine returns the next line with any trailing \r and \n removed, along
// with io.EOF if that line ended the input. The returned bytes point into the
// read buffer and stay valid only until the next read.
func (r *Reader) readLine() ([]byte, error) {
	line, err := r.r.ReadSlice('\n')
	if err == bufio.ErrBufferFull {
		// A line longer than the read buffer, such as an unwrapped
		// chromosome. Stitch the pieces together in scratch space.
		r.long = append(r.long[:0], line...)
		for err == bufio.ErrBufferFull {
			line, err = r.r.ReadSlice('\n')
			r.long = append(r.long, line...)
		}
		line = r.long
	}
	if err != nil && err != io.EOF {
		return nil, err
	}
	r.lineNumber++
	return trimEOL(line), err
}

type nulRun struct {
	start, end                       int // 1-based inclusive sequence positions
	line, column, endLine, endColumn int
}

func (r *Reader) diagnostic(err error) error {
	if r.report == nil {
		return err
	}
	r.report(err)
	return nil
}

func (r *Reader) checkHeader(header []byte, name string) error {
	for offset := 0; offset < len(header); {
		i := bytes.IndexByte(header[offset:], 0)
		if i < 0 {
			break
		}
		i += offset
		end := i + 1
		for end < len(header) && header[end] == 0 {
			end++
		}
		if err := r.diagnostic(fmt.Errorf("fasta: NUL byte (0x00) on line %d, contig %q, header columns %d-%d (%d bytes)", r.lineNumber, name, i+1, end, end-i)); err != nil {
			return err
		}
		offset = end
	}
	return nil
}

func (r *Reader) finishNULRun(name string, pending **nulRun) error {
	if *pending == nil {
		return nil
	}
	run := *pending
	*pending = nil
	return r.diagnostic(fmt.Errorf("fasta: NUL byte (0x00) on line %d, contig %q, sequence positions %d-%d (%d bytes; line:column %d:%d-%d:%d)", run.line, name, run.start, run.end, run.end-run.start+1, run.line, run.column, run.endLine, run.endColumn))
}

func (r *Reader) checkSequenceLine(rawLine, line []byte, name string, seqOffset int, pending **nulRun) error {
	for offset := 0; offset < len(line); {
		i := bytes.IndexByte(line[offset:], 0)
		if i != 0 {
			if err := r.finishNULRun(name, pending); err != nil {
				return err
			}
		}
		if i < 0 {
			return nil
		}
		i += offset
		end := i + 1
		for end < len(line) && line[end] == 0 {
			end++
		}
		if *pending == nil {
			*pending = &nulRun{start: seqOffset + i + 1, line: r.lineNumber, column: bytes.Index(rawLine, line) + i + 1}
		}
		(*pending).end = seqOffset + end
		(*pending).endLine = r.lineNumber
		(*pending).endColumn = bytes.Index(rawLine, line) + end
		// Ordinary parsing stops immediately; diagnostic checks retain a
		// trailing run so it can continue across wrapped sequence lines.
		if r.report == nil || end < len(line) {
			if err := r.finishNULRun(name, pending); err != nil {
				return err
			}
		}
		offset = end
	}
	return nil
}

func trimEOL(line []byte) []byte {
	for len(line) > 0 {
		c := line[len(line)-1]
		if c != '\n' && c != '\r' {
			break
		}
		line = line[:len(line)-1]
	}
	return line
}
