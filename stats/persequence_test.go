package stats_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/stats"
)

// The shared fixture is >a NNGGAAAA, >b GGGCCC, >c ACGT, >d NN, >e A, so the
// rows below cover Ns inside a sequence, a sequence of nothing but Ns, and
// every GC value from 0 to 100.
func perSequenceFixtureRows(path string) string {
	return path + "\ta\t8\t2\t1\t33.33\n" +
		path + "\tb\t6\t0\t0\t100.00\n" +
		path + "\tc\t4\t0\t0\t50.00\n" +
		path + "\td\t2\t2\t1\t0.00\n" +
		path + "\te\t1\t0\t0\t0.00\n"
}

func TestWritePerSequence(t *testing.T) {
	path := writeStatsFixture(t)
	var buf strings.Builder
	if err := stats.WritePerSequence(&buf, []string{path}, 1, stats.FormatTab); err != nil {
		t.Fatalf("WritePerSequence() error = %v", err)
	}

	want := "file\tname\tlength\tN_count\tGaps\tGC\n" + perSequenceFixtureRows(path)
	if got := buf.String(); got != want {
		t.Fatalf("per-sequence output = %q, want %q", got, want)
	}
}

func TestWritePerSequenceNoHeader(t *testing.T) {
	path := writeStatsFixture(t)
	var buf strings.Builder
	if err := stats.WritePerSequence(&buf, []string{path}, 1, stats.FormatTabNoHeader); err != nil {
		t.Fatalf("WritePerSequence() error = %v", err)
	}

	want := perSequenceFixtureRows(path)
	if got := buf.String(); got != want {
		t.Fatalf("per-sequence output = %q, want %q", got, want)
	}
}

func TestWritePerSequenceRejectsNonTabFormats(t *testing.T) {
	path := writeStatsFixture(t)
	for _, format := range []stats.Format{stats.FormatHuman, stats.FormatGreppy} {
		var buf strings.Builder
		if err := stats.WritePerSequence(&buf, []string{path}, 1, format); err == nil {
			t.Errorf("WritePerSequence(format %d) expected an error", format)
		}
		if buf.Len() != 0 {
			t.Errorf("WritePerSequence(format %d) wrote %q, want nothing", format, buf.String())
		}
	}
}

func TestPerSequenceMinimumLength(t *testing.T) {
	path := writeStatsFixture(t)
	var names []string
	err := stats.PerSequence([]string{path}, 5, func(s stats.SeqStats) error {
		names = append(names, s.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("PerSequence() error = %v", err)
	}
	if strings.Join(names, ",") != "a,b" {
		t.Fatalf("names = %v, want a,b", names)
	}
}

func TestPerSequenceKeepsInputsApart(t *testing.T) {
	dir := t.TempDir()
	one := filepath.Join(dir, "one.fa")
	two := filepath.Join(dir, "two.fa")
	// The same sequence name in both inputs: only the file column tells the
	// two rows apart.
	if err := os.WriteFile(one, []byte(">contig1\nGGCC\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(two, []byte(">contig1\nAATT\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf strings.Builder
	if err := stats.WritePerSequence(&buf, []string{one, two}, 1, stats.FormatTabNoHeader); err != nil {
		t.Fatalf("WritePerSequence() error = %v", err)
	}
	want := one + "\tcontig1\t4\t0\t0\t100.00\n" + two + "\tcontig1\t4\t0\t0\t0.00\n"
	if got := buf.String(); got != want {
		t.Fatalf("per-sequence output = %q, want %q", got, want)
	}
}

func TestPerSequenceNamesAGCSamples(t *testing.T) {
	path := writeStatsAGC(t)
	var rows []stats.SeqStats
	err := stats.PerSequence([]string{path}, 1, func(s stats.SeqStats) error {
		rows = append(rows, s)
		return nil
	})
	if err != nil {
		t.Fatalf("PerSequence() error = %v", err)
	}
	// The archive holds 13 contigs across samples ref, a, b and c.
	if len(rows) != 13 {
		t.Fatalf("row count = %d, want 13", len(rows))
	}
	for _, sample := range []string{"ref", "a", "b", "c"} {
		found := false
		for _, row := range rows {
			if row.Filename == path+":"+sample {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no rows for sample %q", sample)
		}
	}
	for _, row := range rows {
		if row.Length == 0 || row.Name == "" {
			t.Errorf("row = %+v, want a named sequence with a length", row)
		}
	}
}

func TestPerSequenceStopsOnYieldError(t *testing.T) {
	path := writeStatsFixture(t)
	sentinel := errors.New("stop")
	seen := 0
	err := stats.PerSequence([]string{path}, 1, func(stats.SeqStats) error {
		seen++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("PerSequence() error = %v, want %v", err, sentinel)
	}
	if seen != 1 {
		t.Fatalf("yield called %d times, want 1", seen)
	}
}
