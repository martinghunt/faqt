package randomcontigs

import (
	"bytes"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/seqio"
)

func TestGenerateNamesAndLengths(t *testing.T) {
	var buf bytes.Buffer
	writer := seqio.NewFASTAWriter(&buf)
	seed := int64(7)
	err := Generate(writer, Options{
		Contigs:     2,
		Length:      3,
		Seed:        &seed,
		FirstNumber: 42,
		Prefix:      "p",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, ">p42\n") || !strings.Contains(got, ">p43\n") {
		t.Fatalf("output names not as expected:\n%s", got)
	}
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, ">") {
			continue
		}
		if len(line) != 3 {
			t.Fatalf("sequence line length = %d, want 3 in output:\n%s", len(line), got)
		}
		for _, base := range line {
			if !strings.ContainsRune("ACGT", base) {
				t.Fatalf("unexpected base %q in output:\n%s", base, got)
			}
		}
	}
}

func TestGenerateNameByLettersCycles(t *testing.T) {
	var buf bytes.Buffer
	writer := seqio.NewFASTAWriter(&buf)
	seed := int64(1)
	if err := Generate(writer, Options{Contigs: 28, Length: 1, NameByLetters: true, Seed: &seed}); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	got := buf.String()
	for _, name := range []string{">A\n", ">Z\n"} {
		if !strings.Contains(got, name) {
			t.Fatalf("missing name %q in output:\n%s", name, got)
		}
	}
	if strings.Count(got, ">A\n") != 2 || strings.Count(got, ">B\n") != 2 {
		t.Fatalf("expected A and B names to cycle in output:\n%s", got)
	}
}

func TestGenerateSeedIsDeterministic(t *testing.T) {
	seed := int64(99)
	var a bytes.Buffer
	var b bytes.Buffer
	if err := Generate(seqio.NewFASTAWriter(&a), Options{Contigs: 3, Length: 5, Seed: &seed}); err != nil {
		t.Fatalf("Generate(a) error = %v", err)
	}
	if err := Generate(seqio.NewFASTAWriter(&b), Options{Contigs: 3, Length: 5, Seed: &seed}); err != nil {
		t.Fatalf("Generate(b) error = %v", err)
	}
	if a.String() != b.String() {
		t.Fatalf("seeded output differs:\n%s\n%s", a.String(), b.String())
	}
}
