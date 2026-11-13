package insdc

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const genBankHeaderWidth = 12

// genBankDivisions are the GenBank division codes Biopython keeps as given.
var genBankDivisions = map[string]bool{
	"PRI": true, "ROD": true, "MAM": true, "VRT": true, "INV": true, "PLN": true,
	"BCT": true, "VRL": true, "PHG": true, "SYN": true, "UNA": true, "EST": true,
	"PAT": true, "STS": true, "GSS": true, "HTG": true, "HTC": true, "ENV": true,
	"CON": true, "TSA": true,
}

// emblToGenBankDivisions maps EMBL division codes that have a GenBank
// equivalent; any other code becomes UNK.
var emblToGenBankDivisions = map[string]string{
	"FUN": "PLN", "HUM": "PRI", "MUS": "ROD", "PRO": "BCT", "UNC": "UNK", "XXX": "UNK",
}

var months = []string{"JAN", "FEB", "MAR", "APR", "MAY", "JUN", "JUL", "AUG", "SEP", "OCT", "NOV", "DEC"}

// GenBankWriter writes records in GenBank format.
type GenBankWriter struct {
	w *bufio.Writer
}

// NewGenBankWriter returns a writer to w. Call Flush when done.
func NewGenBankWriter(w io.Writer) *GenBankWriter {
	return &GenBankWriter{w: bufio.NewWriter(w)}
}

// Flush writes any buffered output.
func (g *GenBankWriter) Flush() error {
	return g.w.Flush()
}

// WriteGenBank writes records to w in GenBank format.
func WriteGenBank(w io.Writer, records ...*Record) error {
	g := NewGenBankWriter(w)
	for _, record := range records {
		if err := g.Write(record); err != nil {
			return err
		}
	}
	return g.Flush()
}

// Write writes one record.
func (g *GenBankWriter) Write(r *Record) error {
	if err := g.writeLocus(r); err != nil {
		return err
	}
	a := r.Annotations

	accessionDefault := r.ID
	if before, after, ok := strings.Cut(r.ID, "."); ok && !strings.Contains(after, ".") && isDigits(after) {
		accessionDefault = before
	}
	accession := firstOr(a.Accessions, accessionDefault)
	versioned := accession
	if strings.HasPrefix(r.ID, accession+".") {
		if version, ok := pythonInt(strings.SplitN(r.ID, ".", 2)[1]); ok {
			versioned = accession + "." + strconv.Itoa(version)
		}
	}
	description := r.Description
	if description == "<unknown description>" {
		description = ""
	}
	g.writeMultiLine("DEFINITION", description+".")
	g.writeSingleLine("ACCESSION", accession)
	if a.GI != "" {
		g.writeSingleLine("VERSION", versioned+"  GI:"+a.GI)
	} else {
		g.writeSingleLine("VERSION", versioned)
	}

	for i, xref := range r.DBXrefs {
		if !strings.Contains(xref, ": ") {
			xref = strings.ReplaceAll(xref, ":", ": ")
		}
		tag := ""
		if i == 0 {
			tag = "DBLINK"
		}
		g.writeSingleLine(tag, xref)
	}

	keywords := "."
	if a.Keywords != nil {
		keywords = strings.Join(a.Keywords, "; ")
		if !strings.HasSuffix(keywords, ".") {
			keywords += "."
		}
	}
	g.writeMultiLine("KEYWORDS", keywords)
	if a.Segment != "" {
		g.writeSingleLine("SEGMENT", a.Segment)
	}
	g.writeMultiLine("SOURCE", stringOr(a.Source, "."))
	organism := []rune(stringOr(a.Organism, "."))
	if len(organism) > maxWidth-genBankHeaderWidth {
		organism = append(organism[:maxWidth-genBankHeaderWidth-4], []rune("...")...)
	}
	g.writeSingleLine("  ORGANISM", string(organism))
	taxonomy := "."
	if a.Taxonomy != nil {
		taxonomy = strings.Join(a.Taxonomy, "; ")
		if !strings.HasSuffix(taxonomy, ".") {
			taxonomy += "."
		}
	}
	g.writeMultiLine("", taxonomy)
	if a.DBSource != "" {
		g.writeSingleLine("DBSOURCE", a.DBSource)
	}
	g.writeReferences(r)
	if a.Comment != nil || a.StructuredComments != nil {
		if err := g.writeComment(a); err != nil {
			return err
		}
	}

	g.w.WriteString("FEATURES             Location/Qualifiers\n")
	features := featureWriter{w: g.w, indent: strings.Repeat(" ", qualifierIndent), prefix: "     "}
	for _, f := range r.Features {
		if err := features.writeFeature(f, len(r.Seq)); err != nil {
			return err
		}
	}
	g.writeSequence(r.Seq)
	g.w.WriteString("//\n")
	return nil
}

func (g *GenBankWriter) writeSingleLine(tag, text string) {
	fmt.Fprintf(g.w, "%-*s%s\n", genBankHeaderWidth, tag, strings.ReplaceAll(text, "\n", " "))
}

func (g *GenBankWriter) writeMultiLine(tag, text string) {
	lines := splitMultiLine(text, maxWidth-genBankHeaderWidth)
	g.writeSingleLine(tag, lines[0])
	for _, line := range lines[1:] {
		g.writeSingleLine("", line)
	}
}

