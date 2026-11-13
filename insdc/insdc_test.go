package insdc

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

// The fixtures in testdata are made by make_fixtures.py: Biopython records
// as JSON, and the GenBank and EMBL that Biopython writes for them.

type jsonPosition struct {
	Kind    string         `json:"kind"`
	Value   int            `json:"value"`
	Left    int            `json:"left"`
	Right   int            `json:"right"`
	Choices []jsonPosition `json:"choices"`
}

type jsonLocation struct {
	Start    jsonPosition   `json:"start"`
	End      jsonPosition   `json:"end"`
	Strand   int            `json:"strand"`
	Ref      string         `json:"ref"`
	Operator string         `json:"operator"`
	Parts    []jsonLocation `json:"parts"`
}

type jsonRecord struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Seq         string   `json:"seq"`
	DBXrefs     []string `json:"dbxrefs"`
	Annotations struct {
		MoleculeType      string   `json:"molecule_type"`
		Topology          string   `json:"topology"`
		DataFileDivision  string   `json:"data_file_division"`
		Date              string   `json:"date"`
		GI                string   `json:"gi"`
		DBSource          string   `json:"db_source"`
		Segment           string   `json:"segment"`
		Source            *string  `json:"source"`
		Organism          *string  `json:"organism"`
		Accessions        []string `json:"accessions"`
		Keywords          []string `json:"keywords"`
		Taxonomy          []string `json:"taxonomy"`
		Comment           []string `json:"comment"`
		StructuredComment []struct {
			Name   string      `json:"name"`
			Fields [][2]string `json:"fields"`
		} `json:"structured_comment"`
		References []struct {
			Location  *[2]int `json:"location"`
			Authors   string  `json:"authors"`
			Consrtm   string  `json:"consrtm"`
			Title     string  `json:"title"`
			Journal   string  `json:"journal"`
			MedlineID string  `json:"medline_id"`
			PubmedID  string  `json:"pubmed_id"`
			Comment   string  `json:"comment"`
		} `json:"references"`
	} `json:"annotations"`
	Features []struct {
		Type       string       `json:"type"`
		Location   jsonLocation `json:"location"`
		Qualifiers []struct {
			Key    string `json:"key"`
			Values []struct {
				Kind string `json:"kind"`
				Text string `json:"text"`
			} `json:"values"`
		} `json:"qualifiers"`
	} `json:"features"`
}

// loadRecords reads records written by record_json.py.
func loadRecords(t *testing.T, path string) []*Record {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw []jsonRecord
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	records := make([]*Record, len(raw))
	for i, jr := range raw {
		a := jr.Annotations
		r := &Record{
			SeqRecord: seqio.SeqRecord{Name: jr.Name, Description: jr.Description, Seq: []byte(jr.Seq)},
			ID:        jr.ID,
			DBXrefs:   jr.DBXrefs,
			Annotations: Annotations{
				MoleculeType:     a.MoleculeType,
				Topology:         a.Topology,
				DataFileDivision: a.DataFileDivision,
				Date:             a.Date,
				Accessions:       a.Accessions,
				GI:               a.GI,
				Keywords:         a.Keywords,
				Segment:          a.Segment,
				Source:           a.Source,
				Organism:         a.Organism,
				Taxonomy:         a.Taxonomy,
				DBSource:         a.DBSource,
				Comment:          a.Comment,
			},
		}
		for _, sc := range a.StructuredComment {
			block := StructuredComment{Name: sc.Name}
			for _, f := range sc.Fields {
				block.Fields = append(block.Fields, KeyValue{Key: f[0], Value: f[1]})
			}
			r.Annotations.StructuredComments = append(r.Annotations.StructuredComments, block)
		}
		for _, ref := range a.References {
			r.Annotations.References = append(r.Annotations.References, Reference{
				Location: ref.Location, Authors: ref.Authors, Consrtm: ref.Consrtm, Title: ref.Title,
				Journal: ref.Journal, MedlineID: ref.MedlineID, PubmedID: ref.PubmedID, Comment: ref.Comment,
			})
		}
		for _, jf := range jr.Features {
			f := Feature{Type: jf.Type, Location: toLocation(jf.Location)}
			for _, q := range jf.Qualifiers {
				qualifier := Qualifier{Key: q.Key}
				for _, v := range q.Values {
					switch v.Kind {
					case "flag":
						qualifier.Values = append(qualifier.Values, Flag())
					case "int":
						qualifier.Values = append(qualifier.Values, Int(v.Text))
					default:
						qualifier.Values = append(qualifier.Values, Text(v.Text))
					}
				}
				f.Qualifiers = append(f.Qualifiers, qualifier)
			}
			r.Features = append(r.Features, f)
		}
		records[i] = r
	}
	return records
}

