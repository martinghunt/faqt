package align

import (
	"bytes"
	"context"
	"errors"
	"math/rand/v2"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestDPBoundaryMatchesScalar(t *testing.T) {
	rng := rand.New(rand.NewPCG(5721, 153))
	for trial := 0; trial < 900; trial++ {
		m, n := rng.IntN(210), rng.IntN(210)
		q, target := dna(m, uint64(trial+901)), dna(n, uint64(trial+4021))
		if trial%3 == 0 {
			target = slicesCopy(q)
			n = m
			for i := 0; i < n/7; i++ {
				target[rng.IntN(n)] = 'N'
			}
		}
		o := DefaultOptions()
		o.ZDrop = rng.IntN(240)
		band := []int{1, 15, 16, 32, 96, 256}[trial%6]
		if trial%9 == 0 {
			o.Match = 40
			o.Mismatch = 65
		}
		if trial%13 == 0 {
			o.Match = 1 << 26
		}
		var want, got dpBoundary
		a, ea := alignDPScalarBoundary(context.Background(), q, target, band, true, o, &want)
		b, eb := alignDPPlatformBoundary(context.Background(), q, target, band, true, o, &got)
		if !reflect.DeepEqual(a, b) || !reflect.DeepEqual(want, got) || !sameError(ea, eb) {
			t.Fatalf("trial=%d m=%d n=%d band=%d\nscalar=%+v %v boundary=%+v\nplatform=%+v %v boundary=%+v", trial, m, n, band, a, ea, want, b, eb, got)
		}
	}
}
func sameError(a, b error) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && a.Error() == b.Error())
}

func TestAdaptiveEndExtension(t *testing.T) {
	for _, size := range []int{0, 1, 15, 16, 31, 63, 64, 65, 127, 128, 129, 255, 256, 300} {
		q := dna(size, uint64(size+5201))
		target := slicesCopy(q)
		if size > 90 {
			target = append(append(slicesCopy(q[:71]), []byte("ACGTACG")...), q[71:]...)
		}
		o := DefaultOptions()
		want, err := alignDP(context.Background(), q, target, o.BandWidth, true, o)
		if err != nil {
			t.Fatal(err)
		}
		o.EndExtension = EndExtensionAdaptive
		got, err := alignEnd(context.Background(), q, target, o)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("size=%d expected supported exact/indel continuation\nwant=%+v\ngot=%+v", size, want, got)
		}
	}
	o := DefaultOptions()
	o.EndExtension = EndExtensionAdaptive
	q := dna(256, 5032)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := alignEnd(ctx, q, q, o); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	o.MaxCells = 1
	var limit *LimitError
	if _, err := alignEnd(context.Background(), q, q, o); !errors.As(err, &limit) {
		t.Fatalf("cell budget: %v", err)
	}
}

func TestAdaptiveBoundaryHeuristicKeepsFullMode(t *testing.T) {
	q := append(append(bytes.Repeat([]byte{'A'}, 32), bytes.Repeat([]byte{'C'}, 32)...), bytes.Repeat([]byte{'T'}, 192)...)
	target := append(append(bytes.Repeat([]byte{'A'}, 32), bytes.Repeat([]byte{'G'}, 32)...), bytes.Repeat([]byte{'T'}, 192)...)
	target[0] = 'C'
	o := DefaultOptions()
	full, err := alignEnd(context.Background(), q, target, o)
	if err != nil {
		t.Fatal(err)
	}
	o.EndExtension = EndExtensionAdaptive
	fast, err := alignEnd(context.Background(), q, target, o)
	if err != nil {
		t.Fatal(err)
	}
	if full.q != 256 || full.t != 256 || full.score <= fast.score || fast.q >= 64 || fast.t >= 64 {
		t.Fatalf("expected documented score-valley tradeoff, full=%+v adaptive=%+v", full, fast)
	}
}

func TestAdaptiveConcurrentCalls(t *testing.T) {
	o := DefaultOptions()
	o.EndExtension = EndExtensionAdaptive
	q := dna(256, 90111)
	target := slicesCopy(q)
	target[51] = 'N'
	target[171] = 'N'
	want, err := alignEnd(context.Background(), q, target, o)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 20; j++ {
				got, err := alignEnd(context.Background(), q, target, o)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Errorf("concurrent result: %+v %v", got, err)
					return
				}
			}
		}()
	}
	group.Wait()
}

func TestAdaptiveEndsPreserveInternalIndelsAndRepeats(t *testing.T) {
	base := dna(1500, 7511)
	inserted := append(append(slicesCopy(base[:700]), dna(31, 7512)...), base[700:]...)
	o := DefaultOptions()
	o.EndExtension = EndExtensionAdaptive
	for _, reverse := range []bool{false, true} {
		target := inserted
		if reverse {
			target = reverseComplement(target)
		}
		found := false
		for _, match := range compare(t, genome(t, base), genome(t, target), o) {
			if match.Reverse == reverse && strings.Contains(match.CIGAR(), "31D") {
				found = true
			}
		}
		if !found {
			t.Fatalf("internal indel lost, reverse=%v", reverse)
		}
	}
	repeat := dna(400, 7513)
	a := genome(t, repeat, repeat)
	targets := [][]byte{repeat, repeat, repeat, repeat, repeat, repeat, repeat, reverseComplement(repeat)}
	if matches := compare(t, a, genome(t, targets...), o); len(matches) != 16 {
		t.Fatalf("repeat placements=%d, expected16", len(matches))
	}
}

func TestAdaptiveScoringScale(t *testing.T) {
	q := dna(190, 88131)
	targets := [][]byte{slicesCopy(q), append(append(slicesCopy(q[:71]), dna(9, 88132)...), q[71:]...), dna(190, 88133), append(append(slicesCopy(q[:30]), dna(40, 88134)...), q[70:]...)}
	for _, target := range targets {
		base := DefaultOptions()
		base.EndExtension = EndExtensionAdaptive
		want, err := alignEnd(context.Background(), q, target, base)
		if err != nil {
			t.Fatal(err)
		}
		for _, factor := range []int{2, 3} {
			scaled := base
			scaled.Match *= factor
			scaled.Mismatch *= factor
			scaled.GapOpen *= factor
			scaled.GapExtend *= factor
			scaled.LongGapOpen *= factor
			scaled.LongGapExtend *= factor
			scaled.ZDrop *= factor
			got, err := alignEnd(context.Background(), q, target, scaled)
			if err != nil {
				t.Fatal(err)
			}
			expected := want
			expected.score *= factor
			if !reflect.DeepEqual(got, expected) {
				t.Fatalf("score scaling %d: got=%+v want=%+v", factor, got, expected)
			}
		}
	}
}
