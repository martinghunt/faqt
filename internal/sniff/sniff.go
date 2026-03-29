package sniff

import (
	"bufio"
	"bytes"
	"fmt"
)

const PeekSize = 8192

func Format(r *bufio.Reader) (string, error) {
	buf, err := r.Peek(PeekSize)
	if err != nil && !isShortPeek(err) {
		return "", err
	}
	buf = bytes.TrimLeft(buf, "\n\r\t ")
	if len(buf) == 0 {
		return "", fmt.Errorf("empty input")
	}
	switch {
	case bytes.HasPrefix(buf, []byte("##gff-version 3")):
		return "gff3", nil
	case bytes.HasPrefix(buf, []byte("LOCUS")):
		return "genbank", nil
	case bytes.HasPrefix(buf, []byte("ID")):
		return "embl", nil
	case looksLikeClustal(buf):
		return "clustal", nil
	case looksLikePhylip(buf):
		return "phylip", nil
	case buf[0] == '>':
		return "fasta", nil
	case looksLikeSAM(buf):
		return "sam", nil
	case looksLikeFASTQ(buf):
		return "fastq", nil
	default:
		return "", fmt.Errorf("could not detect sequence format from content")
	}
}

func looksLikeFASTQ(buf []byte) bool {
	if len(buf) == 0 || buf[0] != '@' {
		return false
	}
	lines := bytes.SplitN(buf, []byte{'\n'}, 4)
	if len(lines) < 4 {
		return false
	}
	return len(lines[1]) > 0 && len(lines[2]) > 0 && lines[2][0] == '+'
}

func looksLikeSAM(buf []byte) bool {
	line, _, _ := bytes.Cut(buf, []byte{'\n'})
	line = bytes.TrimRight(line, "\r")
	if len(line) == 0 {
		return false
	}
	if bytes.HasPrefix(line, []byte("@HD\t")) ||
		bytes.HasPrefix(line, []byte("@SQ\t")) ||
		bytes.HasPrefix(line, []byte("@RG\t")) ||
		bytes.HasPrefix(line, []byte("@PG\t")) ||
		bytes.HasPrefix(line, []byte("@CO\t")) {
		return true
	}
	fields := bytes.Split(line, []byte{'\t'})
	if len(fields) < 11 {
		return false
	}
	if len(fields[0]) == 0 {
		return false
	}
	for _, idx := range []int{1, 3, 4, 7, 8} {
		if !isIntegerField(fields[idx]) {
			return false
		}
	}
	return true
}

func looksLikePhylip(buf []byte) bool {
	line, _, _ := bytes.Cut(buf, []byte{'\n'})
	line = bytes.TrimSpace(line)
	fields := bytes.Fields(line)
	if len(fields) < 2 {
		return false
	}
	return isIntegerField(fields[0]) && isIntegerField(fields[1])
}

func looksLikeClustal(buf []byte) bool {
	line, _, _ := bytes.Cut(buf, []byte{'\n'})
	line = bytes.TrimSpace(line)
	return bytes.HasPrefix(bytes.ToUpper(line), []byte("CLUSTAL"))
}

func isIntegerField(field []byte) bool {
	if len(field) == 0 {
		return false
	}
	if field[0] == '-' {
		field = field[1:]
	}
	if len(field) == 0 {
		return false
	}
	for _, b := range field {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

func isShortPeek(err error) bool {
	return err == bufio.ErrBufferFull || err.Error() == "EOF"
}