func toLocation(jl jsonLocation) Location {
	if len(jl.Parts) != 0 {
		l := Location{Operator: jl.Operator}
		for _, p := range jl.Parts {
			l.Parts = append(l.Parts, toLocation(p))
		}
		return l
	}
	return Location{Start: toPosition(jl.Start), End: toPosition(jl.End), Strand: jl.Strand, Ref: jl.Ref}
}

func toPosition(jp jsonPosition) Position {
	kinds := map[string]PositionKind{"exact": Exact, "before": Before, "after": After, "within": Within, "between": Between, "oneof": OneOf, "unknown": Unknown}
	p := Position{Value: jp.Value, Kind: kinds[jp.Kind], Left: jp.Left, Right: jp.Right}
	for _, c := range jp.Choices {
		p.Choices = append(p.Choices, toPosition(c))
	}
	return p
}

func TestWritersMatchBiopython(t *testing.T) {
	records := loadRecords(t, filepath.Join("testdata", "records.json"))
	for _, tt := range []struct {
		name  string
		want  string
		write func(*bytes.Buffer, ...*Record) error
	}{
		{name: "GenBank", want: "records.gb", write: func(b *bytes.Buffer, r ...*Record) error { return WriteGenBank(b, r...) }},
		{name: "EMBL", want: "records.embl", write: func(b *bytes.Buffer, r ...*Record) error { return WriteEMBL(b, r...) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata", tt.want))
			if err != nil {
				t.Fatal(err)
			}
			var got bytes.Buffer
			if err := tt.write(&got, records...); err != nil {
				t.Fatalf("write error = %v", err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatal(firstDifference(got.Bytes(), want))
			}
		})
	}
}

// firstDifference describes the first differing line.
func firstDifference(got, want []byte) string {
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	for i := 0; i < max(len(gotLines), len(wantLines)); i++ {
		var g, w string
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if g != w {
			return "line " + strconv.Itoa(i+1) + ":\n got: " + g + "\nwant: " + w
		}
	}
	return "outputs differ"
}

func TestWriteErrors(t *testing.T) {
	dna := Annotations{MoleculeType: "DNA"}
	for _, tt := range []struct {
		name    string
		record  Record
		genbank string
		embl    string
	}{
		{name: "missing molecule type", record: Record{ID: "A", SeqRecord: seqio.SeqRecord{Seq: []byte("acgt")}}, genbank: "missing molecule type", embl: "missing molecule type"},
		{name: "whitespace in locus", record: Record{ID: "A", SeqRecord: seqio.SeqRecord{Name: "two words", Seq: []byte("acgt")}, Annotations: dna}, genbank: "invalid whitespace"},
		{name: "semicolon in accession", record: Record{ID: "A;B", SeqRecord: seqio.SeqRecord{Seq: []byte("acgt")}, Annotations: dna}, embl: "semi-colon"},
		{name: "space in accession", record: Record{ID: "A", SeqRecord: seqio.SeqRecord{Seq: []byte("acgt")}, Annotations: Annotations{MoleculeType: "DNA", Accessions: []string{"A B"}}}, embl: "spaces"},
		{name: "unknown molecule type", record: Record{ID: "A", SeqRecord: seqio.SeqRecord{Seq: []byte("acgt")}, Annotations: Annotations{MoleculeType: "other"}}, embl: "failed to understand molecule type"},
		{name: "unknown location", record: Record{ID: "A", SeqRecord: seqio.SeqRecord{Seq: []byte("acgt")}, Annotations: dna, Features: []Feature{
			{Type: "misc_feature", Location: Location{Start: Position{Kind: Unknown}, End: Position{Kind: Unknown}}},
		}}, genbank: "unknown location", embl: "unknown location"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for format, want := range map[string]string{"GenBank": tt.genbank, "EMBL": tt.embl} {
				var err error
				if format == "GenBank" {
					err = WriteGenBank(&bytes.Buffer{}, &tt.record)
				} else {
					err = WriteEMBL(&bytes.Buffer{}, &tt.record)
				}
				if want == "" {
					if err != nil {
						t.Errorf("%s error = %v, want none", format, err)
					}
					continue
				}
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("%s error = %v, want %q", format, err, want)
				}
			}
		})
	}
}

func TestStreamingWriterMatchesWriteAll(t *testing.T) {
	records := loadRecords(t, filepath.Join("testdata", "records.json"))
	var all, streamed bytes.Buffer
	if err := WriteGenBank(&all, records...); err != nil {
		t.Fatal(err)
	}
	w := NewGenBankWriter(&streamed)
	for _, r := range records {
		if err := w.Write(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(all.Bytes(), streamed.Bytes()) {
		t.Fatal("record-at-a-time output differs from WriteGenBank")
	}
}
