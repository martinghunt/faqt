// Package insdc writes annotated sequence records as GenBank and EMBL.
//
// It is the only part of faqt with an annotation model, and it is write-only:
// faqt's GenBank and EMBL readers stay sequence-only. Output matches
// Biopython's SeqIO.write(record, handle, "genbank") and "embl" byte for byte,
// so Record mirrors the parts of Biopython's SeqRecord that those writers
// read. Field comments name the Biopython attribute or record.annotations key
// each one stands for. Output was checked against Biopython 1.88; versions
// 1.79 to 1.88 write the same files, and 1.78 does not write the GenBank
// DBSOURCE line.
//
// Like Biopython, text lengths for wrapping are counted in characters, not
// bytes.
//
// The writers are adapted from Biopython's Bio/SeqIO/InsdcIO.py under the BSD
// 3-Clause License; see third_party/biopython.
package insdc

// Record is an annotated sequence record.
type Record struct {
	// ID is record.id. GenBank and EMBL take the accession and version from
	// it when Annotations.Accessions is empty.
	ID string
	// Name is record.name, the GenBank LOCUS name. If empty, ID is used.
	Name string
	// Description is record.description, written as DEFINITION or DE.
	Description string
	// Seq is the sequence. Output is lower case.
	Seq []byte
	// DBXrefs is record.dbxrefs, written as GenBank DBLINK and EMBL PR.
	DBXrefs     []string
	Annotations Annotations
	Features    []Feature
}

// Annotations holds the record.annotations entries Biopython writes. A nil
// pointer or slice means the key is absent, which Biopython often writes
// differently from an empty value.
type Annotations struct {
	// MoleculeType is "molecule_type", e.g. "DNA". It is required.
	MoleculeType string
	// Topology is "topology": "linear", "circular" or empty.
	Topology string
	// DataFileDivision is "data_file_division", e.g. "BCT" or "PRO". Each
	// format maps the other's codes to its own.
	DataFileDivision string
	// Date is "date", as DD-MON-YYYY. GenBank uses 01-JAN-1980 when it is
	// empty or invalid. EMBL does not write it.
	Date string
	// Accessions is "accessions"; only the first is written.
	Accessions []string
	// GI is "gi". Empty means absent.
	GI string
	// Keywords is "keywords".
	Keywords []string
	// Segment is "segment". Empty means absent.
	Segment string
	// Source is "source", the GenBank SOURCE line ("." when nil).
	Source *string
	// Organism is "organism" ("." when nil).
	Organism *string
	// Taxonomy is "taxonomy".
	Taxonomy []string
	// DBSource is "db_source". Empty means absent.
	DBSource string
	// References is "references".
	References []Reference
	// Comment is "comment", one entry per line. Each entry is wrapped
	// separately.
	Comment []string
	// StructuredComments is "structured_comment" (GenBank only).
	StructuredComments []StructuredComment
}

// Reference is a Bio.SeqFeature.Reference. Empty strings are not written.
type Reference struct {
	// Location optionally gives the bases the reference covers, as a
	// 0-based start and exclusive end.
	Location  *[2]int
	Authors   string
	Consrtm   string
	Title     string
	Journal   string
	MedlineID string
	PubmedID  string
	Comment   string
}

// StructuredComment is one ##Name-START## ... ##Name-END## block.
type StructuredComment struct {
	Name   string
	Fields []KeyValue
}

// KeyValue is one structured comment field.
type KeyValue struct {
	Key   string
	Value string
}

// Feature is a Bio.SeqFeature.SeqFeature.
type Feature struct {
	// Type is the feature key, e.g. "CDS".
	Type       string
	Location   Location
	Qualifiers []Qualifier
}

// Qualifier is one feature.qualifiers entry. Each value is written as its
// own /Key=value line, in order.
type Qualifier struct {
	Key    string
	Values []Value
}

// Value is a qualifier value.
type Value struct {
	text string
	kind valueKind
}

type valueKind int

const (
	textValue valueKind = iota
	intValue
	flagValue
)

// Text is a string value. It is quoted unless the qualifier key is one
// Biopython leaves unquoted, such as transl_table.
func Text(s string) Value { return Value{text: s, kind: textValue} }

// Int is an integer value, written unquoted, like a Python int. Use it for
// Python bools too, passing "True" or "False".
func Int(s string) Value { return Value{text: s, kind: intValue} }

// Flag is a value-less qualifier such as /pseudo (a Python None value).
func Flag() Value { return Value{kind: flagValue} }

// Location is a Bio.SeqFeature.SimpleLocation, or a CompoundLocation when
// Parts is set.
type Location struct {
	// Start and End are 0-based, End exclusive, as in Biopython.
	Start, End Position
	// Strand is 1, -1, or 0 for none.
	Strand int
	// Ref is the optional "Accession:" prefix for a location on another
	// record.
	Ref string
	// Parts and Operator ("join" or "order") make a compound location. The
	// compound strand is the parts' strand if they all share one.
	Parts    []Location
	Operator string
}

// Position is a location bound, possibly fuzzy.
type Position struct {
	// Value is the position. For Within and Between positions it is the
	// value Biopython compares and adds offsets to.
	Value int
	Kind  PositionKind
	// Left and Right bound Within ("(left.right)") and Between ("(left^right)")
	// positions.
	Left, Right int
	// Choices are the alternatives of a OneOf position.
	Choices []Position
}

// PositionKind is the kind of a Position.
type PositionKind int

const (
	Exact PositionKind = iota
	// Before is "<n".
	Before
	// After is ">n".
	After
	// Within is "(left.right)".
	Within
	// Between is "(left^right)".
	Between
	// OneOf is "one-of(a,b,...)".
	OneOf
	// Unknown is an unknown position.
	Unknown
)

// At returns an exact position.
func At(n int) Position { return Position{Value: n} }

// Span returns an exact location from 0-based start to exclusive end.
func Span(start, end, strand int) Location {
	return Location{Start: At(start), End: At(end), Strand: strand}
}

func (l Location) strand() int {
	if len(l.Parts) == 0 {
		return l.Strand
	}
	strand := l.Parts[0].strand()
	for _, part := range l.Parts[1:] {
		if part.strand() != strand {
			return 0
		}
	}
	return strand
}
