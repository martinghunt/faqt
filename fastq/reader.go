package fastq

import (
	"bufio"
	"fmt"
	"io"

	"github.com/martinghunt/faqt/internal/seqrecord"
)

type Reader struct {
	r    *bufio.Reader
	long []byte // scratch for lines longer than the read buffer
}

func NewReader(r *bufio.Reader) *Reader {
	return &Reader{r: r}
}

func (r *Reader) Read() (*seqrecord.SeqRecord, error) {
	header, err := r.readLine()
	if err != nil {
		return nil, err
	}
	if len(header) == 0 || header[0] != '@' {
		return nil, fmt.Errorf("fastq record must start with @")
	}
	name, desc := seqrecord.ParseHeader(header[1:])

	seqLine, err := r.readLine()
	if err != nil {
		return nil, err
	}
	// Sequence and quality are the same length in a valid record, so one
	// allocation sized from the sequence line holds both and every base is
	// copied once. A mismatched record grows the buffer and then fails
	// validation below.
	seqLen := len(seqLine)
	buf := make([]byte, seqLen, 2*seqLen)
	copy(buf, seqLine)

	plus, err := r.readLine()
	if err != nil {
		return nil, err
	}
	if len(plus) == 0 || plus[0] != '+' {
		return nil, fmt.Errorf("fastq separator line must start with +")
	}

	qualLine, err := r.readLine()
	if err != nil {
		return nil, err
	}
	buf = append(buf, qualLine...)

	rec := &seqrecord.SeqRecord{
		Name:        name,
		Description: desc,
		Seq:         buf[:seqLen:seqLen],
		Qual:        buf[seqLen:],
	}
	if err := rec.ValidateFASTQ(); err != nil {
		return nil, err
	}
	return rec, nil
}

// readLine returns the next line with any trailing \r and \n removed. The
// returned bytes point into the read buffer and stay valid only until the next
// read, so callers must copy anything they keep.
func (r *Reader) readLine() ([]byte, error) {
	line, err := r.r.ReadSlice('\n')
	if err == bufio.ErrBufferFull {
		// A line longer than the read buffer, such as a long read.
		// Stitch the pieces together in scratch space.
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
	line = trimEOL(line)
	if err == io.EOF && len(line) == 0 {
		return nil, io.EOF
	}
	return line, nil
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
