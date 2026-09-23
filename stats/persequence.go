package stats

import (
	"bufio"
	"fmt"
	"io"

	"github.com/martinghunt/faqt/seq"
	"github.com/martinghunt/faqt/seqio"
)

// perSequenceBufSize keeps one table row from costing a write syscall.
const perSequenceBufSize = 256 << 10

// SeqStats holds the statistics of a single sequence. Filename names the input
// the sequence came from, or "path:sample" for an AGC sample, so sequences
// sharing a name across inputs stay distinguishable.
type SeqStats struct {
	Filename  string
	Name      string
	Length    int
	NCount    int
	GapCount  int
	GCPercent float64
}

// PerSequence calls yield once per sequence, in input order, for every input
// and every AGC sample. Nothing is accumulated, so inputs far larger than
// memory are fine. An error from yield stops the walk and is returned.
func PerSequence(paths []string, minimumLength int, yield func(SeqStats) error, opts ...seqio.Option) error {
	for _, path := range paths {
		err := visitPathDatasets(path, func(name string, reader seqio.Reader) error {
			for {
				rec, err := reader.Read()
				if err == io.EOF {
					return nil
				}
				if err != nil {
					return err
				}
				if len(rec.Seq) < minimumLength {
					continue
				}
				comp := seq.CountComposition(rec.Seq)
				if err := yield(SeqStats{
					Filename:  name,
					Name:      rec.Name,
					Length:    len(rec.Seq),
					NCount:    comp.N,
					GapCount:  comp.NRuns,
					GCPercent: comp.GCPercent(),
				}); err != nil {
					return err
				}
			}
		}, opts...)
		if err != nil {
			return err
		}
	}
	return nil
}

// WritePerSequence streams per-sequence statistics to w as a tab-delimited
// table. Only FormatTab and FormatTabNoHeader are supported: one row per
// sequence has nothing to say in the human or grep friendly layouts, both of
// which key on the input rather than the sequence.
func WritePerSequence(w io.Writer, paths []string, minimumLength int, format Format, opts ...seqio.Option) error {
	switch format {
	case FormatTab, FormatTabNoHeader:
	default:
		return fmt.Errorf("per-sequence statistics support only tab-delimited output, not format %d", format)
	}

	buf := bufio.NewWriterSize(w, perSequenceBufSize)
	if format == FormatTab {
		if _, err := buf.WriteString(PerSequenceHeader()); err != nil {
			return err
		}
	}
	err := PerSequence(paths, minimumLength, func(s SeqStats) error {
		_, err := buf.WriteString(s.TabRecord())
		return err
	}, opts...)
	if err != nil {
		return err
	}
	return buf.Flush()
}

// PerSequenceHeader is the header line of the per-sequence table.
func PerSequenceHeader() string {
	return "file\tname\tlength\tN_count\tGaps\tGC\n"
}

// TabRecord is one row of the per-sequence table.
func (s SeqStats) TabRecord() string {
	return fmt.Sprintf("%s\t%s\t%d\t%d\t%d\t%.2f\n",
		s.Filename,
		s.Name,
		s.Length,
		s.NCount,
		s.GapCount,
		s.GCPercent,
	)
}
