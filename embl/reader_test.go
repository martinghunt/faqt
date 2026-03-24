package embl

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

func TestReaderMultiRecord(t *testing.T) {
	input := "ID   REC1;\nDE   first record\nSQ   Sequence 6 BP;\n     acgt nn\n//\nID   REC2;\nDE   second record\nSQ   Sequence 4 BP;\n     ttaa\n//\n"
	r := NewReader(bufio.NewReader(strings.NewReader(input)))

	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() first error = %v", err)
	}
	if rec.Name != "REC1" || rec.Description != "first record" || string(rec.Seq) != "acgtnn" {
		t.Fatalf("first record = %+v", rec)
	}

	rec, err = r.Read()
	if err != nil {
		t.Fatalf("Read() second error = %v", err)
	}
	if rec.Name != "REC2" || rec.Description != "second record" || string(rec.Seq) != "ttaa" {
		t.Fatalf("second record = %+v", rec)
	}

	_, err = r.Read()
	if err != io.EOF {
		t.Fatalf("Read() final error = %v, want EOF", err)
	}
}
