package flatseq

import "strings"

func SequenceLetters(line string) []byte {
	out := make([]byte, 0, len(line))
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if ch >= 'A' && ch <= 'Z' {
			ch += 'a' - 'A'
		}
		if ch >= 'a' && ch <= 'z' {
			out = append(out, ch)
		}
	}
	return out
}

func AppendDescription(desc, text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return desc
	}
	if desc == "" {
		return text
	}
	return desc + " " + text
}
