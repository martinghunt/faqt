package flatseq

import "testing"

func TestSequenceLetters(t *testing.T) {
	got := string(SequenceLetters("  1 ACGT nn 123 -x"))
	if got != "acgtnnx" {
		t.Fatalf("SequenceLetters() = %q, want %q", got, "acgtnnx")
	}
}

func TestAppendDescription(t *testing.T) {
	tests := []struct {
		name string
		desc string
		text string
		want string
	}{
		{name: "empty text", desc: "first", text: "   ", want: "first"},
		{name: "first line", text: " first ", want: "first"},
		{name: "continuation", desc: "first", text: " second ", want: "first second"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := AppendDescription(tc.desc, tc.text); got != tc.want {
				t.Fatalf("AppendDescription() = %q, want %q", got, tc.want)
			}
		})
	}
}
