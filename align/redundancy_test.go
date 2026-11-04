package align

import (
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestNormalizeShortIndels(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		q, target := []byte("CAAAAAGT"), []byte("CAAAAGT")
		m := containmentFixture(0, 0, false, []Op{{'=', 3}, {'I', 1}, {'=', 4}})
		if reverse {
			target = reverseComplement(target)
			m.Reverse = true
		}
		a, b := genome(t, q), genome(t, target)
		saved := slices.Clone(m.Ops)
		got := normalizeMatch(m, a, b, DefaultOptions())
		replay(t, a, b, got, DefaultOptions())
		if got.CIGAR() != "1=1I6=" || got.Score != m.Score || got.A != m.A || got.B != m.B {
			t.Fatalf("unexpected normalized path: %+v", got)
		}
		if !slices.Equal(m.Ops, saved) {
			t.Fatal("normalization mutated a shared operation slice")
		}
		if again := normalizeMatch(got, a, b, DefaultOptions()); !reflect.DeepEqual(got, again) {
			t.Fatal("normalization was not idempotent")
		}
	}
}

func TestNearContainedProtectsNovelPlacements(t *testing.T) {
	big := containmentFixture(0, 0, false, []Op{{'=', 20000}})
	small := containmentFixture(0, 0, false, []Op{{'=', 10000}, {'I', 5}, {'=', 10}, {'D', 5}, {'=', 9985}})
	if !nearContained(pairedMatch(big), pairedMatch(small), 100) {
		t.Fatal("nearly identical micro-gap path was retained")
	}
	// The overall fraction is still >99%, but the new stretch at the path's
	// end is large enough to qualify as a separate placement.
	novel := containmentFixture(0, 0, false, []Op{{'=', 19800}, {'I', 1}, {'=', 100}})
	if nearContained(pairedMatch(big), pairedMatch(novel), 100) {
		t.Fatal("independently reportable novel end stretch was suppressed")
	}
	// The same stretch between shared ends is an internal detour.
	detour := containmentFixture(0, 0, false, []Op{{'=', 10000}, {'I', 1}, {'=', 100}, {'D', 1}, {'=', 9899}})
	if !nearContained(pairedMatch(big), pairedMatch(detour), 100) {
		t.Fatal("internal detour with shared ends was retained")
	}
	for _, change := range []func(*Match){
		func(m *Match) { m.B.Start += 30000; m.B.End += 30000 },
		func(m *Match) { m.AContig++ },
		func(m *Match) { m.BContig++ },
		func(m *Match) { m.Reverse = true },
		func(m *Match) { m.Score = big.Score + 1 },
	} {
		m := small
		change(&m)
		if nearContained(pairedMatch(big), pairedMatch(m), 100) {
			t.Fatal("different locus/strand or better path was suppressed")
		}
	}
}

func TestNearRedundancyIndexMatchesScan(t *testing.T) {
	r := rand.New(rand.NewPCG(334, 819))
	var index redundancyIndex
	var retained []pairedPath
	for i := 0; i < 1500; i++ {
		m := containmentFixture(r.IntN(1500), r.IntN(1500), i%3 == 0,
			[]Op{{'=', 50 + r.IntN(300)}, {'I', 1 + r.IntN(8)}, {'=', 50 + r.IntN(300)}})
		m.AContig, m.BContig = i%2, i%3
		if i%4 == 0 && len(retained) != 0 {
			m = containmentSubpath(retained[r.IntN(len(retained))].match, r)
		}
		p := pairedMatch(m)
		want := false
		for _, big := range retained {
			if nearContained(big, p, 100) {
				want = true
				break
			}
		}
		if got := index.contains(p, 100); got != want {
			t.Fatalf("query %d: indexed=%v scan=%v", i, got, want)
		}
		if !want {
			index.add(p)
			retained = append(retained, p)
		}
	}
}

