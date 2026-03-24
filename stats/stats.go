package stats

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/martinghunt/faqt/seqio"
)

type Format int

const (
	FormatHuman Format = iota
	FormatTab
	FormatTabNoHeader
	FormatGreppy
)

type Stats struct {
	Filename    string
	TotalLength int
	Number      int
	MeanLength  float64
	Longest     int
	Shortest    int
	NCount      int
	GapCount    int
	nxx         [9]int
	nxxn        [9]int
}

func FromPath(path string, minimumLength int) (Stats, error) {
	reader, err := seqio.OpenPath(path)
	if err != nil {
		return Stats{}, err
	}
	if closer, ok := reader.(io.Closer); ok {
		defer closer.Close()
	}

	s := Stats{Filename: path}
	lengths := make([]int, 0, 128)
	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Stats{}, err
		}
		if len(rec.Seq) < minimumLength {
			continue
		}
		l := len(rec.Seq)
		lengths = append(lengths, l)
		s.TotalLength += l
		nCount, gapCount := countNsAndGaps(rec.Seq)
		s.NCount += nCount
		s.GapCount += gapCount
	}

	s.finish(lengths)
	return s, nil
}

func (s *Stats) finish(lengths []int) {
	if len(lengths) == 0 {
		return
	}
	sort.Ints(lengths)
	s.Number = len(lengths)
	s.Shortest = lengths[0]
	s.Longest = lengths[len(lengths)-1]
	s.MeanLength = float64(s.TotalLength) / float64(s.Number)

	cumulativeLength := 0
	k := 0
	for i := len(lengths) - 1; i >= 0; i-- {
		cumulativeLength += lengths[i]
		for k < 9 && float64(cumulativeLength) >= float64(s.TotalLength)*float64(k+1)/10.0 {
			s.nxx[k] = lengths[i]
			s.nxxn[k] = len(lengths) - i
			k++
		}
	}
}

func (s Stats) String(format Format) string {
	switch format {
	case FormatHuman:
		return s.humanString()
	case FormatTab:
		return s.tabString(true)
	case FormatTabNoHeader:
		return s.tabString(false)
	case FormatGreppy:
		return s.greppyString()
	default:
		panic(fmt.Sprintf("unsupported stats format %d", format))
	}
}

func RenderMany(all []Stats, format Format) string {
	var buf strings.Builder
	switch format {
	case FormatTab:
		if len(all) == 0 {
			return ""
		}
		buf.WriteString(tabHeader())
		for _, s := range all {
			buf.WriteString(s.tabRecord())
		}
	default:
		for _, s := range all {
			buf.WriteString(s.String(format))
		}
	}
	return buf.String()
}

func (s Stats) humanString() string {
	return fmt.Sprintf(
		"stats for %s\nsum = %d, n = %d, ave = %.2f, largest = %d\nN50 = %d, n = %d\nN60 = %d, n = %d\nN70 = %d, n = %d\nN80 = %d, n = %d\nN90 = %d, n = %d\nN100 = %d, n = %d\nN_count = %d\nGaps = %d\n",
		s.Filename,
		s.TotalLength,
		s.Number,
		s.MeanLength,
		s.Longest,
		s.nxx[4], s.nxxn[4],
		s.nxx[5], s.nxxn[5],
		s.nxx[6], s.nxxn[6],
		s.nxx[7], s.nxxn[7],
		s.nxx[8], s.nxxn[8],
		s.Shortest, s.Number,
		s.NCount,
		s.GapCount,
	)
}

func (s Stats) greppyString() string {
	var buf strings.Builder
	fmt.Fprintf(&buf, "%s\ttotal_length\t%d\n", s.Filename, s.TotalLength)
	fmt.Fprintf(&buf, "%s\tnumber\t%d\n", s.Filename, s.Number)
	fmt.Fprintf(&buf, "%s\tmean_length\t%.2f\n", s.Filename, s.MeanLength)
	fmt.Fprintf(&buf, "%s\tlongest\t%d\n", s.Filename, s.Longest)
	fmt.Fprintf(&buf, "%s\tshortest\t%d\n", s.Filename, s.Shortest)
	fmt.Fprintf(&buf, "%s\tN_count\t%d\n", s.Filename, s.NCount)
	fmt.Fprintf(&buf, "%s\tGaps\t%d\n", s.Filename, s.GapCount)
	for j := 0; j < 9; j++ {
		fmt.Fprintf(&buf, "%s\tn%d0\t%d\n", s.Filename, j+1, s.nxx[j])
		fmt.Fprintf(&buf, "%s\tn%d0n\t%d\n", s.Filename, j+1, s.nxxn[j])
	}
	return buf.String()
}

func tabHeader() string {
	return "filename\ttotal_length\tnumber\tmean_length\tlongest\tshortest\tN_count\tGaps\tN50\tN50n\tN70\tN70n\tN90\tN90n\n"
}

func (s Stats) tabRecord() string {
	return fmt.Sprintf("%s\t%d\t%d\t%.2f\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\t%d\n",
		s.Filename,
		s.TotalLength,
		s.Number,
		s.MeanLength,
		s.Longest,
		s.Shortest,
		s.NCount,
		s.GapCount,
		s.nxx[4], s.nxxn[4],
		s.nxx[6], s.nxxn[6],
		s.nxx[8], s.nxxn[8],
	)
}

func (s Stats) tabString(header bool) string {
	if header {
		return tabHeader() + s.tabRecord()
	}
	return s.tabRecord()
}

func countNsAndGaps(seq []byte) (nCount int, gapCount int) {
	inGap := false
	for _, b := range seq {
		if b == 'N' || b == 'n' {
			nCount++
			if !inGap {
				gapCount++
				inGap = true
			}
			continue
		}
		inGap = false
	}
	return nCount, gapCount
}

func RemoveDashes(rec *seqio.SeqRecord) (*seqio.SeqRecord, error) {
	copyRec := *rec
	copyRec.Seq = bytes.ReplaceAll(rec.Seq, []byte("-"), nil)
	return &copyRec, nil
}
