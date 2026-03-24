package seqio_test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	htsbam "github.com/biogo/hts/bam"
	htssam "github.com/biogo/hts/sam"
	"github.com/martinghunt/faqt/seqio"
)

func TestOpenReaderDetectsFormats(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantIDs []string
	}{
		{
			name:    "fasta",
			input:   ">a desc\nACGT\n>b\nTT\n",
			wantIDs: []string{"a", "b"},
		},
		{
			name:    "fastq",
			input:   "@a desc\nACGT\n+\n!!!!\n@b\nTT\n+\n##\n",
			wantIDs: []string{"a", "b"},
		},
		{
			name:    "sam",
			input:   "@HD\tVN:1.6\nr1\t16\t*\t0\t0\t*\t*\t0\t0\tACGT\tBCDE\nr2\t0\t*\t0\t0\t*\t*\t0\t0\tTTAA\t*\n",
			wantIDs: []string{"r1", "r2"},
		},
		{
			name:    "phylip",
			input:   "2 4\nsample1 ACGT\nsample2 TTAA\n",
			wantIDs: []string{"sample1", "sample2"},
		},
		{
			name:    "clustal",
			input:   "CLUSTAL W (1.83) multiple sequence alignment\n\nseq1    AC-GT\nseq2    ACGGT\n        ** **\n\nseq1    TT-\nseq2    TTT\n",
			wantIDs: []string{"seq1", "seq2"},
		},
		{
			name:    "genbank",
			input:   "LOCUS       REC1\nDEFINITION  alpha\nORIGIN\n        1 acgt\n//\nLOCUS       REC2\nDEFINITION  beta\nORIGIN\n        1 ttaa\n//\n",
			wantIDs: []string{"REC1", "REC2"},
		},
		{
			name:    "embl",
			input:   "ID   REC1;\nDE   alpha\nSQ   Sequence 4 BP;\n     acgt 4\n//\nID   REC2;\nDE   beta\nSQ   Sequence 4 BP;\n     ttaa 4\n//\n",
			wantIDs: []string{"REC1", "REC2"},
		},
		{
			name:    "gff3",
			input:   "##gff-version 3\nchr1\t.\tgene\t1\t4\t.\t+\t.\tID=g1\n##FASTA\n>chr1\nACGT\n>chr2\nTTAA\n",
			wantIDs: []string{"chr1", "chr2"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r, err := seqio.OpenReader(strings.NewReader(tc.input))
			if err != nil {
				t.Fatalf("OpenReader() error = %v", err)
			}
			var got []string
			for {
				rec, err := r.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("Read() error = %v", err)
				}
				got = append(got, rec.Name)
			}
			if strings.Join(got, ",") != strings.Join(tc.wantIDs, ",") {
				t.Fatalf("record names = %v, want %v", got, tc.wantIDs)
			}
		})
	}
}

func TestOpenReaderClustalKeepsDashes(t *testing.T) {
	input := "CLUSTAL W (1.83) multiple sequence alignment\n\nseq1    AC-GT\nseq2    ACGGT\n        ** **\n\nseq1    TT-\nseq2    TTT\n"
	r, err := seqio.OpenReader(strings.NewReader(input))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "seq1" || string(rec.Seq) != "AC-GTTT-" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestOpenReaderBAM(t *testing.T) {
	var bamData bytes.Buffer
	ref, err := htssam.NewReference("ref", "", "", 100, nil, nil)
	if err != nil {
		t.Fatalf("NewReference() error = %v", err)
	}
	header, err := htssam.NewHeader(nil, []*htssam.Reference{ref})
	if err != nil {
		t.Fatalf("NewHeader() error = %v", err)
	}
	bw, err := htsbam.NewWriter(&bamData, header, 0)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}
	rec1, err := htssam.NewRecord("r1", ref, nil, 0, -1, 0, 0, nil, []byte("ACGT"), []byte{30, 31, 32, 33}, nil)
	if err != nil {
		t.Fatalf("NewRecord(rec1) error = %v", err)
	}
	rec1.Flags = htssam.Reverse
	rec2, err := htssam.NewRecord("r2", ref, nil, 0, -1, 0, 0, nil, []byte("TTAA"), nil, nil)
	if err != nil {
		t.Fatalf("NewRecord(rec2) error = %v", err)
	}
	if err := bw.Write(rec1); err != nil {
		t.Fatalf("Write(rec1) error = %v", err)
	}
	if err := bw.Write(rec2); err != nil {
		t.Fatalf("Write(rec2) error = %v", err)
	}
	if err := bw.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	r, err := seqio.OpenReader(bytes.NewReader(bamData.Bytes()))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	first, err := r.Read()
	if err != nil {
		t.Fatalf("Read(first) error = %v", err)
	}
	if first.Name != "r1" || string(first.Seq) != "ACGT" || string(first.Qual) != "BA@?" {
		t.Fatalf("first record = %+v", first)
	}
	second, err := r.Read()
	if err != nil {
		t.Fatalf("Read(second) error = %v", err)
	}
	if second.Name != "r2" || string(second.Seq) != "TTAA" || second.Qual != nil {
		t.Fatalf("second record = %+v", second)
	}
}