func (g *GenBankWriter) writeLocus(r *Record) error {
	locus := r.Name
	if locus == "" || locus == "<unknown name>" {
		locus = r.ID
	}
	if locus == "" || locus == "<unknown id>" {
		locus = firstOr(r.Annotations.Accessions, ".")
	}
	if len(strings.Fields(locus)) > 1 {
		return fmt.Errorf("invalid whitespace in %q for LOCUS line", locus)
	}
	molType := r.Annotations.MoleculeType
	if molType == "" {
		return fmt.Errorf("missing molecule type for record %q", r.ID)
	}
	if runeLen(molType) > 7 {
		molType = strings.ReplaceAll(strings.ReplaceAll(molType, "unassigned ", ""), "genomic ", "")
		if runeLen(molType) > 7 {
			molType = "DNA"
		}
	}
	if molType == "protein" || molType == "PROTEIN" {
		molType = ""
	}
	units := "bp"
	if molType == "" {
		units = "aa"
	}
	topology := strings.Repeat(" ", 8)
	if t := r.Annotations.Topology; t != "" && runeLen(t) <= 8 {
		topology = fmt.Sprintf("%-8s", t)
	}
	length := strconv.Itoa(len(r.Seq))
	var nameLength string
	if runeLen(locus) > 16 && len(length) > 11-(runeLen(locus)-16) {
		nameLength = locus + " " + length
	} else {
		padded := fmt.Sprintf("%28s", length)
		nameLength = locus + padded[runeLen(locus):]
	}
	line := fmt.Sprintf("LOCUS       %s %s    %-7s %s %s %s\n", nameLength, units, molType, topology,
		genBankDivision(r.Annotations.DataFileDivision), genBankDate(r.Annotations.Date))
	if runeLen(line) > 80 {
		fields := strings.Fields(line)
		if fields[3] != "bp" && fields[3] != "aa" {
			return fmt.Errorf("LOCUS line does not contain size units at expected position: %s", line)
		}
		molecule := strings.ToUpper(fields[4])
		if fields[3] != "aa" && !strings.Contains(molecule, "DNA") && !strings.Contains(molecule, "RNA") {
			return fmt.Errorf("LOCUS line does not contain valid sequence type (DNA, RNA, ...): %s", line)
		}
	}
	g.w.WriteString(line)
	return nil
}

func genBankDivision(division string) string {
	if genBankDivisions[division] {
		return division
	}
	if mapped, ok := emblToGenBankDivisions[division]; ok {
		return mapped
	}
	return "UNK"
}

// genBankDate returns date if it is a valid DD-MON-YYYY date, else Biopython's
// default.
func genBankDate(date string) string {
	const defaultDate = "01-JAN-1980"
	if len(date) != 11 {
		return defaultDate
	}
	year, okYear := pythonInt(date[7:])
	day, okDay := pythonInt(date[0:2])
	month := -1
	for i, m := range months {
		if date[3:6] == m {
			month = i + 1
		}
	}
	if !okYear || !okDay || month < 0 || year < 1 || year > 9999 {
		return defaultDate
	}
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if t.Day() != day || int(t.Month()) != month {
		return defaultDate
	}
	return date
}

func (g *GenBankWriter) writeReferences(r *Record) {
	for i, ref := range r.Annotations.References {
		data := strconv.Itoa(i + 1)
		if ref.Location != nil {
			units := "bases"
			if strings.Contains(r.Annotations.MoleculeType, "protein") {
				units = "residues"
			}
			data += fmt.Sprintf("  (%s %d to %d)", units, ref.Location[0]+1, ref.Location[1])
		}
		g.writeSingleLine("REFERENCE", data)
		for _, field := range []struct{ tag, text string }{
			{"  AUTHORS", ref.Authors},
			{"  CONSRTM", ref.Consrtm},
			{"  TITLE", ref.Title},
			{"  JOURNAL", ref.Journal},
			{"  MEDLINE", ref.MedlineID},
			{"   PUBMED", ref.PubmedID},
			{"  REMARK", ref.Comment},
		} {
			if field.text != "" {
				g.writeMultiLine(field.tag, field.text)
			}
		}
	}
}

func (g *GenBankWriter) writeComment(a Annotations) error {
	var lines []string
	if a.StructuredComments != nil {
		padding := 0
		for _, block := range a.StructuredComments {
			for _, field := range block.Fields {
				padding = max(padding, runeLen(field.Key))
			}
		}
		for _, block := range a.StructuredComments {
			lines = append(lines, "##"+block.Name+"-START##")
			for _, field := range block.Fields {
				lines = append(lines, field.Key+strings.Repeat(" ", padding-runeLen(field.Key))+" :: "+field.Value)
			}
			lines = append(lines, "##"+block.Name+"-END##")
		}
	}
	lines = append(lines, a.Comment...)
	if len(lines) == 0 {
		return fmt.Errorf("empty comment")
	}
	g.writeMultiLine("COMMENT", lines[0])
	for _, line := range lines[1:] {
		g.writeMultiLine("", line)
	}
	return nil
}

func (g *GenBankWriter) writeSequence(seq []byte) {
	data := lowerASCII(seq)
	g.w.WriteString("ORIGIN\n")
	for start := 0; start < len(data); start += 60 {
		fmt.Fprintf(g.w, "%9d", start+1)
		for block := start; block < min(start+60, len(data)); block += 10 {
			g.w.WriteByte(' ')
			g.w.Write(data[block:min(block+10, len(data))])
		}
		g.w.WriteByte('\n')
	}
}

// isDigits reports whether s is non-empty and all ASCII digits, like
// Python's str.isdigit for the text found in identifiers.
func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// pythonInt parses s as Python's int() does for the forms that occur here:
// optional surrounding whitespace and sign, and decimal digits.
func pythonInt(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil
}
