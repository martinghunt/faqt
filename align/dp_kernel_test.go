package align

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestDPPlatformMatchesScalar(t *testing.T) {
	r := rand.New(rand.NewPCG(178, 642))
	ctx := context.Background()
	for trial := 0; trial < 1500; trial++ {
		m, n := r.IntN(90)+1, r.IntN(90)+1
		q, targ := dna(m, uint64(trial+11)), dna(n, uint64(trial+319))
		if trial%3 == 0 {
			n = m
			targ = slicesCopy(q)
			for k := 0; k < m/10; k++ {
				targ[r.IntN(m)] = 'N'
			}
		}
		o := DefaultOptions()
		o.ZDrop = r.IntN(80)
		band := r.IntN(100) + 1
		if trial%5 == 0 {
			o.Match = 1 + r.IntN(5)
			o.Mismatch = 1 + r.IntN(8)
			o.GapOpen = 1 + r.IntN(8)
			o.LongGapOpen = 1 + r.IntN(30)
		}
		for _, ext := range []bool{true, false} {
			a, ea := alignDPScalar(ctx, q, targ, band, ext, o)
			for name, fn := range map[string]func(context.Context, []byte, []byte, int, bool, Options) (dpResult, error){"platform": alignDPPlatform} {
				b, eb := fn(ctx, q, targ, band, ext, o)
				if fmt.Sprint(ea) != fmt.Sprint(eb) || !reflect.DeepEqual(a, b) {
					t.Fatalf("%s trial=%d ext=%v band=%d\nref=%+v %v\ngot=%+v %v", name, trial, ext, band, a, ea, b, eb)
				}
			}
		}
	}
}

func TestDPPlatformFallbacks(t *testing.T) {
	ctx := context.Background()
	for _, size := range []int{0, 1, 256, 4097} {
		q, targ := dna(size, 492), dna(3, 493)
		for _, ext := range []bool{false, true} {
			for _, limit := range []int64{1, 16000000} {
				o := DefaultOptions()
				o.MaxCells = limit
				a, ea := alignDPScalar(ctx, q, targ, 96, ext, o)
				b, eb := alignDPPlatform(ctx, q, targ, 96, ext, o)
				if fmt.Sprint(ea) != fmt.Sprint(eb) || !reflect.DeepEqual(a, b) {
					t.Fatalf("fallback size=%d ext=%v limit=%d scalar=%+v %v platform=%+v %v", size, ext, limit, a, ea, b, eb)
				}
			}
		}
	}

	// The public options permit scores wider than the NEON int32 lanes.
	wide := DefaultOptions()
	wide.Match = 1 << 26
	sequence := dna(8, 495)
	a, ea := alignDPScalar(ctx, sequence, sequence, 96, true, wide)
	b, eb := alignDPPlatform(ctx, sequence, sequence, 96, true, wide)
	if fmt.Sprint(ea) != fmt.Sprint(eb) || !reflect.DeepEqual(a, b) {
		t.Fatal("wide scoring did not use the scalar fallback")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	o := DefaultOptions()
	q := dna(100, 494)
	_, ea = alignDPScalar(canceled, q, q, 96, true, o)
	_, eb = alignDPPlatform(canceled, q, q, 96, true, o)
	if fmt.Sprint(ea) != fmt.Sprint(eb) || eb == nil {
		t.Fatal("cancellation changed")
	}
}

func BenchmarkDPKernels(b *testing.B) {
	ctx := context.Background()
	o := DefaultOptions()
	for _, size := range []int{32, 128, 256, 1000} {
		q := dna(size, 947)
		target := slicesCopy(q)
		for i := 15; i < size; i += 37 {
			target[i] = 'N'
		}
		for name, fn := range map[string]func(context.Context, []byte, []byte, int, bool, Options) (dpResult, error){"scalar": alignDPScalar, "platform": alignDPPlatform} {
			b.Run(fmt.Sprintf("%d/%s", size, name), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, err := fn(ctx, q, target, 96, true, o)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