func TestGFF3WithoutFASTAErrors(t *testing.T) {
	r, err := seqio.OpenReader(strings.NewReader("##gff-version 3\nchr1\t.\tgene\t1\t4\t.\t+\t.\tID=g1\n"))
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	_, err = r.Read()
	if err == nil || !strings.Contains(err.Error(), "##FASTA") {
		t.Fatalf("Read() error = %v, want missing ##FASTA", err)
	}
}

func TestOpenPathDetectsCompressionAndIgnoresFilename(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "misleading_name.txt")
	w, err := seqio.CreatePath(path+".gz", seqio.FASTA)
	if err != nil {
		t.Fatalf("CreatePath() error = %v", err)
	}
	if err := w.Write(&seqio.SeqRecord{Name: "rec1", Seq: []byte("ACGT")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := os.Rename(path+".gz", path); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}

	r, err := seqio.OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "rec1" || string(rec.Seq) != "ACGT" {
		t.Fatalf("record = %+v", rec)
	}
}

func TestCreatePathCompressionByExtension(t *testing.T) {
	paths := []string{"out.fa.gz", "out.fa.bz2", "out.fa.xz", "out.fa.zst"}
	for _, name := range paths {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			w, err := seqio.CreatePath(path, seqio.FASTA)
			if err != nil {
				t.Fatalf("CreatePath() error = %v", err)
			}
			if err := w.Write(&seqio.SeqRecord{Name: "x", Seq: []byte("ACGT")}); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			r, err := seqio.OpenPath(path)
			if err != nil {
				t.Fatalf("OpenPath() error = %v", err)
			}
			rec, err := r.Read()
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
			if rec.Name != "x" {
				t.Fatalf("record name = %q, want x", rec.Name)
			}
		})
	}
}

func TestRecordStringWriteToAndWriter(t *testing.T) {
	rec := seqio.SeqRecord{Name: "r1", Description: "desc", Seq: []byte("ACGT")}
	if got := rec.String(); got != ">r1 desc\nACGT\n" {
		t.Fatalf("String() = %q", got)
	}
	if got := rec.FASTAString(3); got != ">r1 desc\nACG\nT\n" {
		t.Fatalf("FASTAString() = %q", got)
	}

	var buf bytes.Buffer
	if _, err := rec.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	if buf.String() != ">r1 desc\nACGT\n" {
		t.Fatalf("WriteTo() output = %q", buf.String())
	}
	buf.Reset()
	if _, err := rec.WriteFASTATo(&buf, 3); err != nil {
		t.Fatalf("WriteFASTATo() error = %v", err)
	}
	if buf.String() != ">r1 desc\nACG\nT\n" {
		t.Fatalf("WriteFASTATo() output = %q", buf.String())
	}

	buf.Reset()
	w := seqio.NewFASTAWriter(&buf, seqio.WithWrap(3))
	if err := w.Write(&rec); err != nil {
		t.Fatalf("Writer.Write() error = %v", err)
	}
	if buf.String() != ">r1 desc\nACG\nT\n" {
		t.Fatalf("wrapped output = %q", buf.String())
	}
}

func TestFASTQValidation(t *testing.T) {
	var buf bytes.Buffer
	w := seqio.NewFASTQWriter(&buf)
	err := w.Write(&seqio.SeqRecord{Name: "bad", Seq: []byte("AC"), Qual: []byte("!")})
	if err == nil || !strings.Contains(err.Error(), "quality length") {
		t.Fatalf("Write() error = %v, want quality length mismatch", err)
	}
}

func TestCreateFASTAPathHelper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.fa.gz")
	w, err := seqio.CreateFASTAPath(path, seqio.WithWrap(2))
	if err != nil {
		t.Fatalf("CreateFASTAPath() error = %v", err)
	}
	if err := w.Write(&seqio.SeqRecord{Name: "r1", Seq: []byte("ACGT")}); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	r, err := seqio.OpenPath(path)
	if err != nil {
		t.Fatalf("OpenPath() error = %v", err)
	}
	rec, err := r.Read()
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if rec.Name != "r1" || string(rec.Seq) != "ACGT" {
		t.Fatalf("record = %+v", rec)
	}
}