func TestNearRedundancyRepeatCopiesAndSymmetry(t *testing.T) {
	o := DefaultOptions()
	o.Redundancy = RedundancyNear
	unit := dna(60, 3)
	left, right := dna(1400, 4), dna(1400, 5)
	a := append(append(slices.Clone(left), []byte(strings.Repeat(string(unit), 3))...), right...)
	b := append(append(slices.Clone(left), []byte(strings.Repeat(string(unit), 6))...), right...)
	o.MinLength, o.MinChainScore = 50, 25
	ms := compare(t, genome(t, a), genome(t, b), o)
	for i := 0; i < 3; i++ {
		for j := 0; j < 6; j++ {
			found := false
			for _, m := range ms {
				found = found || (!m.Reverse && pathCoversPair(m, 1400+i*60, 1400+j*60, 60))
			}
			if !found {
				t.Fatalf("missing tandem placement %d/%d", i, j)
			}
		}
	}
	s := dna(400, 88)
	q := genome(t, s, s)
	target := genome(t, s, s, s, reverseComplement(s))
	if ms := compare(t, q, target, o); len(ms) != 8 {
		t.Fatalf("missing Cartesian repeat placements: %d", len(ms))
	}
	compare(t, target, target, o)
	// Equal-content genomes with reverse copies must close the retained path
	// set under exact operation swapping, including ambiguous short indels.
	edited := append(append(slices.Clone(s[:200]), 'A'), s[200:]...)
	self := genome(t, s, edited, reverseComplement(edited))
	compare(t, self, self, o)
}

func TestNearRedundancyRandomReplay(t *testing.T) {
	r := rand.New(rand.NewPCG(888, 3))
	for trial := 0; trial < 120; trial++ {
		q := dna(300+r.IntN(400), uint64(trial+100))
		target := slices.Clone(q)
		at := 30 + r.IntN(len(q)-60)
		if trial%2 == 0 {
			target = append(append(slices.Clone(target[:at]), dna(1+r.IntN(10), uint64(trial))...), target[at:]...)
		} else {
			target = append(slices.Clone(target[:at]), target[at+1+r.IntN(10):]...)
		}
		if trial%3 == 0 {
			target = reverseComplement(target)
		}
		o := DefaultOptions()
		o.Redundancy = RedundancyNear
		o.EndExtension = EndExtensionMode(trial % 3)
		a, b := genome(t, q), genome(t, target)
		if slices.Compare(a.digest[:], b.digest[:]) > 0 {
			a, b = b, a
		}
		ms := compare(t, a, b, o)
		if len(ms) == 0 {
			t.Fatal("lost edited whole-sequence match")
		}
		for _, m := range ms {
			n := normalizeMatch(m, a, b, o)
			if !reflect.DeepEqual(m, n) {
				t.Fatalf("trial %d: normalization was not idempotent: %s -> %s", trial, m.CIGAR(), n.CIGAR())
			}
		}
	}
}

func TestNearRedundancyCancellationAndOptions(t *testing.T) {
	g := genome(t, dna(400, 77))
	o := DefaultOptions()
	o.Redundancy = RedundancyNear + 1
	if _, err := Compare(context.Background(), g, g, o); err == nil {
		t.Fatal("accepted invalid redundancy policy")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if ms, err := cleanRedundantMatches(ctx, g, g, []Match{{}}, DefaultOptions()); ms != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v, %v", ms, err)
	}
}

func TestNearRedundancyConcurrentCalls(t *testing.T) {
	s := dna(900, 91)
	edited := append(append(slices.Clone(s[:450]), 'A'), s[450:]...)
	a, b := genome(t, s, reverseComplement(s)), genome(t, edited)
	o := DefaultOptions()
	o.Redundancy = RedundancyNear
	want := compare(t, a, b, o)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := Compare(context.Background(), a, b, o)
			if err == nil && !reflect.DeepEqual(got, want) {
				err = errors.New("results differ")
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal("concurrent comparison changed results", err)
		}
	}
}
