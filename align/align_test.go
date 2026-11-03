package align

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"

	"github.com/martinghunt/faqt/fasta"
	"github.com/martinghunt/faqt/seqio"
)

func dna(n int, seed uint64) []byte {
	r := rand.New(rand.NewPCG(seed, seed+91))
	b := make([]byte, n)
	for i := range b {
		b[i] = "ACGT"[r.IntN(4)]
	}
	return b
}
func genome(t testing.TB, sequences ...[]byte) *Genome {
	t.Helper()
	rs := make([]seqio.SeqRecord, len(sequences))
	for i, s := range sequences {
		rs[i] = seqio.SeqRecord{Name: fmt.Sprintf("contig%d", i), Seq: s}
	}
	g, e := Prepare(rs, SketchOptions{})
	if e != nil {
		t.Fatal(e)
	}
	return g
}
func compare(t testing.TB, a, b *Genome, o Options) []Match {
	t.Helper()
	m, e := Compare(context.Background(), a, b, o)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range m {
		replay(t, a, b, v, o)
	}
	swapped, e := Compare(context.Background(), b, a, o)
	if e != nil {
		t.Fatal(e)
	}
	for i := range swapped {
		swapped[i] = swapped[i].Swap()
	}
	SortMatches(swapped)
	if !reflect.DeepEqual(m, swapped) {
		t.Fatalf("input swap changed results:\n%+v\n%+v", m, swapped)
	}
	return m
}
func replay(t testing.TB, a, b *Genome, m Match, o Options) {
	t.Helper()
	q := a.contigs[m.AContig].bases[m.A.Start:m.A.End]
	r := b.contigs[m.BContig].bases[m.B.Start:m.B.End]
	if m.Reverse {
		r = reverseComplement(r)
	}
	i, j, matches, cols, score := 0, 0, 0, 0, 0
	for _, op := range m.Ops {
		if op.Length <= 0 {
			t.Fatal("nonpositive operation")
		}
		cols += op.Length
		switch op.Code {
		case '=', 'X':
			for z := 0; z < op.Length; z++ {
				if i >= len(q) || j >= len(r) {
					t.Fatal("paired op past interval")
				}
				eq := exact(q[i], r[j])
				if eq != (op.Code == '=') {
					t.Fatalf("incorrect %c operation at %d/%d", op.Code, i, j)
				}
				if eq {
					matches++
					score += o.Match
				} else {
					score -= o.Mismatch
				}
				i++
				j++
			}
		case 'I':
			i += op.Length
			score -= gapCost(op.Length, o)
		case 'D':
			j += op.Length
			score -= gapCost(op.Length, o)
		default:
			t.Fatalf("bad operation %q", op.Code)
		}
	}
	if i != len(q) || j != len(r) || matches != m.Matches || cols != m.Columns || score != m.Score {
		t.Fatalf("inconsistent alignment: %+v replay=%d/%d/%d/%d/%d", m, i, j, matches, cols, score)
	}
}
func hasPlacement(ms []Match, ac, bc, as, ae, bs, be int, rev bool) bool {
	for _, m := range ms {
		if m.AContig == ac && m.BContig == bc && m.Reverse == rev && m.A.Start <= as && m.A.End >= ae && m.B.Start <= bs && m.B.End >= be {
			return true
		}
	}
	return false
}

