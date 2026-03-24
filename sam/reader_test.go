package sam

import (
	"io"
	"strings"
	"testing"
)

func TestReaderReverseComplementsReverseStrand(t *testing.T) {
	input := "read1\t16\tref\t1\t60\t4M\t*\t0\t0\tATGC\tABCD\n"
	r, err := NewReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("NewReader() error = %v", err)
	}

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "read1" || string(rec.Seq) != "GCAT" || string(rec.Qual) != "DCBA" {
		t.Fatalf("record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}
