package fasta

import (
	"bufio"
	"bytes"
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r           *bufio.Reader
	pending     []byte // owned copy of the next record's header line
	havePending bool
	eof         bool
	long        []byte // scratch for lines longer than the read buffer
	seq         []byte // sequence accumulator, reused between records
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
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

	// Accumulate into a buffer the Reader keeps, so a wrapped record costs
	// no allocation per line and the buffer stops growing once it is big
	// enough for the records in this file.
	seq := r.seq[:0]
	for {
		line, err := r.readLine()
		if err != nil && err != io.EOF {
			return nil, err
		}
		if len(line) > 0 && line[0] == '>' {
			r.pending = append(r.pending[:0], line...)
			r.havePending = true
			break
		}
		seq = append(seq, bytes.TrimSpace(line)...)
		if err == io.EOF {
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
	return trimEOL(line), err
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
