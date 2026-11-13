package insdc

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

const emblHeaderWidth = 5

// emblDivisions are the EMBL division codes Biopython keeps as given.
var emblDivisions = map[string]bool{
	"PHG": true, "ENV": true, "FUN": true, "HUM": true, "INV": true, "MAM": true,
	"VRT": true, "MUS": true, "PLN": true, "PRO": true, "ROD": true, "SYN": true,
	"TGN": true, "UNC": true, "VRL": true, "XXX": true,
}

// EMBLWriter writes records in EMBL format.
type EMBLWriter struct {
	w *bufio.Writer
}

// NewEMBLWriter returns a writer to w. Call Flush when done.
func NewEMBLWriter(w io.Writer) *EMBLWriter {
	return &EMBLWriter{w: bufio.NewWriter(w)}
}

// Flush writes any buffered output.
func (e *EMBLWriter) Flush() error {
	return e.w.Flush()
}

// WriteEMBL writes records to w in EMBL format.
func WriteEMBL(w io.Writer, records ...*Record) error {
	e := NewEMBLWriter(w)
	for _, record := range records {
		if err := e.Write(record); err != nil {
			return err
		}
	}
	return e.Flush()
}

// Write writes one record.
func (e *EMBLWriter) Write(r *Record) error {
	if err := e.writeIDAndAccession(r); err != nil {
		return err
	}
	a := r.Annotations

	xrefs := append([]string(nil), r.DBXrefs...)
	sort.Strings(xrefs)
	for _, xref := range xrefs {
		if strings.HasPrefix(xref, "BioProject:") {
			e.writeSingleLine("PR", xref[3:]+";")
			e.w.WriteString("XX\n")
			break
		}
		if strings.HasPrefix(xref, "Project:") {
			e.writeSingleLine("PR", xref+";")
			e.w.WriteString("XX\n")
			break
		}
	}

	description := r.Description
	if description == "<unknown description>" {
		description = "."
	}
	e.writeMultiLine("DE", description)
	e.w.WriteString("XX\n")
	if a.Keywords != nil {
		for _, keyword := range a.Keywords {
			e.writeSingleLine("KW", keyword)
		}
		e.w.WriteString("XX\n")
	}
	e.writeMultiLine("OS", stringOr(a.Organism, "."))
	taxonomy := "."
	if a.Taxonomy != nil {
		taxonomy = strings.Join(a.Taxonomy, "; ") + "."
	}
	e.writeMultiLine("OC", taxonomy)
	e.w.WriteString("XX\n")
	e.writeReferences(a.References)
	if a.Comment != nil && len(a.Comment) != 0 {
		for _, line := range a.Comment {
			e.writeMultiLine("CC", line)
		}
		e.w.WriteString("XX\n")
	}

	e.w.WriteString("FH   Key             Location/Qualifiers\nFH\n")
	features := featureWriter{w: e.w, indent: "FT" + strings.Repeat(" ", qualifierIndent-2), prefix: "FT   "}
	for _, f := range r.Features {
		if err := features.writeFeature(f, len(r.Seq)); err != nil {
			return err
		}
	}
	e.w.WriteString("XX\n")
	e.writeSequence(r)
	e.w.WriteString("//\n")
	return nil
}

func (e *EMBLWriter) writeSingleLine(tag, text string) {
	e.w.WriteString(tag + "   " + text + "\n")
}

func (e *EMBLWriter) writeMultiLine(tag, text string) {
	for _, line := range splitMultiLine(text, maxWidth-emblHeaderWidth) {
		e.writeSingleLine(tag, line)
	}
}

func (e *EMBLWriter) writeIDAndAccession(r *Record) error {
	version := ""
	accessionDefault := r.ID
	if dot := strings.LastIndex(r.ID, "."); dot >= 0 && isDigits(r.ID[dot+1:]) {
		version = "SV " + r.ID[dot+1:]
		accessionDefault = r.ID[:dot]
	}
	accession := firstOr(r.Annotations.Accessions, accessionDefault)
	if strings.Contains(accession, ";") {
		return fmt.Errorf("cannot have semi-colon in EMBL accession %q", accession)
	}
	if strings.Contains(accession, " ") {
		return fmt.Errorf("cannot have spaces in EMBL accession %q", accession)
	}
	molType := r.Annotations.MoleculeType
	if molType == "" {
		return fmt.Errorf("missing molecule type for record %q", r.ID)
	}
	upper := strings.ToUpper(molType)
	var units string
	switch {
	case strings.Contains(upper, "DNA"), strings.Contains(upper, "RNA"):
		units = "BP"
	case strings.Contains(upper, "PROTEIN"):
		molType = "PROTEIN"
		units = "AA"
	default:
		return fmt.Errorf("failed to understand molecule type %q", molType)
	}
	e.writeSingleLine("ID", fmt.Sprintf("%s; %s; %s; %s; ; %s; %d %s.",
		accession, version, r.Annotations.Topology, molType, emblDivision(r.Annotations.DataFileDivision), len(r.Seq), units))
	e.w.WriteString("XX\n")
	e.writeSingleLine("AC", accession+";")
	e.w.WriteString("XX\n")
	return nil
}

func emblDivision(division string) string {
	if emblDivisions[division] {
		return division
	}
	switch division {
	case "BCT":
		return "PRO"
	default:
		return "UNC"
	}
}

func (e *EMBLWriter) writeReferences(refs []Reference) {
	for i, ref := range refs {
		e.writeSingleLine("RN", "["+strconv.Itoa(i+1)+"]")
		if ref.Location != nil {
			e.writeSingleLine("RP", fmt.Sprintf("%d-%d", ref.Location[0]+1, ref.Location[1]))
		}
		if ref.PubmedID != "" {
			e.writeSingleLine("RX", "PUBMED; "+ref.PubmedID+".")
		}
		if ref.Consrtm != "" {
			e.writeSingleLine("RG", ref.Consrtm)
		}
		if ref.Authors != "" {
			e.writeMultiLine("RA", ref.Authors+";")
		}
		if ref.Title != "" {
			e.writeMultiLine("RT", `"`+ref.Title+`";`)
		}
		if ref.Journal != "" {
			e.writeMultiLine("RL", ref.Journal)
		}
		e.w.WriteString("XX\n")
	}
}

func (e *EMBLWriter) writeSequence(r *Record) {
	data := lowerASCII(r.Seq)
	if strings.Contains(r.Annotations.MoleculeType, "DNA") {
		var a, c, g, t int
		for _, b := range data {
			switch b {
			case 'a':
				a++
			case 'c':
				c++
			case 'g':
				g++
			case 't':
				t++
			}
		}
		fmt.Fprintf(e.w, "SQ   Sequence %d BP; %d A; %d C; %d G; %d T; %d other;\n", len(data), a, c, g, t, len(data)-a-c-g-t)
	} else {
		e.w.WriteString("SQ   \n")
	}
	full := len(data) / 60
	for line := 0; line < full; line++ {
		e.w.WriteString("    ")
		for block := 0; block < 6; block++ {
			start := 60*line + 10*block
			e.w.WriteByte(' ')
			e.w.Write(data[start : start+10])
		}
		fmt.Fprintf(e.w, "%10d\n", (line+1)*60)
	}
	if len(data)%60 != 0 {
		e.w.WriteString("    ")
		for block := 0; block < 6; block++ {
			start := min(60*full+10*block, len(data))
			chunk := " " + string(data[start:min(start+10, len(data))])
			fmt.Fprintf(e.w, "%-11s", chunk)
		}
		fmt.Fprintf(e.w, "%10d\n", len(data))
	}
}