func TestExactAndReverse(t *testing.T) {
	s := dna(2000, 1)
	for _, rev := range []bool{false, true} {
		t.Run(fmt.Sprint(rev), func(t *testing.T) {
			b := s
			if rev {
				b = reverseComplement(s)
			}
			a, g := genome(t, s), genome(t, b)
			ms := compare(t, a, g, DefaultOptions())
			if !hasPlacement(ms, 0, 0, 0, len(s), 0, len(s), rev) {
				t.Fatalf("missing whole exact placement: %+v", ms)
			}
			for _, m := range ms {
				if m.Matches != 2000 || m.Identity() != 1 {
					t.Fatalf("unexpected exact hit %+v", m)
				}
			}
		})
	}
}
func TestAllRepeatPlacements(t *testing.T) {
	s := dna(400, 2)
	a := genome(t, s, s)
	var target [][]byte
	for i := 0; i < 7; i++ {
		target = append(target, s)
	}
	target = append(target, reverseComplement(s))
	b := genome(t, target...)
	ms := compare(t, a, b, DefaultOptions())
	if len(ms) != 16 {
		t.Fatalf("expected all 2x8 placements, got %d", len(ms))
	}
	for ac := 0; ac < 2; ac++ {
		for bc := 0; bc < 8; bc++ {
			if !hasPlacement(ms, ac, bc, 0, 400, 0, 400, bc == 7) {
				t.Fatalf("missing repeat %d/%d", ac, bc)
			}
		}
	}
}
func TestTandemRepeatLengthChange(t *testing.T) {
	unit := dna(60, 3)
	left, right := dna(1400, 4), dna(1400, 5)
	a := append(slicesCopy(left), []byte(strings.Repeat(string(unit), 3))...)
	a = append(a, right...)
	b := append(slicesCopy(left), []byte(strings.Repeat(string(unit), 6))...)
	b = append(b, right...)
	ga, gb := genome(t, a), genome(t, b)
	o := DefaultOptions()
	o.MinLength = 50
	o.MinChainScore = 25
	ms := compare(t, ga, gb, o)
	for i := 0; i < 3; i++ {
		for j := 0; j < 6; j++ {
			found := false
			for _, m := range ms {
				if m.Reverse {
					continue
				}
				if pathCoversPair(m, 1400+i*60, 1400+j*60, 60) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing tandem repeat copy %d/%d in %d matches", i, j, len(ms))
			}
		}
	}
}
func slicesCopy(b []byte) []byte { return append([]byte(nil), b...) }
func pathCoversPair(m Match, q, t, n int) bool {
	p, r := m.A.Start, m.B.Start
	for _, op := range m.Ops {
		if op.Code == '=' && q >= p && q+n <= p+op.Length && t-r == q-p {
			return true
		}
		if op.Code != 'D' {
			p += op.Length
		}
		if op.Code != 'I' {
			r += op.Length
		}
	}
	return false
}
func TestIndelsSNPsAndInversion(t *testing.T) {
	base := dna(2400, 6)
	for _, test := range []struct {
		name string
		b    []byte
	}{{"snp", func() []byte {
		b := slicesCopy(base)
		for i := 250; i < 2200; i += 137 {
			b[i] = "ACGT"[(strings.IndexByte("ACGT", b[i])+1)%4]
		}
		return b
	}()}, {"insertion", append(append(slicesCopy(base[:1200]), dna(180, 7)...), base[1200:]...)}, {"inversion", append(append(slicesCopy(base[:800]), reverseComplement(base[800:1600])...), base[1600:]...)}} {
		t.Run(test.name, func(t *testing.T) {
			ms := compare(t, genome(t, base), genome(t, test.b), DefaultOptions())
			if len(ms) == 0 {
				t.Fatal("no matches")
			}
			covered := make([]bool, len(base))
			for _, m := range ms {
				for i := m.A.Start; i < m.A.End; i++ {
					covered[i] = true
				}
			}
			missing := 0
			for _, c := range covered {
				if !c {
					missing++
				}
			}
			if missing > 40 {
				t.Fatalf("lost %d bases", missing)
			}
		})
	}
}
func TestSharedFlanksAndNestedDuplication(t *testing.T) {
	p, r, s := dna(500, 8), dna(500, 9), dna(500, 10)
	a := append(append(slicesCopy(p), r...), s...)
	b := append(append(append(slicesCopy(p), r...), r...), s...)
	ms := compare(t, genome(t, a), genome(t, b), DefaultOptions())
	for _, bs := range []int{500, 1000} {
		found := false
		for _, m := range ms {
			if pathCoversPair(m, 500, bs, 500) {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing nested copy at %d", bs)
		}
	}
}
func TestAmbiguityNormalizationAndUnrelated(t *testing.T) {
	s := dna(500, 11)
	for _, test := range []struct {
		name string
		a, b []byte
		want bool
	}{{"case", []byte(strings.ToLower(string(s))), s, true}, {"ambiguity", []byte(strings.Repeat("N", 500)), []byte(strings.Repeat("N", 500)), false}, {"unrelated", s, dna(500, 12), false}, {"empty", nil, s, false}} {
		t.Run(test.name, func(t *testing.T) {
			ms := compare(t, genome(t, test.a), genome(t, test.b), DefaultOptions())
			if (len(ms) > 0) != test.want {
				t.Fatalf("unexpected matches %d", len(ms))
			}
		})
	}
}
func TestNoCrossContigAlignment(t *testing.T) {
	s := dna(800, 13)
	ms := compare(t, genome(t, s[:400], s[400:]), genome(t, s), DefaultOptions())
	if !hasPlacement(ms, 0, 0, 0, 400, 0, 400, false) || !hasPlacement(ms, 1, 0, 0, 400, 400, 800, false) {
		t.Fatal("missing contig-local matches")
	}
}
func TestCancelledAndResourceLimits(t *testing.T) {
	a := genome(t, dna(1000, 14))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ms, e := Compare(ctx, a, a, DefaultOptions()); !errors.Is(e, context.Canceled) || ms != nil {
		t.Fatalf("cancellation: %v", e)
	}
	o := DefaultOptions()
	o.MaxAnchors = 1
	if ms, e := Compare(context.Background(), a, a, o); e == nil || ms != nil {
		t.Fatal("anchor exhaustion must fail atomically")
	} else {
		var limit *LimitError
		if !errors.As(e, &limit) {
			t.Fatal(e)
		}
	}
	o = DefaultOptions()
	o.MaxCells = 2
	mutated := slicesCopy(a.contigs[0].bases)
	mutated[500] = "ACGT"[(strings.IndexByte("ACGT", mutated[500])+1)%4]
	b := genome(t, mutated)
	if ms, e := Compare(context.Background(), a, b, o); e == nil || ms != nil {
		t.Fatal("cell exhaustion must fail atomically")
	}
	o = DefaultOptions()
	o.Progress = func(p Progress) { cancel() }
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	b = genome(t, a.contigs[0].bases, a.contigs[0].bases)
	if ms, e := Compare(ctx, a, b, o); !errors.Is(e, context.Canceled) || ms != nil {
		t.Fatalf("progress cancellation: %v", e)
	}
}
func TestInvalidOptionsAndInput(t *testing.T) {
	for _, s := range []SketchOptions{{K: 2}, {K: 32}, {W: 256}, {K: -1}} {
		if _, e := Prepare(nil, s); e == nil {
			t.Fatal("accepted invalid sketch")
		}
	}
	if _, e := Prepare([]seqio.SeqRecord{{Seq: []byte("ACG-")}}, SketchOptions{}); e == nil {
		t.Fatal("accepted gap")
	}
	a := genome(t, dna(100, 16))
	for _, value := range []float64{-1, 1.1, math.NaN()} {
		o := DefaultOptions()
		o.MinIdentity = value
		if _, e := Compare(context.Background(), a, a, o); e == nil {
			t.Fatal("accepted identity")
		}
	}
	b, e := Prepare(nil, SketchOptions{K: 17})
	if e != nil {
		t.Fatal(e)
	}
	if _, e := Compare(context.Background(), a, b, DefaultOptions()); e == nil {
		t.Fatal("accepted incompatible sketches")
	}
}
func TestReadGenomeOwnsSequenceAndProgress(t *testing.T) {
	s := dna(200, 17)
	reader := fasta.NewReader(bufio.NewReader(strings.NewReader(">one\n" + string(s) + "\n>two\n" + string(s) + "\n")))
	g, e := ReadGenome(reader, SketchOptions{})
	if e != nil {
		t.Fatal(e)
	}
	if g.ContigCount() != 2 || g.ContigName(1) != "two" || g.ContigLength(0) != 200 {
		t.Fatal("bad genome metadata")
	}
	calls := 0
	o := DefaultOptions()
	o.Progress = func(p Progress) {
		calls++
		if p.Completed != calls || p.Total != 8 {
			t.Fatalf("bad progress %+v", p)
		}
	}
	ms, e := Compare(context.Background(), g, g, o)
	if e != nil || len(ms) != 4 || calls != 8 {
		t.Fatalf("matches=%d progress=%d err=%v", len(ms), calls, e)
	}
	rs := []seqio.SeqRecord{{Seq: s}}
	g, e = Prepare(rs, SketchOptions{})
	if e != nil {
		t.Fatal(e)
	}
	s[0] = 'N'
	if g.contigs[0].bases[0] == 'N' {
		t.Fatal("Prepare retained caller memory")
	}
}
func TestSwapWithReverseIndels(t *testing.T) {
	s := dna(1500, 18)
	b := append(append(slicesCopy(s[:700]), dna(31, 19)...), s[700:]...)
	ms := compare(t, genome(t, s), genome(t, reverseComplement(b)), DefaultOptions())
	found := false
	for _, m := range ms {
		if m.Reverse && strings.Contains(m.CIGAR(), "D") {
			found = true
		}
		if !reflect.DeepEqual(m, m.Swap().Swap()) {
			t.Fatal("Swap not involutive")
		}
	}
	if !found {
		t.Fatal("missing reverse indel")
	}
}
func BenchmarkCompareBacterialLike(b *testing.B) {
	s := dna(200000, 20)
	target := slicesCopy(s)
	for i := 1000; i < len(s); i += 1000 {
		target[i] = "ACGT"[(strings.IndexByte("ACGT", target[i])+1)%4]
	}
	a, g := genome(b, s), genome(b, target)
	o := DefaultOptions()
	b.ResetTimer()
	for b.Loop() {
		if _, e := Compare(context.Background(), a, g, o); e != nil {
			b.Fatal(e)
		}
	}
}

func FuzzCompareInvariants(f *testing.F) {
	f.Add([]byte("ACGTACGT"), byte(0))
	f.Add([]byte("GATTACAGTCCGAT"), byte(1))
	f.Add([]byte("AAAAAAAAAAAAAAAA"), byte(2))
	f.Fuzz(func(t *testing.T, data []byte, mode byte) {
		if len(data) > 120 {
			data = data[:120]
		}
		a := make([]byte, len(data))
		for i, v := range data {
			a[i] = "ACGT"[v%4]
		}
		b := slicesCopy(a)
		switch mode % 4 {
		case 0:
			if len(b) > 0 {
				b[len(b)/2] = "ACGT"[(strings.IndexByte("ACGT", b[len(b)/2])+1)%4]
			}
		case 1:
			b = reverseComplement(b)
		case 2:
			b = append(b, a...)
		case 3:
			if len(b) > 8 {
				b = append(b[:len(b)/2], b[len(b)/2+3:]...)
			}
		}
		prepare := func(s []byte) *Genome {
			g, e := Prepare([]seqio.SeqRecord{{Name: "x", Seq: s}}, SketchOptions{K: 5, W: 3})
			if e != nil {
				t.Fatal(e)
			}
			return g
		}
		o := DefaultOptions()
		o.MinLength = 10
		o.MinChainScore = 5
		o.MinScore = 5
		o.MaxGap = 50
		o.Extension = 16
		o.BandWidth = 8
		compare(t, prepare(a), prepare(b), o)
	})
}

func TestConcurrentPreparedGenomes(t *testing.T) {
	a, b := genome(t, dna(700, 121)), genome(t, dna(700, 121))
	want, e := Compare(context.Background(), a, b, DefaultOptions())
	if e != nil {
		t.Fatal(e)
	}
	errs := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			got, e := Compare(context.Background(), a, b, DefaultOptions())
			if e == nil && !reflect.DeepEqual(got, want) {
				e = fmt.Errorf("concurrent result changed")
			}
			errs <- e
		}()
	}
	for i := 0; i < 4; i++ {
		if e := <-errs; e != nil {
			t.Fatal(e)
		}
	}
}
func TestNamesDoNotChangeSymmetry(t *testing.T) {
	s := dna(600, 122)
	a := genome(t, s, s)
	b := genome(t, s, s)
	for i := range a.contigs {
		a.contigs[i].name = "same-name"
		b.contigs[i].name = fmt.Sprintf("different-%d", i)
	}
	ms := compare(t, a, b, DefaultOptions())
	if len(ms) != 4 {
		t.Fatalf("equal-content genomes lost placements: %d", len(ms))
	}
}
