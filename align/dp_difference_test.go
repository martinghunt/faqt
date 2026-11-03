//go:build goexperiment.simd && (arm64 || amd64)

package align

import (
	"context"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func TestDifferenceKernelLongPathsAndScoring(t *testing.T) {
	r := rand.New(rand.NewPCG(402, 941))
	ctx := context.Background()
	for trial := 0; trial < 1200; trial++ {
		m := 16 + r.IntN(370)
		q := dna(m, uint64(trial+812))
		target := slicesCopy(q)
		for i := 0; i < m/17; i++ {
			target[r.IntN(len(target))] = "ACGTN"[r.IntN(5)]
		}
		switch trial % 4 {
		case 0:
			at := r.IntN(m)
			target = append(append(slicesCopy(target[:at]), dna(1+r.IntN(80), uint64(trial+991))...), target[at:]...)
		case 1:
			target = dna(16+r.IntN(370), uint64(trial+211))
		case 2:
			for i := range target {
				if i%5 == 0 {
					target[i] = 'N'
				}
			}
		}
		o := DefaultOptions()
		o.ZDrop = r.IntN(400)
		if trial%3 == 0 {
			o.Match = 1 + r.IntN(6)
			o.Mismatch = 1 + r.IntN(12)
			o.GapOpen = 1 + r.IntN(12)
			o.GapExtend = 1 + r.IntN(6)
			o.LongGapOpen = 1 + r.IntN(24)
			o.LongGapExtend = 1 + r.IntN(5)
		}
		band := 16 + r.IntN(150)
		for _, ext := range []bool{false, true} {
			want, ew := alignDPScalar(ctx, q, target, band, ext, o)
			got, eg := alignDPPlatform(ctx, q, target, band, ext, o)
			if fmt.Sprint(ew) != fmt.Sprint(eg) || !reflect.DeepEqual(got, want) {
				t.Fatalf("trial=%d ext=%v band=%d opts=%+v\nwant=%+v %v\ngot=%+v %v", trial, ext, band, o, want, ew, got, eg)
			}
		}
	}
	for _, pair := range [][2]string{{strings.Repeat("A", 1000), strings.Repeat("A", 1000)}, {strings.Repeat("A", 512), strings.Repeat("T", 512)}, {strings.Repeat("N", 256), strings.Repeat("N", 256)}} {
		for _, ext := range []bool{false, true} {
			o := DefaultOptions()
			q, target := []byte(pair[0]), []byte(pair[1])
			want, ew := alignDPScalar(ctx, q, target, 96, ext, o)
			got, eg := alignDPPlatform(ctx, q, target, 96, ext, o)
			if fmt.Sprint(ew) != fmt.Sprint(eg) || !reflect.DeepEqual(got, want) {
				t.Fatalf("repetitive ext=%v want=%+v %v got=%+v %v", ext, want, ew, got, eg)
			}
		}
	}
}
