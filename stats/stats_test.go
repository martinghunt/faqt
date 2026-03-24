package stats_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/martinghunt/faqt/stats"
)

func TestStatsHumanReadable(t *testing.T) {
	path := writeStatsFixture(t)
	s, err := stats.FromPath(path, 1)
	if err != nil {
		t.Fatalf("FromPath() error = %v", err)
	}
	expected := "stats for " + path + "\n" +
		"sum = 21, n = 5, ave = 4.20, largest = 8\n" +
		"N50 = 6, n = 2\n" +
		"N60 = 6, n = 2\n" +
		"N70 = 4, n = 3\n" +
		"N80 = 4, n = 3\n" +
		"N90 = 2, n = 4\n" +
		"N100 = 1, n = 5\n" +
		"N_count = 4\n" +
		"Gaps = 2\n"
	if got := s.String(stats.FormatHuman); got != expected {
		t.Fatalf("human output = %q, want %q", got, expected)
	}
}

func TestStatsGreppy(t *testing.T) {
	path := writeStatsFixture(t)
	s, err := stats.FromPath(path, 1)
	if err != nil {
		t.Fatalf("FromPath() error = %v", err)
	}
	expected := path + "\ttotal_length\t21\n" +
		path + "\tnumber\t5\n" +
		path + "\tmean_length\t4.20\n" +
		path + "\tlongest\t8\n" +
		path + "\tshortest\t1\n" +
		path + "\tN_count\t4\n" +
		path + "\tGaps\t2\n" +
		path + "\tn10\t8\n" +
		path + "\tn10n\t1\n" +
		path + "\tn20\t8\n" +
		path + "\tn20n\t1\n" +
		path + "\tn30\t8\n" +
		path + "\tn30n\t1\n" +
		path + "\tn40\t6\n" +
		path + "\tn40n\t2\n" +
		path + "\tn50\t6\n" +
		path + "\tn50n\t2\n" +
		path + "\tn60\t6\n" +
		path + "\tn60n\t2\n" +
		path + "\tn70\t4\n" +
		path + "\tn70n\t3\n" +
		path + "\tn80\t4\n" +
		path + "\tn80n\t3\n" +
		path + "\tn90\t2\n" +
		path + "\tn90n\t4\n"
	if got := s.String(stats.FormatGreppy); got != expected {
		t.Fatalf("greppy output = %q, want %q", got, expected)
	}
}

func TestStatsTabDelimited(t *testing.T) {
	path := writeStatsFixture(t)
	s, err := stats.FromPath(path, 1)
	if err != nil {
		t.Fatalf("FromPath() error = %v", err)
	}
	expected := "filename\ttotal_length\tnumber\tmean_length\tlongest\tshortest\tN_count\tGaps\tN50\tN50n\tN70\tN70n\tN90\tN90n\n" +
		path + "\t21\t5\t4.20\t8\t1\t4\t2\t6\t2\t4\t3\t2\t4\n"
	if got := s.String(stats.FormatTab); got != expected {
		t.Fatalf("tab output = %q, want %q", got, expected)
	}
}

func TestStatsTabDelimitedNoHeader(t *testing.T) {
	path := writeStatsFixture(t)
	s, err := stats.FromPath(path, 1)
	if err != nil {
		t.Fatalf("FromPath() error = %v", err)
	}
	expected := path + "\t21\t5\t4.20\t8\t1\t4\t2\t6\t2\t4\t3\t2\t4\n"
	if got := s.String(stats.FormatTabNoHeader); got != expected {
		t.Fatalf("tab no header output = %q, want %q", got, expected)
	}
}

func TestMinimumLength(t *testing.T) {
	path := writeStatsFixture(t)
	s, err := stats.FromPath(path, 5)
	if err != nil {
		t.Fatalf("FromPath() error = %v", err)
	}
	if s.TotalLength != 14 || s.Number != 2 || s.Shortest != 6 || s.Longest != 8 {
		t.Fatalf("stats = %+v", s)
	}
}

func writeStatsFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stats_unittest.fasta")
	data := "" +
		">a\nNNAAAAAA\n" +
		">b\nAAAAAA\n" +
		">c\nAAAA\n" +
		">d\nNN\n" +
		">e\nA\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}
