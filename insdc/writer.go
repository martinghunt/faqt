package insdc

import (
	"bufio"
	"fmt"
	"strings"
	"unicode"
)

const (
	maxWidth         = 80
	qualifierIndent  = 21
	locationMaxWidth = maxWidth - qualifierIndent
)

// unquotedQualifiers are the keys Biopython writes without quotes.
var unquotedQualifiers = map[string]bool{
	"anticodon":        true,
	"citation":         true,
	"codon_start":      true,
	"compare":          true,
	"direction":        true,
	"estimated_length": true,
	"mod_base":         true,
	"number":           true,
	"rpt_type":         true,
	"rpt_unit_range":   true,
	"tag_peptide":      true,
	"transl_except":    true,
	"transl_table":     true,
}

// featureWriter writes the feature table shared by GenBank and EMBL, which
// differ only in the indent of continuation lines and the feature line prefix.
type featureWriter struct {
	w *bufio.Writer
	// indent starts qualifier lines: 21 spaces, or "FT" and 19 spaces.
	indent string
	// prefix starts feature key lines: 5 spaces, or "FT" and 3 spaces.
	prefix string
}

func (fw featureWriter) writeFeature(f Feature, recordLength int) error {
	location, err := locationString(f.Location, recordLength)
	if err != nil {
		return err
	}
	featureType := strings.ReplaceAll(f.Type, " ", "_")
	// Biopython pads the key to column 21 and cuts it there.
	head := []rune(fw.prefix + featureType + strings.Repeat(" ", 16))
	if len(head) > qualifierIndent {
		head = head[:qualifierIndent]
	}
	fw.w.WriteString(string(head))
	fw.w.WriteString(fw.wrapLocation(location))
	fw.w.WriteByte('\n')
	for _, q := range f.Qualifiers {
		for _, v := range q.Values {
			fw.writeQualifier(q.Key, v)
		}
	}
	return nil
}

// wrapLocation splits a long location after commas.
func (fw featureWriter) wrapLocation(location string) string {
	if len(location) <= locationMaxWidth {
		return location
	}
	index := strings.LastIndex(location[:locationMaxWidth], ",")
	if index == -1 {
		return location
	}
	return location[:index+1] + "\n" + fw.indent + fw.wrapLocation(location[index+1:])
}

// writeQualifier is Biopython's _write_feature_qualifier, line wrapping
// included.
func (fw featureWriter) writeQualifier(key string, v Value) {
	if v.kind == flagValue {
		fmt.Fprintf(fw.w, "%s/%s\n", fw.indent, key)
		return
	}
	value := v.text
	quote := v.kind == textValue && !unquotedQualifiers[key]
	if v.kind == textValue {
		value = strings.ReplaceAll(value, `"`, `""`)
	}
	var line []rune
	if quote {
		line = []rune(fw.indent + "/" + key + `="` + value + `"`)
	} else {
		line = []rune(fw.indent + "/" + key + "=" + value)
	}
	if len(line) <= maxWidth {
		fw.w.WriteString(string(line) + "\n")
		return
	}
	for strings.TrimLeftFunc(string(line), unicode.IsSpace) != "" {
		if len(line) <= maxWidth {
			fw.w.WriteString(string(line) + "\n")
			return
		}
		// Break at the last space up to column 80, but not in the indent.
		index := qualifierIndent + 2
		for i := min(len(line)-1, maxWidth); i > qualifierIndent+1; i-- {
			index = i
			if line[i] == ' ' {
				break
			}
		}
		if line[index] != ' ' {
			index = maxWidth
		}
		fw.w.WriteString(string(line[:index]) + "\n")
		line = []rune(fw.indent + strings.TrimLeftFunc(string(line[index:]), unicode.IsSpace))
	}
}

// splitMultiLine is Biopython's _split_multi_line: word wrapping to maxLen
// characters, with over-long words on their own line.
func splitMultiLine(text string, maxLen int) []string {
	text = strings.TrimFunc(text, unicode.IsSpace)
	if runeLen(text) <= maxLen {
		return []string{text}
	}
	words := strings.Fields(text)
	var lines []string
	line := ""
	for len(words) > 0 && runeLen(line)+1+runeLen(words[0]) <= maxLen {
		line = strings.TrimFunc(line+" "+words[0], unicode.IsSpace)
		words = words[1:]
	}
	lines = append(lines, line)
	for len(words) > 0 {
		line = words[0]
		words = words[1:]
		for len(words) > 0 && runeLen(line)+1+runeLen(words[0]) <= maxLen {
			line = strings.TrimFunc(line+" "+words[0], unicode.IsSpace)
			words = words[1:]
		}
		lines = append(lines, line)
	}
	return lines
}

func runeLen(s string) int {
	return len([]rune(s))
}

// lowerASCII lower-cases a sequence, as Python's str.lower does for the
// letters sequences contain.
func lowerASCII(seq []byte) []byte {
	out := make([]byte, len(seq))
	for i, b := range seq {
		if 'A' <= b && b <= 'Z' {
			b += 'a' - 'A'
		}
		out[i] = b
	}
	return out
}

// firstOr returns the first item, or def if there is none.
func firstOr(items []string, def string) string {
	if len(items) == 0 {
		return def
	}
	return items[0]
}

func stringOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return *s
}
