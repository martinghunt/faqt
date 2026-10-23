package align

import (
	"context"
	"github.com/martinghunt/faqt/seq"
	"math/rand/v2"
	"testing"
)

// Independent cubic reference: enumerate every terminal gap length. This does
// not share the five-state recurrence or band layout with the implementation.
func referenceScore(q, t []byte, o Options) int {
	cols := len(t) + 1
	f := make([]int, (len(q)+1)*cols)
	for i := 0; i <= len(q); i++ {
		for j := 0; j <= len(t); j++ {
			if i+j == 0 {
				continue
			}
			best := minusInfinity
			if i > 0 && j > 0 {
				pair := -o.Mismatch
				if exact(q[i-1], t[j-1]) {
					pair = o.Match
				}
				best = f[(i-1)*cols+j-1] + pair
			}
			for n := 1; n <= i; n++ {
				best = max(best, f[(i-n)*cols+j]-gapCost(n, o))
			}
			for n := 1; n <= j; n++ {
				best = max(best, f[i*cols+j-n]-gapCost(n, o))
			}
			f[i*cols+j] = best
		}
	}
	return f[len(f)-1]
}
func TestDualAffineDPAgainstReference(t *testing.T) {
	o := DefaultOptions()
	o.ZDrop = 1000000
	o.BandWidth = 1
	r := rand.New(rand.NewPCG(41, 42))
	for n := 0; n < 200; n++ {
		q, targ := dna(r.IntN(20), uint64(n+50)), dna(r.IntN(20), uint64(n+1000))
		if n%4 == 0 && len(q) > 0 {
			q[len(q)/2] = 'N'
		}
		got, e := alignGap(context.Background(), q, targ, o)
		if e != nil {
			t.Fatal(e)
		}
		want := referenceScore(q, targ, o)
		if got.score != want {
			t.Fatalf("%s/%s score %d want %d ops=%+v", q, targ, got.score, want, got.ops)
		}
		i, j, score := 0, 0, 0
		for _, op := range got.ops {
			switch op.Code {
			case '=', 'X':
				for k := 0; k < op.Length; k++ {
					if exact(q[i], targ[j]) {
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
			}
		}
		if i != len(q) || j != len(targ) || score != want {
			t.Fatal("traceback disagrees with oracle")
		}
	}
}
func TestAdaptiveBandAndLongGap(t *testing.T) {
	o := DefaultOptions()
	o.BandWidth = 2
	o.ZDrop = 1000000
	q := dna(220, 43)
	target := append(append(slicesCopy(q[:100]), dna(80, 44)...), q[100:]...)
	got, e := alignGap(context.Background(), q, target, o)
	if e != nil {
		t.Fatal(e)
	}
	if got.score != referenceScore(q, target, o) {
		t.Fatalf("adaptive band score=%d", got.score)
	}
	if gapCost(80, o) != o.LongGapOpen+80*o.LongGapExtend {
		t.Fatal("long gap cost not selected")
	}
}
func TestAnchoredExtensionCannotRestart(t *testing.T) {
	o := DefaultOptions()
	o.ZDrop = 20
	q := dna(80, 45)
	target := append([]byte("NNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNNN"), q...)
	r, e := alignDP(context.Background(), q, target, 96, true, o)
	if e != nil {
		t.Fatal(e)
	}
	if r.score != 0 || r.q != 0 || r.t != 0 {
		t.Fatalf("extension restarted across unsupported flank %+v", r)
	}
}
func TestChainingSkipsSpuriousAnchorAndSharesPrefix(t *testing.T) {
	o := DefaultOptions()
	a := []anchor{{0, 0, 100}, {120, 500, 15}, {200, 200, 100}, {200, 800, 100}, {400, 400, 100}, {400, 1000, 100}}
	chains, e := chainAnchors(context.Background(), a, o)
	if e != nil {
		t.Fatal(e)
	}
	normal, alternate := false, false
	for _, c := range chains {
		if len(c) == 3 && c[0] == 0 && c[1] == 2 && c[2] == 4 {
			normal = true
		}
		if len(c) > 0 && c[len(c)-1] == 5 {
			alternate = true
		}
	}
	if !normal || !alternate {
		t.Fatalf("lost chain branch: %+v", chains)
	}
}
func TestStraySeedCannotPoisonFollowingRun(t *testing.T) {
	q := dna(18000, 46)
	target := append(append(slicesCopy(q[:14000]), dna(493, 47)...), q[14000:]...)
	o := DefaultOptions()
	anchors := []anchor{{0, 0, 15}, {14000, 14493, 4000}}
	chains, e := chainAnchors(context.Background(), anchors, o)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, c := range chains {
		ms, e := refineChain(context.Background(), q, target, anchors, c, o)
		if e != nil {
			t.Fatal(e)
		}
		for _, m := range ms {
			if m.A.Start <= 14000 && m.A.End == 18000 {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("stray seed hid valid downstream run")
	}
}
func TestDistinctPathsAreNotDeduplicated(t *testing.T) {
	a := Match{A: interval(0, 100), B: interval(0, 100), Ops: []Op{{'=', 100}}, Columns: 100}
	b := Match{A: interval(0, 100), B: interval(0, 100), Ops: []Op{{'=', 40}, {'I', 1}, {'=', 19}, {'D', 1}, {'=', 40}}, Columns: 101}
	if len(deduplicate([]Match{a, b})) != 2 {
		t.Fatal("different alignment paths discarded")
	}
	c := Match{A: interval(20, 80), B: interval(20, 80), Ops: []Op{{'=', 60}}, Columns: 60}
	if len(deduplicate([]Match{a, c})) != 1 {
		t.Fatal("identical subpath retained")
	}
}

func interval(start, end int) seq.Interval { return seq.Interval{Start: start, End: end} }

func TestAdaptiveBandBalancedIndels(t *testing.T) {
	o := DefaultOptions()
	o.BandWidth = 2
	o.ZDrop = 1000000
	q := dna(220, 71)
	target := append(append(append(slicesCopy(q[:60]), dna(50, 72)...), q[60:140]...), q[190:]...)
	got, e := alignGap(context.Background(), q, target, o)
	if e != nil {
		t.Fatal(e)
	}
	if want := referenceScore(q, target, o); got.score != want {
		t.Fatalf("balanced long indels: got %d want %d", got.score, want)
	}
}
